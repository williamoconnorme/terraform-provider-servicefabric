package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	stringplanmodifier "github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/williamoconnorme/terraform-provider-servicefabric/internal/servicefabric"
)

var _ resource.Resource = &applicationResource{}
var _ resource.ResourceWithImportState = &applicationResource{}

var (
	applicationMetricAttrTypes = map[string]attr.Type{
		"name":                     types.StringType,
		"maximum_capacity":         types.Int64Type,
		"reservation_capacity":     types.Int64Type,
		"total_application_capacity": types.Int64Type,
	}
	applicationCapacityAttrTypes = map[string]attr.Type{
		"minimum_nodes":       types.Int64Type,
		"maximum_nodes":       types.Int64Type,
		"application_metrics": types.ListType{ElemType: types.ObjectType{AttrTypes: applicationMetricAttrTypes}},
	}
	managedApplicationIdentityAttrTypes = map[string]attr.Type{
		"token_service_endpoint": types.StringType,
		"identities":             types.ListType{ElemType: types.StringType},
	}
	guidRegex = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

type applicationResource struct {
	client   *servicefabric.Client
	features providerFeatures
}

type applicationResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	TypeName    types.String `tfsdk:"type_name"`
	TypeVersion types.String `tfsdk:"type_version"`
	Parameters  types.Map    `tfsdk:"parameters"`
	Status      types.String `tfsdk:"status"`
	HealthState types.String `tfsdk:"health_state"`
	ApplicationCapacity         types.Object `tfsdk:"application_capacity"`
	ManagedApplicationIdentity  types.Object `tfsdk:"managed_application_identity"`
}

type applicationCapacityModel struct {
	MinimumNodes       types.Int64                 `tfsdk:"minimum_nodes"`
	MaximumNodes       types.Int64                 `tfsdk:"maximum_nodes"`
	ApplicationMetrics []applicationMetricModel    `tfsdk:"application_metrics"`
}

type applicationMetricModel struct {
	Name                   types.String `tfsdk:"name"`
	MaximumCapacity        types.Int64  `tfsdk:"maximum_capacity"`
	ReservationCapacity    types.Int64  `tfsdk:"reservation_capacity"`
	TotalApplicationCapacity types.Int64 `tfsdk:"total_application_capacity"`
}

type managedApplicationIdentityModel struct {
	TokenServiceEndpoint types.String                `tfsdk:"token_service_endpoint"`
	Identities           types.List                  `tfsdk:"identities"`
}

func NewApplicationResource() resource.Resource {
	return &applicationResource{}
}

func (r *applicationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application"
}

func (r *applicationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = rschema.Schema{
		Attributes: map[string]rschema.Attribute{
			"id": rschema.StringAttribute{
				Computed:      true,
				Description:   "Application identifier in the format \"{type_name}|{application_name}\".",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": rschema.StringAttribute{
				Required:    true,
				Description: "Fully-qualified Service Fabric application name, e.g. fabric:/MyApp.",
			},
			"type_name": rschema.StringAttribute{
				Required:    true,
				Description: "Application type name to deploy.",
			},
			"type_version": rschema.StringAttribute{
				Required:    true,
				Description: "Application type version to deploy.",
			},
		"parameters": rschema.MapAttribute{
			Optional:    true,
			ElementType: types.StringType,
			Description: "Application parameters supplied to the deployment.",
		},
		"application_capacity": rschema.SingleNestedAttribute{
			Optional:    true,
			Description: "Application capacity settings used to reserve and limit cluster resources.",
			Attributes: map[string]rschema.Attribute{
				"minimum_nodes": rschema.Int64Attribute{
					Optional:    true,
					Description: "Minimum number of nodes where the application will reserve capacity.",
				},
				"maximum_nodes": rschema.Int64Attribute{
					Optional:    true,
					Description: "Maximum number of nodes where the application can reserve capacity (0 means unlimited).",
				},
				"application_metrics": rschema.ListNestedAttribute{
					Optional:    true,
					Description: "Application metric capacity settings applied across the cluster.",
					NestedObject: rschema.NestedAttributeObject{
						Attributes: map[string]rschema.Attribute{
							"name": rschema.StringAttribute{
								Required:    true,
								Description: "Metric name.",
							},
							"maximum_capacity": rschema.Int64Attribute{
								Optional:    true,
								Description: "Maximum capacity per node for this metric (0 means unlimited).",
							},
							"reservation_capacity": rschema.Int64Attribute{
								Optional:    true,
								Description: "Reserved capacity per node for this metric.",
							},
							"total_application_capacity": rschema.Int64Attribute{
								Optional:    true,
								Description: "Total capacity for this metric across the application (0 means unlimited).",
							},
						},
					},
				},
			},
		},
		"managed_application_identity": rschema.SingleNestedAttribute{
			Optional:    true,
			Description: "Configures managed identities attached to the Service Fabric application.",
			Attributes: map[string]rschema.Attribute{
				"token_service_endpoint": rschema.StringAttribute{
					Optional:    true,
					Description: "Token service endpoint used for identity propagation.",
				},
				"identities": rschema.ListAttribute{
					Optional:    true,
					ElementType: types.StringType,
					Description: "List of managed identity names or principal IDs (GUIDs).",
				},
			},
		},
		"status": rschema.StringAttribute{
			Computed:    true,
			Description: "Current application status.",
		},
			"health_state": rschema.StringAttribute{
				Computed:    true,
				Description: "Cluster-reported health state.",
			},
		},
	}
}

