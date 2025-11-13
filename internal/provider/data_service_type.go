package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/williamoconnorme/terraform-provider-servicefabric/internal/servicefabric"
)

var _ datasource.DataSource = &serviceTypeDataSource{}

type serviceTypeDataSource struct {
	client *servicefabric.Client
}

type serviceTypeDataSourceModel struct {
	ID                         types.String `tfsdk:"id"`
	ApplicationTypeName        types.String `tfsdk:"application_type_name"`
	ApplicationTypeVersion     types.String `tfsdk:"application_type_version"`
	ServiceTypeName            types.String `tfsdk:"service_type_name"`
	ServiceManifestName        types.String `tfsdk:"service_manifest_name"`
	ServiceManifestVersion     types.String `tfsdk:"service_manifest_version"`
	IsServiceGroup             types.Bool   `tfsdk:"is_service_group"`
	Kind                       types.String `tfsdk:"kind"`
	HasPersistedState          types.Bool   `tfsdk:"has_persisted_state"`
	ServiceTypeDescriptionJSON types.String `tfsdk:"service_type_description_json"`
	ServiceTypes               types.List   `tfsdk:"service_types"`
}

var serviceTypeItemAttrTypes = map[string]attr.Type{
	"service_type_name":             types.StringType,
	"kind":                          types.StringType,
	"service_manifest_name":         types.StringType,
	"service_manifest_version":      types.StringType,
	"is_service_group":              types.BoolType,
	"has_persisted_state":           types.BoolType,
	"service_type_description_json": types.StringType,
}

var serviceTypeItemObjectType = types.ObjectType{
	AttrTypes: serviceTypeItemAttrTypes,
}

func NewServiceTypeDataSource() datasource.DataSource {
	return &serviceTypeDataSource{}
}

func (d *serviceTypeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_type"
}

func (d *serviceTypeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Unique identifier derived from application type and service type values.",
			},
			"application_type_name": schema.StringAttribute{
				Required:    true,
				Description: "Application type name that declares the service type.",
			},
			"application_type_version": schema.StringAttribute{
				Required:    true,
				Description: "Application type version that declares the service type.",
			},
			"service_type_name": schema.StringAttribute{
				Optional:    true,
				Description: "Specific service type name to fetch. When omitted, all service types in the application type version are returned.",
			},
			"service_manifest_name": schema.StringAttribute{
				Computed:    true,
				Description: "Name of the service manifest that defines the service type.",
			},
			"service_manifest_version": schema.StringAttribute{
				Computed:    true,
				Description: "Version of the service manifest that defines the service type.",
			},
			"is_service_group": schema.BoolAttribute{
				Computed:    true,
				Description: "Indicates whether the service type is part of a service group.",
			},
			"kind": schema.StringAttribute{
				Computed:    true,
				Description: "Service type kind (Stateful or Stateless).",
			},
			"has_persisted_state": schema.BoolAttribute{
				Computed:    true,
				Description: "When known, indicates whether the service type persists state.",
			},
			"service_type_description_json": schema.StringAttribute{
				Computed:    true,
				Description: "Raw JSON payload returned by Service Fabric describing the service type.",
			},
			"service_types": schema.ListNestedAttribute{
				Computed:    true,
				Description: "List of service types returned by the query.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"service_type_name": schema.StringAttribute{
							Computed:    true,
							Description: "Service type name.",
						},
						"kind": schema.StringAttribute{
							Computed:    true,
							Description: "Service kind.",
						},
						"service_manifest_name": schema.StringAttribute{
							Computed:    true,
							Description: "Service manifest name containing the service type.",
						},
						"service_manifest_version": schema.StringAttribute{
							Computed:    true,
							Description: "Service manifest version containing the service type.",
						},
						"is_service_group": schema.BoolAttribute{
							Computed:    true,
							Description: "Indicates whether the service type belongs to a service group.",
						},
						"has_persisted_state": schema.BoolAttribute{
							Computed:    true,
							Description: "Whether services of this type persist state when known.",
						},
						"service_type_description_json": schema.StringAttribute{
							Computed:    true,
							Description: "Raw JSON describing the service type.",
						},
					},
				},
			},
		},
	}
}

func (d *serviceTypeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok || data == nil {
		return
	}
	d.client = data.Client
}

