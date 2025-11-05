package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	stringplanmodifier "github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/williamoconnorme/terraform-provider-servicefabric/internal/servicefabric"
)

var _ resource.Resource = &applicationResource{}
var _ resource.ResourceWithImportState = &applicationResource{}

type applicationResource struct {
	client *servicefabric.Client
}

type applicationResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	TypeName    types.String `tfsdk:"type_name"`
	TypeVersion types.String `tfsdk:"type_version"`
	Parameters  types.Map    `tfsdk:"parameters"`
	Status      types.String `tfsdk:"status"`
	HealthState types.String `tfsdk:"health_state"`
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
	r.client = req.ProviderData.(*servicefabric.Client)
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

	if err := r.client.CreateApplication(ctx, desc); err != nil {
		resp.Diagnostics.AddError("Failed to create application", err.Error())
		return
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

	return nil
}