func (r *applicationResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok || data == nil {
		return
	}
	r.client = data.Client
	r.features = data.Features
}

func (r *applicationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan applicationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	paramMap := map[string]string{}
	if !plan.Parameters.IsNull() {
		diag := plan.Parameters.ElementsAs(ctx, &paramMap, false)
		resp.Diagnostics.Append(diag...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	desc := servicefabric.ApplicationDescription{
		Name:         plan.Name.ValueString(),
		TypeName:     plan.TypeName.ValueString(),
		TypeVersion:  plan.TypeVersion.ValueString(),
		ParameterMap: paramMap,
	}

	capDesc, capDiags := expandApplicationCapacity(ctx, plan.ApplicationCapacity)
	resp.Diagnostics.Append(capDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if capDesc != nil {
		desc.ApplicationCapacity = capDesc
	}

	identityDesc, identityDiags := expandManagedApplicationIdentity(ctx, plan.ManagedApplicationIdentity)
	resp.Diagnostics.Append(identityDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if identityDesc != nil {
		desc.ManagedApplicationIdentity = identityDesc
	}

	if err := r.client.CreateApplication(ctx, desc); err != nil {
		if r.features.ApplicationRecreateOnUpgrade && servicefabric.IsApplicationAlreadyExistsError(err) {
			tflog.Info(ctx, "Existing Service Fabric application detected, initiating upgrade instead of create", map[string]any{
				"name":         plan.Name.ValueString(),
				"type_name":    plan.TypeName.ValueString(),
				"type_version": plan.TypeVersion.ValueString(),
			})
			upgradeDesc := servicefabric.ApplicationUpgradeDescription{
				Name:                         plan.Name.ValueString(),
				TargetApplicationTypeVersion: plan.TypeVersion.ValueString(),
				ParameterMap:                 paramMap,
				ForceRestart:                 true,
			}
			if upgradeErr := r.client.UpgradeApplication(ctx, upgradeDesc); upgradeErr != nil {
				resp.Diagnostics.AddError("Failed to upgrade existing application", upgradeErr.Error())
				return
			}
		} else {
			resp.Diagnostics.AddError("Failed to create application", err.Error())
			return
		}
	}

	tflog.Info(ctx, "Created Service Fabric application", map[string]any{
		"name":         plan.Name.ValueString(),
		"type_name":    plan.TypeName.ValueString(),
		"type_version": plan.TypeVersion.ValueString(),
	})

	plan.ID = types.StringValue(applicationCompositeID(plan.TypeName.ValueString(), plan.Name.ValueString()))

	if err := r.refreshState(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Failed to read application", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *applicationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state applicationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.refreshState(ctx, &state); err != nil {
		if servicefabric.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read application", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *applicationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan applicationResourceModel
	var state applicationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.TypeName.ValueString() != state.TypeName.ValueString() {
		resp.Diagnostics.AddError(
			"Changing application type name requires replacement",
			"Modify the resource definition to recreate the application when switching type_name.",
		)
		return
	}

	if plan.Name.IsNull() || plan.Name.ValueString() == "" {
		plan.Name = state.Name
	}
	if plan.TypeName.IsNull() || plan.TypeName.ValueString() == "" {
		plan.TypeName = state.TypeName
	}

	planCapacity, planCapDiags := expandApplicationCapacity(ctx, plan.ApplicationCapacity)
	resp.Diagnostics.Append(planCapDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	stateCapacity, stateCapDiags := expandApplicationCapacity(ctx, state.ApplicationCapacity)
	resp.Diagnostics.Append(stateCapDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !applicationCapacityEqual(planCapacity, stateCapacity) {
		resp.Diagnostics.AddAttributeError(
			path.Root("application_capacity"),
			"Application capacity changes require recreation",
			"Updating application_capacity for an existing Service Fabric application is not supported. Please recreate the resource to apply changes.",
		)
		return
	}

	planIdentity, planIdentityDiags := expandManagedApplicationIdentity(ctx, plan.ManagedApplicationIdentity)
	resp.Diagnostics.Append(planIdentityDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	stateIdentity, stateIdentityDiags := expandManagedApplicationIdentity(ctx, state.ManagedApplicationIdentity)
	resp.Diagnostics.Append(stateIdentityDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !managedApplicationIdentityEqual(planIdentity, stateIdentity) {
		resp.Diagnostics.AddAttributeError(
			path.Root("managed_application_identity"),
			"Managed application identity changes require recreation",
			"Updating managed_application_identity for an existing Service Fabric application is not supported. Please recreate the resource to apply changes.",
		)
		return
	}

	stateParams := map[string]string{}
	if !state.Parameters.IsNull() && !state.Parameters.IsUnknown() {
		diag := state.Parameters.ElementsAs(ctx, &stateParams, false)
		resp.Diagnostics.Append(diag...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	planParams := map[string]string{}
	if !plan.Parameters.IsNull() && !plan.Parameters.IsUnknown() {
		diag := plan.Parameters.ElementsAs(ctx, &planParams, false)
		resp.Diagnostics.Append(diag...)
		if resp.Diagnostics.HasError() {
			return
		}
	} else {
		planParams = stateParams
	}

	versionChanged := plan.TypeVersion.ValueString() != state.TypeVersion.ValueString()
	parametersChanged := !stringMapEqual(planParams, stateParams)

	if !versionChanged && !parametersChanged {
		if err := r.refreshState(ctx, &plan); err != nil {
			resp.Diagnostics.AddError("Failed to read application", err.Error())
			return
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		return
	}

	upgradeDesc := servicefabric.ApplicationUpgradeDescription{
		Name:                         plan.Name.ValueString(),
		TargetApplicationTypeVersion: plan.TypeVersion.ValueString(),
		ParameterMap:                 planParams,
	}

	tflog.Info(ctx, "Starting Service Fabric application upgrade", map[string]any{
		"name":              plan.Name.ValueString(),
		"type_version":      plan.TypeVersion.ValueString(),
		"type_name":         plan.TypeName.ValueString(),
		"parametersChanged": parametersChanged,
		"versionChanged":    versionChanged,
	})

	if err := r.client.UpgradeApplication(ctx, upgradeDesc); err != nil {
		resp.Diagnostics.AddError("Failed to upgrade application", err.Error())
		return
	}

	if err := r.refreshState(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Failed to read application", err.Error())
		return
	}

	plan.ID = types.StringValue(applicationCompositeID(plan.TypeName.ValueString(), plan.Name.ValueString()))

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *applicationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state applicationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteApplication(ctx, state.Name.ValueString(), false); err != nil {
		if servicefabric.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to delete application", err.Error())
	}
}

func (r *applicationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if id == "" {
		resp.Diagnostics.AddError("Missing identifier", "Import requires an application name.")
		return
	}
	typeName, appName, ok := splitApplicationCompositeID(id)
	if !ok {
		appName = id
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), applicationCompositeID(typeName, appName))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), appName)...)
	if ok {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type_name"), typeName)...)
	}
}

func expandApplicationCapacity(ctx context.Context, value types.Object) (*servicefabric.ApplicationCapacityDescription, diag.Diagnostics) {
	var diags diag.Diagnostics
	if value.IsNull() || value.IsUnknown() {
		return nil, diags
	}
	var model applicationCapacityModel
	options := basetypes.ObjectAsOptions{UnhandledNullAsEmpty: true, UnhandledUnknownAsEmpty: true}
	diags.Append(value.As(ctx, &model, options)...)
	if diags.HasError() {
		return nil, diags
	}

	result := &servicefabric.ApplicationCapacityDescription{}
	hasValue := false
	if !model.MinimumNodes.IsNull() && !model.MinimumNodes.IsUnknown() {
		v := model.MinimumNodes.ValueInt64()
		result.MinimumNodes = &v
		hasValue = true
	}
	if !model.MaximumNodes.IsNull() && !model.MaximumNodes.IsUnknown() {
		v := model.MaximumNodes.ValueInt64()
		result.MaximumNodes = &v
		hasValue = true
	}
	for _, metric := range model.ApplicationMetrics {
		if metric.Name.IsNull() || metric.Name.IsUnknown() {
			continue
		}
		name := metric.Name.ValueString()
		if name == "" {
			continue
		}
		m := servicefabric.ApplicationMetricDescription{Name: name}
		if !metric.MaximumCapacity.IsNull() && !metric.MaximumCapacity.IsUnknown() {
			v := metric.MaximumCapacity.ValueInt64()
			m.MaximumCapacity = &v
		}
		if !metric.ReservationCapacity.IsNull() && !metric.ReservationCapacity.IsUnknown() {
			v := metric.ReservationCapacity.ValueInt64()
			m.ReservationCapacity = &v
		}
		if !metric.TotalApplicationCapacity.IsNull() && !metric.TotalApplicationCapacity.IsUnknown() {
			v := metric.TotalApplicationCapacity.ValueInt64()
			m.TotalApplicationCapacity = &v
		}
		result.ApplicationMetrics = append(result.ApplicationMetrics, m)
	}
	if len(result.ApplicationMetrics) > 0 {
		hasValue = true
	}
	if !hasValue {
		return nil, diags
	}
	return result, diags
}

func expandManagedApplicationIdentity(ctx context.Context, value types.Object) (*servicefabric.ManagedApplicationIdentityDescription, diag.Diagnostics) {
	var diags diag.Diagnostics
	if value.IsNull() || value.IsUnknown() {
		return nil, diags
	}
	var model managedApplicationIdentityModel
	options := basetypes.ObjectAsOptions{UnhandledNullAsEmpty: true, UnhandledUnknownAsEmpty: true}
	diags.Append(value.As(ctx, &model, options)...)
	if diags.HasError() {
		return nil, diags
	}

	result := &servicefabric.ManagedApplicationIdentityDescription{}
	hasValue := false
	if !model.TokenServiceEndpoint.IsNull() && !model.TokenServiceEndpoint.IsUnknown() {
		result.TokenServiceEndpoint = model.TokenServiceEndpoint.ValueString()
		hasValue = true
	}
	var identityInputs []string
	if !model.Identities.IsNull() && !model.Identities.IsUnknown() {
		identityDiags := model.Identities.ElementsAs(ctx, &identityInputs, false)
		diags.Append(identityDiags...)
		if diags.HasError() {
			return nil, diags
		}
	}
	for _, raw := range identityInputs {
		candidate := strings.TrimSpace(raw)
		if candidate == "" {
			continue
		}
		entry := servicefabric.ManagedApplicationIdentity{}
		if isGUID(candidate) {
			entry.PrincipalID = candidate
		} else {
			entry.Name = candidate
		}
		result.IdentityRefs = append(result.IdentityRefs, entry)
	}
	if len(result.IdentityRefs) > 0 {
		hasValue = true
	}
	if !hasValue {
		return nil, diags
	}
	return result, diags
}

func flattenApplicationCapacity(_ context.Context, cap *servicefabric.ApplicationCapacityDescription) (types.Object, diag.Diagnostics) {
	metricsType := types.ObjectType{AttrTypes: applicationMetricAttrTypes}
	if cap == nil {
		return types.ObjectNull(applicationCapacityAttrTypes), nil
	}
	attrs := map[string]attr.Value{
		"minimum_nodes":       types.Int64Null(),
		"maximum_nodes":       types.Int64Null(),
		"application_metrics": types.ListNull(metricsType),
	}
	hasValue := false
	if cap.MinimumNodes != nil {
		attrs["minimum_nodes"] = types.Int64Value(*cap.MinimumNodes)
		hasValue = true
	}
	if cap.MaximumNodes != nil {
		attrs["maximum_nodes"] = types.Int64Value(*cap.MaximumNodes)
		hasValue = true
	}
	metricValues := make([]attr.Value, 0, len(cap.ApplicationMetrics))
	for _, metric := range cap.ApplicationMetrics {
		if metric.Name == "" {
			continue
		}
		metricHasValue := true
		metricAttrs := map[string]attr.Value{
			"name":                     types.StringValue(metric.Name),
			"maximum_capacity":         types.Int64Null(),
			"reservation_capacity":     types.Int64Null(),
			"total_application_capacity": types.Int64Null(),
		}
		if metric.MaximumCapacity != nil {
			metricAttrs["maximum_capacity"] = types.Int64Value(*metric.MaximumCapacity)
			metricHasValue = true
		}
		if metric.ReservationCapacity != nil {
			metricAttrs["reservation_capacity"] = types.Int64Value(*metric.ReservationCapacity)
			metricHasValue = true
		}
		if metric.TotalApplicationCapacity != nil {
			metricAttrs["total_application_capacity"] = types.Int64Value(*metric.TotalApplicationCapacity)
			metricHasValue = true
		}
		metricValue, metricDiags := types.ObjectValue(applicationMetricAttrTypes, metricAttrs)
		if metricDiags.HasError() {
			return types.ObjectNull(applicationCapacityAttrTypes), metricDiags
		}
		metricValues = append(metricValues, metricValue)
		if metricHasValue {
			hasValue = true
		}
	}
	if len(metricValues) > 0 {
		hasValue = true
	}
	if !hasValue {
		return types.ObjectNull(applicationCapacityAttrTypes), nil
	}
	metricList, metricListDiags := types.ListValue(metricsType, metricValues)
	if metricListDiags.HasError() {
		return types.ObjectNull(applicationCapacityAttrTypes), metricListDiags
	}
	attrs["application_metrics"] = metricList

obj, objDiags := types.ObjectValue(applicationCapacityAttrTypes, attrs)
if objDiags.HasError() {
	return types.ObjectNull(applicationCapacityAttrTypes), objDiags
}
return obj, nil
}

func flattenManagedApplicationIdentity(_ context.Context, identity *servicefabric.ManagedApplicationIdentityDescription) (types.Object, diag.Diagnostics) {
	if identity == nil {
		return types.ObjectNull(managedApplicationIdentityAttrTypes), nil
	}
	attrs := map[string]attr.Value{
		"token_service_endpoint": types.StringNull(),
		"identities":             types.ListNull(types.StringType),
	}
	hasValue := false
	if identity.TokenServiceEndpoint != "" {
		attrs["token_service_endpoint"] = types.StringValue(identity.TokenServiceEndpoint)
		hasValue = true
	}
	identities := make([]attr.Value, 0, len(identity.IdentityRefs))
	for _, item := range identity.IdentityRefs {
		repr := identityReferenceToString(item)
		if repr == "" {
			continue
		}
		identities = append(identities, types.StringValue(repr))
		hasValue = true
	}
	if !hasValue {
		return types.ObjectNull(managedApplicationIdentityAttrTypes), nil
	}
	listValue, listDiags := types.ListValue(types.StringType, identities)
	if listDiags.HasError() {
		return types.ObjectNull(managedApplicationIdentityAttrTypes), listDiags
	}
	attrs["identities"] = listValue
	incoming, incomingDiags := types.ObjectValue(managedApplicationIdentityAttrTypes, attrs)
	if incomingDiags.HasError() {
		return types.ObjectNull(managedApplicationIdentityAttrTypes), incomingDiags
	}
	return incoming, nil
}

func int64PointerValue(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*v)
}

func firstDiagnosticError(diags diag.Diagnostics) string {
	for _, d := range diags {
		if d.Severity() == diag.SeverityError {
			if d.Detail() != "" {
				return d.Detail()
			}
			return d.Summary()
		}
	}
	return "unknown error"
}

func applicationCapacityEqual(a, b *servicefabric.ApplicationCapacityDescription) bool {
	if a == nil && b == nil {
		return true
	}
	if (a == nil) != (b == nil) {
		return false
	}
	if !int64PtrEqual(a.MinimumNodes, b.MinimumNodes) {
		return false
	}
	if !int64PtrEqual(a.MaximumNodes, b.MaximumNodes) {
		return false
	}
	if len(a.ApplicationMetrics) != len(b.ApplicationMetrics) {
		return false
	}
	metricsA := make(map[string]servicefabric.ApplicationMetricDescription, len(a.ApplicationMetrics))
	for _, metric := range a.ApplicationMetrics {
		metricsA[metric.Name] = metric
	}
	for _, metricB := range b.ApplicationMetrics {
		metricA, ok := metricsA[metricB.Name]
		if !ok {
			return false
		}
		if !int64PtrEqual(metricA.MaximumCapacity, metricB.MaximumCapacity) {
			return false
		}
		if !int64PtrEqual(metricA.ReservationCapacity, metricB.ReservationCapacity) {
			return false
		}
		if !int64PtrEqual(metricA.TotalApplicationCapacity, metricB.TotalApplicationCapacity) {
			return false
		}
	}
	return true
}

func managedApplicationIdentityEqual(a, b *servicefabric.ManagedApplicationIdentityDescription) bool {
	if a == nil && b == nil {
		return true
	}
	if (a == nil) != (b == nil) {
		return false
	}
	if a.TokenServiceEndpoint != b.TokenServiceEndpoint {
		return false
	}
	if len(a.IdentityRefs) != len(b.IdentityRefs) {
		return false
	}
	used := make([]bool, len(b.IdentityRefs))
	for _, ia := range a.IdentityRefs {
		matched := false
		for j, ib := range b.IdentityRefs {
			if used[j] {
				continue
			}
			if identityRefsEqual(ia, ib) {
				used[j] = true
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func int64PtrEqual(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if (a == nil) != (b == nil) {
		return false
	}
	return *a == *b
}

func identityReferenceToString(identity servicefabric.ManagedApplicationIdentity) string {
	if identity.Name != "" {
		return identity.Name
	}
	return identity.PrincipalID
}

func identityRefsEqual(a, b servicefabric.ManagedApplicationIdentity) bool {
	switch {
	case a.PrincipalID != "" && b.PrincipalID != "":
		return strings.EqualFold(a.PrincipalID, b.PrincipalID)
	case a.Name != "" && b.Name != "":
		return a.Name == b.Name
	default:
		return strings.EqualFold(a.PrincipalID, b.PrincipalID) && a.Name == b.Name
	}
}

func isGUID(v string) bool {
	return guidRegex.MatchString(v)
}

func (r *applicationResource) refreshState(ctx context.Context, state *applicationResourceModel) error {
	info, err := r.client.GetApplication(ctx, state.Name.ValueString())
	if err != nil {
		return err
	}

	state.Name = types.StringValue(info.Name)
	state.TypeName = types.StringValue(info.TypeName)
	state.TypeVersion = types.StringValue(info.TypeVersion)
	state.Status = types.StringValue(info.Status)
	state.HealthState = types.StringValue(info.HealthState)
	state.ID = types.StringValue(applicationCompositeID(info.TypeName, info.Name))

	params := servicefabric.ParameterListToMap(info.ParameterEntries())
	state.Parameters = types.MapValueMust(types.StringType, convertStringMapToAttrValues(params))

	state.ApplicationCapacity = types.ObjectNull(applicationCapacityAttrTypes)
	if info.ApplicationCapacity != nil {
		capVal, diags := flattenApplicationCapacity(ctx, info.ApplicationCapacity)
		if diags.HasError() {
			return fmt.Errorf("failed to decode application capacity: %s", firstDiagnosticError(diags))
		}
		state.ApplicationCapacity = capVal
	}

	state.ManagedApplicationIdentity = types.ObjectNull(managedApplicationIdentityAttrTypes)
	if info.ManagedApplicationIdentity != nil {
		identityVal, diags := flattenManagedApplicationIdentity(ctx, info.ManagedApplicationIdentity)
		if diags.HasError() {
			return fmt.Errorf("failed to decode managed application identity: %s", firstDiagnosticError(diags))
		}
		state.ManagedApplicationIdentity = identityVal
	}

	return nil
}
