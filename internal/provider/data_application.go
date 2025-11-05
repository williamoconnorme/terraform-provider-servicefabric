package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/williamoconnorme/terraform-provider-servicefabric/internal/servicefabric"
)

var _ datasource.DataSource = &applicationDataSource{}

type applicationDataSource struct {
	client *servicefabric.Client
}

type applicationDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	TypeName    types.String `tfsdk:"type_name"`
	TypeVersion types.String `tfsdk:"type_version"`
	Parameters  types.Map    `tfsdk:"parameters"`
	Status      types.String `tfsdk:"status"`
	HealthState types.String `tfsdk:"health_state"`
}

func NewApplicationDataSource() datasource.DataSource {
	return &applicationDataSource{}
}

func (d *applicationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application"
}

func (d *applicationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Application identifier.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Service Fabric application name.",
			},
			"type_name": schema.StringAttribute{
				Computed:    true,
				Description: "Application type name.",
			},
			"type_version": schema.StringAttribute{
				Computed:    true,
				Description: "Application type version.",
			},
			"parameters": schema.MapAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "Application parameters supplied during deployment.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Current application status.",
			},
			"health_state": schema.StringAttribute{
				Computed:    true,
				Description: "Health state reported by the cluster.",
			},
		},
	}
}

func (d *applicationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	d.client = req.ProviderData.(*servicefabric.Client)
}

func (d *applicationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state applicationDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	info, err := d.client.GetApplication(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read application", err.Error())
		return
	}

	state.ID = types.StringValue(info.Name)
	state.TypeName = types.StringValue(info.TypeName)
	state.TypeVersion = types.StringValue(info.TypeVersion)
	state.Status = types.StringValue(info.Status)
	state.HealthState = types.StringValue(info.HealthState)
	state.Parameters = types.MapValueMust(types.StringType, convertStringMapToAttrValues(servicefabric.ParameterListToMap(info.ParameterEntries())))

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
