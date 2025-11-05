package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
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
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Version    types.String `tfsdk:"version"`
	PackageURI types.String `tfsdk:"package_uri"`
	Status     types.String `tfsdk:"status"`
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"status": rschema.StringAttribute{
				Computed:    true,
				Description: "Provisioning status reported by the cluster.",
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
	return nil
}

func (r *applicationTypeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All mutable attributes are ForceNew; nothing to do.
	var state applicationTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *applicationTypeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state applicationTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.UnprovisionApplicationType(ctx, state.Name.ValueString(), state.Version.ValueString(), false)
	if err != nil {
		if servicefabric.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to unprovision application type", err.Error())
		return
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
