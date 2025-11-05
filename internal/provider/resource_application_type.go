package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	stringplanmodifier "github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/williamoconnorme/terraform-provider-servicefabric/internal/servicefabric"
)

var _ resource.Resource = &applicationTypeResource{}
var _ resource.ResourceWithImportState = &applicationTypeResource{}

type applicationTypeResource struct {
	client *servicefabric.Client
}

type applicationTypeResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Version        types.String `tfsdk:"version"`
	PackageURI     types.String `tfsdk:"package_uri"`
	Status         types.String `tfsdk:"status"`
	RetainVersions types.Bool   `tfsdk:"retain_versions"`
}

func NewApplicationTypeResource() resource.Resource {
	return &applicationTypeResource{}
}

func (r *applicationTypeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application_type"
}

func (r *applicationTypeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = rschema.Schema{
		Attributes: map[string]rschema.Attribute{
			"id": rschema.StringAttribute{
				Computed:      true,
				Description:   "Unique identifier in the format \"{name}/{version}\".",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": rschema.StringAttribute{
				Required:    true,
				Description: "Application type name as registered in the cluster.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"version": rschema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				Description: "Application type version.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"package_uri": rschema.StringAttribute{
				Required:    true,
				Description: "Service Fabric package URI (SAS URL) pointing to the SFPKG.",
			},
			"status": rschema.StringAttribute{
				Computed:    true,
				Description: "Provisioning status reported by the cluster.",
			},
			"retain_versions": rschema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "When true, previously provisioned versions are retained in the cluster instead of being unprovisioned on destroy.",
			},
		},
	}
}

func (r *applicationTypeResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(*servicefabric.Client)
}

func (r *applicationTypeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan applicationTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.RetainVersions.IsNull() || plan.RetainVersions.IsUnknown() {
		plan.RetainVersions = types.BoolValue(false)
	}

	if err := r.client.ProvisionApplicationType(ctx, plan.Name.ValueString(), plan.Version.ValueString(), plan.PackageURI.ValueString()); err != nil {
		resp.Diagnostics.AddError("Provisioning failed", err.Error())
		return
	}

	tflog.Info(ctx, "Provisioned Service Fabric application type", map[string]any{
		"name":    plan.Name.ValueString(),
		"version": plan.Version.ValueString(),
	})

	plan.ID = types.StringValue(fmt.Sprintf("%s/%s", plan.Name.ValueString(), plan.Version.ValueString()))

	if err := r.readIntoState(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Failed to read application type", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *applicationTypeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state applicationTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.readIntoState(ctx, &state); err != nil {
		if servicefabric.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read application type", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *applicationTypeResource) readIntoState(ctx context.Context, state *applicationTypeResourceModel) error {
	info, err := r.client.GetApplicationTypeVersion(ctx, state.Name.ValueString(), state.Version.ValueString())
	if err != nil {
		return err
	}
	state.Status = types.StringValue(info.Status)
	if state.ID.IsNull() || state.ID.ValueString() == "" {
		state.ID = types.StringValue(fmt.Sprintf("%s/%s", state.Name.ValueString(), state.Version.ValueString()))
	}
	if state.RetainVersions.IsNull() || state.RetainVersions.IsUnknown() {
		state.RetainVersions = types.BoolValue(false)
	}
	return nil
}

func (r *applicationTypeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan applicationTypeResourceModel
	var state applicationTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	versionChanged := plan.Version.ValueString() != state.Version.ValueString()
	packageChanged := plan.PackageURI.ValueString() != state.PackageURI.ValueString()

	if versionChanged {
		resp.Diagnostics.AddError(
			"Application type version change requires replacement",
			"Terraform planned an in-place update but version changes are handled via resource replacement. Set `lifecycle { create_before_destroy = true }` if you need zero-downtime upgrades.",
		)
		return
	}

	if !versionChanged && !packageChanged {
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		return
	}

	if err := r.client.ProvisionApplicationType(ctx, plan.Name.ValueString(), plan.Version.ValueString(), plan.PackageURI.ValueString()); err != nil {
		resp.Diagnostics.AddError("Provisioning failed", err.Error())
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s/%s", plan.Name.ValueString(), plan.Version.ValueString()))

	if err := r.readIntoState(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Failed to read application type", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *applicationTypeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state applicationTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	retain := true
	if !state.RetainVersions.IsNull() && !state.RetainVersions.IsUnknown() {
		retain = state.RetainVersions.ValueBool()
	}
	if retain {
		tflog.Info(ctx, "Retaining Service Fabric application type version per configuration", map[string]any{
			"name":    state.Name.ValueString(),
			"version": state.Version.ValueString(),
		})
		return
	}

	err := r.client.UnprovisionApplicationType(ctx, state.Name.ValueString(), state.Version.ValueString(), false)
	if err != nil {
		switch {
		case servicefabric.IsNotFoundError(err):
			return
		case servicefabric.IsApplicationTypeInUseError(err):
			resp.Diagnostics.AddWarning(
				"Application type still in use",
				fmt.Sprintf("Skipped unprovisioning %s/%s because it is still referenced by an application. Service Fabric will retain older versions until no longer needed.", state.Name.ValueString(), state.Version.ValueString()),
			)
		default:
			resp.Diagnostics.AddError("Failed to unprovision application type", err.Error())
		}
	}
}

func (r *applicationTypeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Unexpected import identifier", "Expected identifier in the format name/version.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("version"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
