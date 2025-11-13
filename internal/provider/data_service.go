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

var _ datasource.DataSource = &serviceDataSource{}

type serviceDataSource struct {
	client *servicefabric.Client
}

type serviceDataSourceModel struct {
	ID                types.String `tfsdk:"id"`
	ApplicationName   types.String `tfsdk:"application_name"`
	Name              types.String `tfsdk:"name"`
	ServiceTypeName   types.String `tfsdk:"service_type_name"`
	TypeName          types.String `tfsdk:"type_name"`
	ManifestVersion   types.String `tfsdk:"manifest_version"`
	ServiceKind       types.String `tfsdk:"service_kind"`
	HealthState       types.String `tfsdk:"health_state"`
	ServiceStatus     types.String `tfsdk:"service_status"`
	IsServiceGroup    types.Bool   `tfsdk:"is_service_group"`
	HasPersistedState types.Bool   `tfsdk:"has_persisted_state"`
	ArmResourceID     types.String `tfsdk:"arm_resource_id"`
	Services          types.List   `tfsdk:"services"`
}

var serviceItemAttrTypes = map[string]attr.Type{
	"id":                  types.StringType,
	"name":                types.StringType,
	"service_kind":        types.StringType,
	"type_name":           types.StringType,
	"manifest_version":    types.StringType,
	"health_state":        types.StringType,
	"service_status":      types.StringType,
	"is_service_group":    types.BoolType,
	"has_persisted_state": types.BoolType,
	"arm_resource_id":     types.StringType,
}

var serviceItemObjectType = types.ObjectType{
	AttrTypes: serviceItemAttrTypes,
}

func NewServiceDataSource() datasource.DataSource {
	return &serviceDataSource{}
}

func (d *serviceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service"
}

func (d *serviceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Identifier for the lookup. Uses the Service Fabric service ID when a single service is returned.",
			},
			"application_name": schema.StringAttribute{
				Required:    true,
				Description: "Full Service Fabric application name (fabric:/...) that owns the services.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Description: "Full Service Fabric service name (fabric:/...) to retrieve. When omitted, all services in the application are listed.",
			},
			"service_type_name": schema.StringAttribute{
				Optional:    true,
				Description: "Filter returned services by the given service type name when listing.",
			},
			"type_name": schema.StringAttribute{
				Computed:    true,
				Description: "Service type name for the selected service.",
			},
			"manifest_version": schema.StringAttribute{
				Computed:    true,
				Description: "Service manifest version associated with the selected service.",
			},
			"service_kind": schema.StringAttribute{
				Computed:    true,
				Description: "Service kind (Stateful or Stateless).",
			},
			"health_state": schema.StringAttribute{
				Computed:    true,
				Description: "Current health state reported by the cluster.",
			},
			"service_status": schema.StringAttribute{
				Computed:    true,
				Description: "Provisioning status of the service.",
			},
			"is_service_group": schema.BoolAttribute{
				Computed:    true,
				Description: "Indicates whether the service is part of a service group.",
			},
			"has_persisted_state": schema.BoolAttribute{
				Computed:    true,
				Description: "When known, indicates whether the service has persisted state.",
			},
			"arm_resource_id": schema.StringAttribute{
				Computed:    true,
				Description: "ARM resource identifier reported for the service when available.",
			},
			"services": schema.ListNestedAttribute{
				Computed:    true,
				Description: "List of services that matched the query.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:    true,
							Description: "Service Fabric service ID.",
						},
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "Full Service Fabric service name.",
						},
						"service_kind": schema.StringAttribute{
							Computed:    true,
							Description: "Service kind.",
						},
						"type_name": schema.StringAttribute{
							Computed:    true,
							Description: "Service type name.",
						},
						"manifest_version": schema.StringAttribute{
							Computed:    true,
							Description: "Manifest version associated with the service.",
						},
						"health_state": schema.StringAttribute{
							Computed:    true,
							Description: "Health state of the service.",
						},
						"service_status": schema.StringAttribute{
							Computed:    true,
							Description: "Service status.",
						},
						"is_service_group": schema.BoolAttribute{
							Computed:    true,
							Description: "Indicates whether the service belongs to a service group.",
						},
						"has_persisted_state": schema.BoolAttribute{
							Computed:    true,
							Description: "Whether the service has persisted state when known.",
						},
						"arm_resource_id": schema.StringAttribute{
							Computed:    true,
							Description: "ARM resource identifier for the service when available.",
						},
					},
				},
			},
		},
	}
}

