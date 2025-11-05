package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/williamoconnorme/terraform-provider-servicefabric/internal/servicefabric"
)

var _ datasource.DataSource = &applicationTypeDataSource{}

type applicationTypeDataSource struct {
	client *servicefabric.Client
}

type applicationTypeDataSourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Version           types.String `tfsdk:"version"`
	Status            types.String `tfsdk:"status"`
	DefaultParameters types.Map    `tfsdk:"default_parameters"`
	ApplicationTypes  types.List   `tfsdk:"application_types"`
}

var applicationTypeItemAttrTypes = map[string]attr.Type{
	"name":               types.StringType,
	"version":            types.StringType,
	"status":             types.StringType,
	"default_parameters": types.MapType{ElemType: types.StringType},
}

var applicationTypeItemObjectType = types.ObjectType{
	AttrTypes: applicationTypeItemAttrTypes,
}

func NewApplicationTypeDataSource() datasource.DataSource {
	return &applicationTypeDataSource{}
}

func (d *applicationTypeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application_type"
}

func (d *applicationTypeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Unique identifier composed of name/version.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Description: "Application type name. When omitted, all application types are returned.",
			},
			"version": schema.StringAttribute{
				Optional:    true,
				Description: "Application type version. Requires name. When omitted, all versions are returned.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Provisioning status.",
			},
			"default_parameters": schema.MapAttribute{
				ElementType: types.StringType,
				Computed:    true,
				Description: "Default application parameters declared in the manifest.",
			},
			"application_types": schema.ListNestedAttribute{
				Computed:    true,
				Description: "List of application types matching the given filters.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "Application type name.",
						},
						"version": schema.StringAttribute{
							Computed:    true,
							Description: "Application type version.",
						},
						"status": schema.StringAttribute{
							Computed:    true,
							Description: "Provisioning status of the application type.",
						},
						"default_parameters": schema.MapAttribute{
							ElementType: types.StringType,
							Computed:    true,
							Description: "Default parameters for the application type.",
						},
					},
				},
			},
		},
	}
}

func (d *applicationTypeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok || data == nil {
		return
	}
	d.client = data.Client
}

func (d *applicationTypeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state applicationTypeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := ""
	if !state.Name.IsNull() {
		name = state.Name.ValueString()
	}
	version := ""
	if !state.Version.IsNull() {
		version = state.Version.ValueString()
	}

	// version without name is invalid.
	if version != "" && name == "" {
		resp.Diagnostics.AddError(
			"Invalid application type data source configuration",
			"`version` requires `name` to be specified.",
		)
		return
	}

	var (
		infos []servicefabric.ApplicationTypeInfo
		err   error
	)

	switch {
	case name != "" && version != "":
		var info *servicefabric.ApplicationTypeInfo
		info, err = d.client.GetApplicationTypeVersion(ctx, name, version)
		if err == nil {
			infos = []servicefabric.ApplicationTypeInfo{*info}
		}
	case name != "":
		infos, err = d.client.ListApplicationTypeVersions(ctx, name)
	default:
		infos, err = d.client.ListApplicationTypeVersions(ctx, "")
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read application type", err.Error())
		return
	}

	if len(infos) == 0 {
		resp.Diagnostics.AddError(
			"Application type not found",
			fmt.Sprintf("No application types matched name %q and version %q.", name, version),
		)
		return
	}

	// Prepare list output.
	itemValues := make([]attr.Value, 0, len(infos))
	for _, info := range infos {
		params := servicefabric.ParameterListToMap(info.DefaultParameterList)
		paramsVal := types.MapNull(types.StringType)
		if len(params) > 0 {
			paramsVal = types.MapValueMust(types.StringType, convertStringMapToAttrValues(params))
		}

		objVal, diag := types.ObjectValue(applicationTypeItemAttrTypes, map[string]attr.Value{
			"name":               types.StringValue(info.TypeName()),
			"version":            types.StringValue(info.TypeVersion()),
			"status":             types.StringValue(info.Status),
			"default_parameters": paramsVal,
		})
		resp.Diagnostics.Append(diag...)
		if resp.Diagnostics.HasError() {
			return
		}
		itemValues = append(itemValues, objVal)
	}

	listVal := types.ListNull(applicationTypeItemObjectType)
	if len(itemValues) > 0 {
		listComputed, diags := types.ListValue(applicationTypeItemObjectType, itemValues)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		listVal = listComputed
	}

	// Prepare top-level fields.
	state.ApplicationTypes = listVal
	state.ID = types.StringNull()
	state.Status = types.StringNull()
	state.DefaultParameters = types.MapNull(types.StringType)

	// Preserve provided filters.
	if name != "" {
		state.Name = types.StringValue(name)
	} else {
		state.Name = types.StringNull()
	}
	if version != "" {
		state.Version = types.StringValue(version)
	} else {
		state.Version = types.StringNull()
	}

	// When a single entry is returned, populate convenience attributes.
	var target *servicefabric.ApplicationTypeInfo
	if len(infos) == 1 {
		target = &infos[0]
	}
	if target != nil {
		state.ID = types.StringValue(fmt.Sprintf("%s/%s", target.TypeName(), target.TypeVersion()))
		state.Name = types.StringValue(target.TypeName())
		state.Version = types.StringValue(target.TypeVersion())
		state.Status = types.StringValue(target.Status)

		params := servicefabric.ParameterListToMap(target.DefaultParameterList)
		if len(params) > 0 {
			state.DefaultParameters = types.MapValueMust(types.StringType, convertStringMapToAttrValues(params))
		} else {
			state.DefaultParameters = types.MapNull(types.StringType)
		}
	}
	if target == nil {
		// Keep defaults when not a single target.
		if state.DefaultParameters.IsNull() {
			state.DefaultParameters = types.MapNull(types.StringType)
		}
		state.Status = types.StringNull()
		state.ID = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