func (d *serviceTypeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state serviceTypeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.ApplicationTypeName.IsNull() || state.ApplicationTypeName.ValueString() == "" {
		resp.Diagnostics.AddError("Missing application type name", "application_type_name must be set.")
		return
	}
	if state.ApplicationTypeVersion.IsNull() || state.ApplicationTypeVersion.ValueString() == "" {
		resp.Diagnostics.AddError("Missing application type version", "application_type_version must be set.")
		return
	}

	appTypeName := state.ApplicationTypeName.ValueString()
	appTypeVersion := state.ApplicationTypeVersion.ValueString()

	serviceTypeName := ""
	if !state.ServiceTypeName.IsNull() && state.ServiceTypeName.ValueString() != "" {
		serviceTypeName = state.ServiceTypeName.ValueString()
	}

	var (
		items []servicefabric.ServiceTypeInfo
		err   error
	)
	if serviceTypeName != "" {
		var info *servicefabric.ServiceTypeInfo
		info, err = d.client.GetServiceType(ctx, appTypeName, appTypeVersion, serviceTypeName)
		if err == nil && info != nil {
			items = []servicefabric.ServiceTypeInfo{*info}
		}
	} else {
		items, err = d.client.ListServiceTypes(ctx, appTypeName, appTypeVersion)
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read service types", err.Error())
		return
	}

	if len(items) == 0 {
		resp.Diagnostics.AddError("Service types not found", fmt.Sprintf("No service types matched application type %q version %q.", appTypeName, appTypeVersion))
		return
	}

	listValues := make([]attr.Value, 0, len(items))
	for _, item := range items {
		details := extractServiceTypeDetails(item)
		listObj, diags := types.ObjectValue(serviceTypeItemAttrTypes, map[string]attr.Value{
			"service_type_name":             stringOrNull(details.Name),
			"kind":                          stringOrNull(details.Kind),
			"service_manifest_name":         stringOrNull(item.ServiceManifestName),
			"service_manifest_version":      stringOrNull(item.ServiceManifestVersion),
			"is_service_group":              types.BoolValue(item.IsServiceGroup),
			"has_persisted_state":           boolOrNull(details.HasPersistedState),
			"service_type_description_json": stringOrNull(details.DescriptionJSON),
		})
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		listValues = append(listValues, listObj)
	}

	listAttr := types.ListNull(serviceTypeItemObjectType)
	if len(listValues) > 0 {
		computed, diags := types.ListValue(serviceTypeItemObjectType, listValues)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		listAttr = computed
	}

	state.ServiceTypes = listAttr
	state.ApplicationTypeName = types.StringValue(appTypeName)
	state.ApplicationTypeVersion = types.StringValue(appTypeVersion)
	if serviceTypeName != "" {
		state.ServiceTypeName = types.StringValue(serviceTypeName)
	} else {
		state.ServiceTypeName = types.StringNull()
	}
	state.ServiceManifestName = types.StringNull()
	state.ServiceManifestVersion = types.StringNull()
	state.Kind = types.StringNull()
	state.HasPersistedState = types.BoolNull()
	state.ServiceTypeDescriptionJSON = types.StringNull()
	state.IsServiceGroup = types.BoolNull()
	state.ID = types.StringValue(fmt.Sprintf("%s/%s", appTypeName, appTypeVersion))

	if len(items) == 1 {
		item := items[0]
		details := extractServiceTypeDetails(item)
		state.ID = types.StringValue(fmt.Sprintf("%s/%s/%s", appTypeName, appTypeVersion, details.Name))
		state.ServiceTypeName = stringOrNull(details.Name)
		state.ServiceManifestName = stringOrNull(item.ServiceManifestName)
		state.ServiceManifestVersion = stringOrNull(item.ServiceManifestVersion)
		state.Kind = stringOrNull(details.Kind)
		state.HasPersistedState = boolOrNull(details.HasPersistedState)
		state.ServiceTypeDescriptionJSON = stringOrNull(details.DescriptionJSON)
		state.IsServiceGroup = types.BoolValue(item.IsServiceGroup)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

type serviceTypeDetails struct {
	Name              string
	Kind              string
	HasPersistedState *bool
	DescriptionJSON   string
}

func extractServiceTypeDetails(info servicefabric.ServiceTypeInfo) serviceTypeDetails {
	result := serviceTypeDetails{
		DescriptionJSON: strings.TrimSpace(string(info.ServiceTypeDescription)),
	}
	if len(info.ServiceTypeDescription) == 0 {
		return result
	}
	var payload struct {
		ServiceTypeName   string `json:"ServiceTypeName"`
		Kind              string `json:"Kind"`
		HasPersistedState *bool  `json:"HasPersistedState"`
	}
	if err := json.Unmarshal(info.ServiceTypeDescription, &payload); err == nil {
		result.Name = payload.ServiceTypeName
		result.Kind = payload.Kind
		result.HasPersistedState = payload.HasPersistedState
	}
	return result
}

func stringOrNull(v string) types.String {
	if v == "" {
		return types.StringNull()
	}
	return types.StringValue(v)
}

func boolOrNull(v *bool) types.Bool {
	if v == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*v)
}