func (d *serviceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok || data == nil {
		return
	}
	d.client = data.Client
}

func (d *serviceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state serviceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.ApplicationName.IsNull() || state.ApplicationName.ValueString() == "" {
		resp.Diagnostics.AddError("Missing application name", "application_name must be supplied.")
		return
	}

	appName := state.ApplicationName.ValueString()
	serviceName := ""
	if !state.Name.IsNull() && state.Name.ValueString() != "" {
		serviceName = state.Name.ValueString()
	}
	filterType := ""
	if !state.ServiceTypeName.IsNull() && state.ServiceTypeName.ValueString() != "" {
		filterType = state.ServiceTypeName.ValueString()
	}

	var (
		items []servicefabric.ServiceInfo
		err   error
	)
	if serviceName != "" {
		var info *servicefabric.ServiceInfo
		info, err = d.client.GetService(ctx, appName, serviceName)
		if err == nil && info != nil {
			items = []servicefabric.ServiceInfo{*info}
		}
	} else {
		items, err = d.client.ListServices(ctx, appName, filterType)
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read services", err.Error())
		return
	}
	if len(items) == 0 {
		resp.Diagnostics.AddError("Service not found", fmt.Sprintf("No services matched application %q.", appName))
		return
	}

	listValues := make([]attr.Value, 0, len(items))
	for _, item := range items {
		obj, diags := types.ObjectValue(serviceItemAttrTypes, map[string]attr.Value{
			"id":                  stringOrNull(item.ID),
			"name":                stringOrNull(item.Name),
			"service_kind":        stringOrNull(serviceKindValue(item)),
			"type_name":           stringOrNull(item.TypeName),
			"manifest_version":    stringOrNull(item.ManifestVersion),
			"health_state":        stringOrNull(item.HealthState),
			"service_status":      stringOrNull(item.ServiceStatus),
			"is_service_group":    types.BoolValue(item.IsServiceGroup),
			"has_persisted_state": boolOrNull(item.HasPersistedState),
			"arm_resource_id":     stringOrNull(armResourceID(item)),
		})
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		listValues = append(listValues, obj)
	}

	listAttr := types.ListNull(serviceItemObjectType)
	if len(listValues) > 0 {
		computed, diags := types.ListValue(serviceItemObjectType, listValues)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		listAttr = computed
	}

	state.Services = listAttr
	state.ApplicationName = types.StringValue(appName)
	if serviceName != "" {
		state.Name = types.StringValue(serviceName)
	} else {
		state.Name = types.StringNull()
	}
	if filterType != "" {
		state.ServiceTypeName = types.StringValue(filterType)
	} else {
		state.ServiceTypeName = types.StringNull()
	}
	state.TypeName = types.StringNull()
	state.ManifestVersion = types.StringNull()
	state.ServiceKind = types.StringNull()
	state.HealthState = types.StringNull()
	state.ServiceStatus = types.StringNull()
	state.IsServiceGroup = types.BoolNull()
	state.HasPersistedState = types.BoolNull()
	state.ArmResourceID = types.StringNull()
	state.ID = stringOrNull(appName)

	if len(items) == 1 {
		item := items[0]
		state.ID = stringOrNull(item.ID)
		state.Name = stringOrNull(item.Name)
		state.TypeName = stringOrNull(item.TypeName)
		state.ManifestVersion = stringOrNull(item.ManifestVersion)
		state.ServiceKind = stringOrNull(serviceKindValue(item))
		state.HealthState = stringOrNull(item.HealthState)
		state.ServiceStatus = stringOrNull(item.ServiceStatus)
		state.IsServiceGroup = types.BoolValue(item.IsServiceGroup)
		state.HasPersistedState = boolOrNull(item.HasPersistedState)
		state.ArmResourceID = stringOrNull(armResourceID(item))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func serviceKindValue(info servicefabric.ServiceInfo) string {
	if info.ServiceKind != "" {
		return info.ServiceKind
	}
	return info.Kind
}

func armResourceID(info servicefabric.ServiceInfo) string {
	if info.ServiceMetadata == nil || info.ServiceMetadata.ArmMetadata == nil {
		return ""
	}
	return info.ServiceMetadata.ArmMetadata.ArmResourceID
}
