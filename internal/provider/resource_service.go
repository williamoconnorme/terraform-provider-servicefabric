package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	stringplanmodifier "github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/williamoconnorme/terraform-provider-servicefabric/internal/servicefabric"
)

var _ resource.Resource = &serviceResource{}

type serviceResource struct {
	client *servicefabric.Client
}

type serviceResourceModel struct {
	ID                           types.String `tfsdk:"id"`
	Name                         types.String `tfsdk:"name"`
	ApplicationName              types.String `tfsdk:"application_name"`
	ServiceTypeName              types.String `tfsdk:"service_type_name"`
	ServiceKind                  types.String `tfsdk:"service_kind"`
	PlacementConstraints         types.String `tfsdk:"placement_constraints"`
	DefaultMoveCost              types.String `tfsdk:"default_move_cost"`
	ServicePackageActivationMode types.String `tfsdk:"service_package_activation_mode"`
	ServiceDnsName               types.String `tfsdk:"service_dns_name"`
	ForceRemove                  types.Bool   `tfsdk:"force_remove"`
	Partition                    types.Object `tfsdk:"partition"`
	Stateless                    types.Object `tfsdk:"stateless"`
	Stateful                     types.Object `tfsdk:"stateful"`
	HealthState                  types.String `tfsdk:"health_state"`
	ServiceStatus                types.String `tfsdk:"service_status"`
}

type partitionModel struct {
	Scheme  types.String `tfsdk:"scheme"`
	Count   types.Int64  `tfsdk:"count"`
	Names   types.List   `tfsdk:"names"`
	LowKey  types.Int64  `tfsdk:"low_key"`
	HighKey types.Int64  `tfsdk:"high_key"`
}

type statelessServiceModel struct {
	InstanceCount              types.Int64 `tfsdk:"instance_count"`
	MinInstanceCount           types.Int64 `tfsdk:"min_instance_count"`
	MinInstancePercentage      types.Int64 `tfsdk:"min_instance_percentage"`
	InstanceCloseDelaySeconds  types.Int64 `tfsdk:"instance_close_delay_seconds"`
	InstanceRestartWaitSeconds types.Int64 `tfsdk:"instance_restart_wait_seconds"`
}

type statefulServiceModel struct {
	TargetReplicaSetSize             types.Int64 `tfsdk:"target_replica_set_size"`
	MinReplicaSetSize                types.Int64 `tfsdk:"min_replica_set_size"`
	HasPersistedState                types.Bool  `tfsdk:"has_persisted_state"`
	ReplicaRestartWaitSeconds        types.Int64 `tfsdk:"replica_restart_wait_seconds"`
	QuorumLossWaitSeconds            types.Int64 `tfsdk:"quorum_loss_wait_seconds"`
	StandByReplicaKeepSeconds        types.Int64 `tfsdk:"standby_replica_keep_seconds"`
	ServicePlacementTimeLimitSeconds types.Int64 `tfsdk:"service_placement_time_limit_seconds"`
}

func NewServiceResource() resource.Resource {
	return &serviceResource{}
}

func (r *serviceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service"
}

func (r *serviceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = rschema.Schema{
		Attributes: map[string]rschema.Attribute{
			"id": rschema.StringAttribute{
				Computed:      true,
				Description:   "Unique identifier for the service (Service Fabric name).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": rschema.StringAttribute{
				Required:    true,
				Description: "Fully-qualified Service Fabric service name, e.g. fabric:/App/Service.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"application_name": rschema.StringAttribute{
				Required:    true,
				Description: "Service Fabric application that owns the service.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"service_type_name": rschema.StringAttribute{
				Required:    true,
				Description: "Service type registered in the application manifest.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"service_kind": rschema.StringAttribute{
				Required:    true,
				Description: "Service kind. Supported values: Stateful, Stateless.",
				Validators: []validator.String{
					stringvalidator.OneOf("Stateful", "Stateless"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"placement_constraints": rschema.StringAttribute{
				Optional:    true,
				Description: "Node placement constraints applied to the service.",
			},
			"default_move_cost": rschema.StringAttribute{
				Optional:    true,
				Description: "Service move cost preference. Allowed values: Zero, Low, Medium, High, VeryHigh.",
				Validators: []validator.String{
					stringvalidator.OneOf("Zero", "Low", "Medium", "High", "VeryHigh"),
				},
			},
			"service_package_activation_mode": rschema.StringAttribute{
				Optional:    true,
				Description: "Service package activation mode. Supported values: SharedProcess, ExclusiveProcess.",
				Validators: []validator.String{
					stringvalidator.OneOf("SharedProcess", "ExclusiveProcess"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"service_dns_name": rschema.StringAttribute{
				Optional:    true,
				Description: "DNS name assigned to the service, if configured.",
			},
			"force_remove": rschema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Forcefully delete the service without graceful shutdown.",
			},
			"health_state": rschema.StringAttribute{
				Computed:    true,
				Description: "Current health state reported by the cluster.",
			},
			"service_status": rschema.StringAttribute{
				Computed:    true,
				Description: "Provisioning status reported by the cluster.",
			},
			"partition": rschema.SingleNestedAttribute{
				Required:    true,
				Description: "Partitioning settings that determine how services are distributed.",
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
				Attributes: map[string]rschema.Attribute{
					"scheme": rschema.StringAttribute{
						Required:    true,
						Description: "Partition scheme. Supported values: Singleton, UniformInt64Range, Named.",
						Validators: []validator.String{
							stringvalidator.OneOf("Singleton", "UniformInt64Range", "Named"),
						},
					},
					"count": rschema.Int64Attribute{
						Optional:    true,
						Description: "Partition count for Named or UniformInt64Range schemes.",
					},
					"names": rschema.ListAttribute{
						Optional:    true,
						ElementType: types.StringType,
						Description: "Partition names when using the Named scheme.",
					},
					"low_key": rschema.Int64Attribute{
						Optional:    true,
						Description: "Low key for UniformInt64Range partitions.",
					},
					"high_key": rschema.Int64Attribute{
						Optional:    true,
						Description: "High key for UniformInt64Range partitions.",
					},
				},
			},
			"stateless": rschema.SingleNestedAttribute{
				Optional:    true,
				Description: "Stateless service configuration. Required when service_kind is Stateless.",
				Attributes: map[string]rschema.Attribute{
					"instance_count": rschema.Int64Attribute{
						Optional:    true,
						Description: "Number of instances per application partition (-1 deploys to every node).",
					},
					"min_instance_count": rschema.Int64Attribute{
						Optional:    true,
						Description: "Minimum number of instances to keep even when upgrades are rolling.",
					},
					"min_instance_percentage": rschema.Int64Attribute{
						Optional:    true,
						Description: "Minimum percentage of instances to keep during upgrades.",
					},
					"instance_close_delay_seconds": rschema.Int64Attribute{
						Optional:    true,
						Description: "Delay (seconds) before closing an instance during upgrades.",
					},
					"instance_restart_wait_seconds": rschema.Int64Attribute{
						Optional:    true,
						Description: "Wait duration (seconds) before restarting a failed instance.",
					},
				},
			},
			"stateful": rschema.SingleNestedAttribute{
				Optional:    true,
				Description: "Stateful service configuration. Required when service_kind is Stateful.",
				Attributes: map[string]rschema.Attribute{
					"target_replica_set_size": rschema.Int64Attribute{
						Optional:    true,
						Description: "Number of replicas for each partition.",
					},
					"min_replica_set_size": rschema.Int64Attribute{
						Optional:    true,
						Description: "Minimum replicas required for quorum.",
					},
					"has_persisted_state": rschema.BoolAttribute{
						Optional:    true,
						Description: "Indicates whether the service persists state.",
					},
					"replica_restart_wait_seconds": rschema.Int64Attribute{
						Optional:    true,
						Description: "Wait duration (seconds) before restarting a failed replica.",
					},
					"quorum_loss_wait_seconds": rschema.Int64Attribute{
						Optional:    true,
						Description: "Duration (seconds) to wait before declaring quorum loss.",
					},
					"standby_replica_keep_seconds": rschema.Int64Attribute{
						Optional:    true,
						Description: "Time (seconds) to keep standby replicas in the cluster.",
					},
					"service_placement_time_limit_seconds": rschema.Int64Attribute{
						Optional:    true,
						Description: "Maximum time (seconds) to wait for placement before aborting.",
					},
				},
			},
		},
	}
}

func (r *serviceResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok || data == nil {
		return
	}
	r.client = data.Client
}

func (r *serviceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	desc, diags := r.expandServiceDescription(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.CreateService(ctx, desc); err != nil {
		resp.Diagnostics.AddError("Failed to create service", err.Error())
		return
	}

	plan.ID = plan.Name
	if plan.HealthState.IsNull() || plan.HealthState.IsUnknown() {
		plan.HealthState = types.StringNull()
	}
	if plan.ServiceStatus.IsNull() || plan.ServiceStatus.IsUnknown() {
		plan.ServiceStatus = types.StringNull()
	}

	appName := applicationNameForModel(plan)
	info, err := r.client.GetService(ctx, appName, plan.Name.ValueString())
	if err == nil {
		r.applyInfoToState(&plan, info)
	} else if !servicefabric.IsNotFoundError(err) {
		resp.Diagnostics.AddError("Failed to read service after creation", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	appName := applicationNameForModel(state)
	info, err := r.client.GetService(ctx, appName, state.Name.ValueString())
	if err != nil {
		if servicefabric.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read service", err.Error())
		return
	}

	r.applyInfoToState(&state, info)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serviceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serviceResourceModel
	var state serviceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateDesc, changed, diags := r.buildUpdateDescription(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if changed {
		if err := r.client.UpdateService(ctx, plan.Name.ValueString(), updateDesc); err != nil {
			resp.Diagnostics.AddError("Failed to update service", err.Error())
			return
		}
	}

	appName := applicationNameForModel(plan)
	info, err := r.client.GetService(ctx, appName, plan.Name.ValueString())
	if err == nil {
		r.applyInfoToState(&plan, info)
	} else if !servicefabric.IsNotFoundError(err) {
		resp.Diagnostics.AddError("Failed to refresh service state", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serviceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	forceDelete, _ := boolValue(state.ForceRemove)
	if err := r.client.DeleteService(ctx, state.Name.ValueString(), forceDelete); err != nil {
		if servicefabric.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to delete service", err.Error())
		return
	}
}

func (r *serviceResource) applyInfoToState(state *serviceResourceModel, info *servicefabric.ServiceInfo) {
	state.ID = types.StringValue(info.Name)
	state.Name = types.StringValue(info.Name)
	if appName, err := deriveApplicationNameFromService(info.Name); err == nil {
		state.ApplicationName = types.StringValue(appName)
	}
	if info.TypeName != "" {
		state.ServiceTypeName = types.StringValue(info.TypeName)
	}
	if kind := serviceKindFromInfo(*info); kind != "" {
		state.ServiceKind = types.StringValue(kind)
	}
	if info.HealthState != "" {
		state.HealthState = types.StringValue(info.HealthState)
	} else {
		state.HealthState = types.StringNull()
	}
	if info.ServiceStatus != "" {
		state.ServiceStatus = types.StringValue(info.ServiceStatus)
	} else {
		state.ServiceStatus = types.StringNull()
	}
}

func (r *serviceResource) expandServiceDescription(ctx context.Context, plan serviceResourceModel) (any, diag.Diagnostics) {
	var diags diag.Diagnostics

	if plan.Partition.IsNull() || plan.Partition.IsUnknown() {
		diags.AddError("Missing partition configuration", "The partition block must be provided.")
		return nil, diags
	}

	partitionDesc, partDiags := expandPartitionDescription(ctx, plan.Partition)
	diags.Append(partDiags...)
	if diags.HasError() {
		return nil, diags
	}

	serviceKind, _ := stringValue(plan.ServiceKind)
	canonicalKind := canonicalServiceKind(serviceKind)
	if canonicalKind == "" {
		diags.AddError("Invalid service kind", "service_kind must be either Stateful or Stateless.")
		return nil, diags
	}

	base := servicefabric.ServiceDescription{
		ServiceKind:          canonicalKind,
		ApplicationName:      strings.TrimSpace(plan.ApplicationName.ValueString()),
		ServiceName:          strings.TrimSpace(plan.Name.ValueString()),
		ServiceTypeName:      strings.TrimSpace(plan.ServiceTypeName.ValueString()),
		PartitionDescription: *partitionDesc,
	}
	if v, ok := stringValue(plan.PlacementConstraints); ok {
		base.PlacementConstraints = v
	}
	if v, ok := stringValue(plan.DefaultMoveCost); ok {
		base.DefaultMoveCost = v
	}
	if v, ok := stringValue(plan.ServicePackageActivationMode); ok {
		base.ServicePackageActivationMode = v
	}
	if v, ok := stringValue(plan.ServiceDnsName); ok {
		base.ServiceDnsName = v
	}

	switch canonicalKind {
	case "Stateless":
		model, statelessDiags := decodeStatelessModel(ctx, plan.Stateless)
		diags.Append(statelessDiags...)
		if diags.HasError() {
			return nil, diags
		}
		if model == nil {
			diags.AddError("Missing stateless configuration", "stateless block must be provided when service_kind is Stateless.")
			return nil, diags
		}
		desc := &servicefabric.StatelessServiceDescription{
			ServiceDescription: base,
			InstanceCount:      -1,
		}
		if v, ok := int64Value(model.InstanceCount); ok {
			desc.InstanceCount = v
		}
		if v, ok := int64Value(model.MinInstanceCount); ok {
			desc.MinInstanceCount = &v
		}
		if v, ok := int64Value(model.MinInstancePercentage); ok {
			desc.MinInstancePercentage = &v
		}
		if str := secondsString(model.InstanceCloseDelaySeconds); str != nil {
			desc.InstanceCloseDelayDurationSeconds = str
		}
		if str := secondsString(model.InstanceRestartWaitSeconds); str != nil {
			desc.InstanceRestartWaitDurationSeconds = str
		}
		return desc, diags
	case "Stateful":
		model, statefulDiags := decodeStatefulModel(ctx, plan.Stateful)
		diags.Append(statefulDiags...)
		if diags.HasError() {
			return nil, diags
		}
		if model == nil {
			diags.AddError("Missing stateful configuration", "stateful block must be provided when service_kind is Stateful.")
			return nil, diags
		}
		targetSize, okTarget := int64Value(model.TargetReplicaSetSize)
		minSize, okMin := int64Value(model.MinReplicaSetSize)
		hasPersisted, okPersisted := boolValue(model.HasPersistedState)
		if !okTarget || !okMin || !okPersisted {
			diags.AddError("Incomplete stateful configuration", "target_replica_set_size, min_replica_set_size, and has_persisted_state must be specified.")
			return nil, diags
		}
		desc := &servicefabric.StatefulServiceDescription{
			ServiceDescription:   base,
			TargetReplicaSetSize: targetSize,
			MinReplicaSetSize:    minSize,
			HasPersistedState:    hasPersisted,
		}
		if str := secondsString(model.ReplicaRestartWaitSeconds); str != nil {
			desc.ReplicaRestartWaitDurationSeconds = str
		}
		if str := secondsString(model.QuorumLossWaitSeconds); str != nil {
			desc.QuorumLossWaitDurationSeconds = str
		}
		if str := secondsString(model.StandByReplicaKeepSeconds); str != nil {
			desc.StandByReplicaKeepDurationSeconds = str
		}
		if str := secondsString(model.ServicePlacementTimeLimitSeconds); str != nil {
			desc.ServicePlacementTimeLimitSeconds = str
		}
		return desc, diags
	default:
		diags.AddError("Unsupported service kind", fmt.Sprintf("service_kind %q is not supported", serviceKind))
		return nil, diags
	}
}

func (r *serviceResource) buildUpdateDescription(ctx context.Context, plan serviceResourceModel) (any, bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	kind, _ := stringValue(plan.ServiceKind)
	canonical := canonicalServiceKind(kind)
	switch canonical {
	case "Stateless":
		model, statelessDiags := decodeStatelessModel(ctx, plan.Stateless)
		diags.Append(statelessDiags...)
		if diags.HasError() {
			return nil, false, diags
		}
		desc := &servicefabric.StatelessServiceUpdateDescription{
			ServiceKind: "Stateless",
		}
		var flags uint32
		if v, ok := stringValue(plan.PlacementConstraints); ok {
			desc.PlacementConstraints = &v
			flags |= 0x0002
		}
		if v, ok := stringValue(plan.DefaultMoveCost); ok {
			desc.DefaultMoveCost = &v
			flags |= 0x0020
		}
		if v, ok := stringValue(plan.ServiceDnsName); ok {
			desc.ServiceDnsName = &v
			flags |= 0x0800
		}
		if model != nil {
			if v, ok := int64Value(model.InstanceCount); ok {
				desc.InstanceCount = &v
				flags |= 0x0001
			}
			if v, ok := int64Value(model.MinInstanceCount); ok {
				desc.MinInstanceCount = &v
				flags |= 0x0080
			}
			if v, ok := int64Value(model.MinInstancePercentage); ok {
				desc.MinInstancePercentage = &v
				flags |= 0x0100
			}
			if str := secondsString(model.InstanceCloseDelaySeconds); str != nil {
				desc.InstanceCloseDelayDurationSeconds = str
				flags |= 0x0200
			}
			if str := secondsString(model.InstanceRestartWaitSeconds); str != nil {
				desc.InstanceRestartWaitDurationSeconds = str
				flags |= 0x0400
			}
		}
		if flags == 0 {
			return nil, false, diags
		}
		desc.Flags = strconv.FormatUint(uint64(flags), 10)
		return desc, true, diags
	case "Stateful":
		model, statefulDiags := decodeStatefulModel(ctx, plan.Stateful)
		diags.Append(statefulDiags...)
		if diags.HasError() {
			return nil, false, diags
		}
		desc := &servicefabric.StatefulServiceUpdateDescription{
			ServiceKind: "Stateful",
		}
		var flags uint32
		if v, ok := stringValue(plan.PlacementConstraints); ok {
			desc.PlacementConstraints = &v
			flags |= 0x0020
		}
		if v, ok := stringValue(plan.DefaultMoveCost); ok {
			desc.DefaultMoveCost = &v
			flags |= 0x0200
		}
		if v, ok := stringValue(plan.ServiceDnsName); ok {
			desc.ServiceDnsName = &v
			flags |= 0x2000
		}
		if model != nil {
			if v, ok := int64Value(model.TargetReplicaSetSize); ok {
				desc.TargetReplicaSetSize = &v
				flags |= 0x0001
			}
			if v, ok := int64Value(model.MinReplicaSetSize); ok {
				desc.MinReplicaSetSize = &v
				flags |= 0x0010
			}
			if str := secondsString(model.ReplicaRestartWaitSeconds); str != nil {
				desc.ReplicaRestartWaitDurationSeconds = str
				flags |= 0x0002
			}
			if str := secondsString(model.QuorumLossWaitSeconds); str != nil {
				desc.QuorumLossWaitDurationSeconds = str
				flags |= 0x0004
			}
			if str := secondsString(model.StandByReplicaKeepSeconds); str != nil {
				desc.StandByReplicaKeepDurationSeconds = str
				flags |= 0x0008
			}
			if str := secondsString(model.ServicePlacementTimeLimitSeconds); str != nil {
				desc.ServicePlacementTimeLimitSeconds = str
				flags |= 0x0800
			}
		}
		if flags == 0 {
			return nil, false, diags
		}
		desc.Flags = strconv.FormatUint(uint64(flags), 10)
		return desc, true, diags
	default:
		return nil, false, diags
	}
}

func expandPartitionDescription(ctx context.Context, value types.Object) (*servicefabric.PartitionDescription, diag.Diagnostics) {
	var diags diag.Diagnostics
	var model partitionModel
	options := basetypes.ObjectAsOptions{UnhandledNullAsEmpty: true, UnhandledUnknownAsEmpty: true}
	diags.Append(value.As(ctx, &model, options)...)
	if diags.HasError() {
		return nil, diags
	}

	scheme, ok := stringValue(model.Scheme)
	if !ok {
		diags.AddError("Missing partition scheme", "partition.scheme must be specified.")
		return nil, diags
	}
	canonicalScheme := canonicalPartitionScheme(scheme)
	if canonicalScheme == "" {
		diags.AddError("Invalid partition scheme", fmt.Sprintf("Unsupported partition scheme %q.", scheme))
		return nil, diags
	}

	result := &servicefabric.PartitionDescription{
		PartitionScheme: canonicalScheme,
	}
	switch canonicalScheme {
	case "Singleton":
		if !model.Count.IsNull() && !model.Count.IsUnknown() {
			diags.AddError("Invalid partition configuration", "Singleton partitions cannot specify count.")
			return nil, diags
		}
	case "Named":
		var names []string
		if !model.Names.IsNull() && !model.Names.IsUnknown() {
			diags.Append(model.Names.ElementsAs(ctx, &names, false)...)
			if diags.HasError() {
				return nil, diags
			}
		}
		for i, name := range names {
			names[i] = strings.TrimSpace(name)
		}
		if len(names) == 0 {
			diags.AddError("Invalid partition configuration", "Named partitions require at least one entry in names.")
			return nil, diags
		}
		result.Names = names
		if count, ok := int64Value(model.Count); ok {
			result.Count = &count
		} else {
			v := int64(len(names))
			result.Count = &v
		}
	case "UniformInt64Range":
		low, okLow := int64Value(model.LowKey)
		high, okHigh := int64Value(model.HighKey)
		if !okLow || !okHigh {
			diags.AddError("Invalid partition configuration", "uniform_int64_range partitions require low_key and high_key.")
			return nil, diags
		}
		if high < low {
			diags.AddError("Invalid partition configuration", "high_key must be greater than or equal to low_key.")
			return nil, diags
		}
		result.LowKey = &low
		result.HighKey = &high
		if count, ok := int64Value(model.Count); ok {
			result.Count = &count
		}
	}
	return result, diags
}

func decodeStatelessModel(ctx context.Context, value types.Object) (*statelessServiceModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if value.IsNull() || value.IsUnknown() {
		return nil, diags
	}
	var model statelessServiceModel
	options := basetypes.ObjectAsOptions{UnhandledNullAsEmpty: true, UnhandledUnknownAsEmpty: true}
	diags.Append(value.As(ctx, &model, options)...)
	if diags.HasError() {
		return nil, diags
	}
	return &model, diags
}

func decodeStatefulModel(ctx context.Context, value types.Object) (*statefulServiceModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if value.IsNull() || value.IsUnknown() {
		return nil, diags
	}
	var model statefulServiceModel
	options := basetypes.ObjectAsOptions{UnhandledNullAsEmpty: true, UnhandledUnknownAsEmpty: true}
	diags.Append(value.As(ctx, &model, options)...)
	if diags.HasError() {
		return nil, diags
	}
	return &model, diags
}

func canonicalServiceKind(input string) string {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "stateless":
		return "Stateless"
	case "stateful":
		return "Stateful"
	default:
		return ""
	}
}

func canonicalPartitionScheme(input string) string {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "singleton":
		return "Singleton"
	case "uniformint64range", "uniform_int64_range":
		return "UniformInt64Range"
	case "named":
		return "Named"
	default:
		return ""
	}
}

func secondsString(value types.Int64) *string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	sec := value.ValueInt64()
	s := fmt.Sprintf("%d", sec)
	return &s
}

func applicationNameForModel(model serviceResourceModel) string {
	if v, ok := stringValue(model.ApplicationName); ok {
		return v
	}
	if model.Name.IsNull() || model.Name.IsUnknown() {
		return ""
	}
	appName, err := deriveApplicationNameFromService(model.Name.ValueString())
	if err != nil {
		return ""
	}
	return appName
}
