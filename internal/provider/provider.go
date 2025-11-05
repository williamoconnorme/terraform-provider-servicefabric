package provider

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/williamoconnorme/terraform-provider-servicefabric/internal/servicefabric"
)

// Ensure provider satisfies the interface.
var _ provider.Provider = &serviceFabricProvider{}

// New instantiates the Service Fabric provider.
func New() provider.Provider {
	return &serviceFabricProvider{}
}

// serviceFabricProviderModel defines the provider configuration model.
type serviceFabricProviderModel struct {
	Endpoint                  types.String `tfsdk:"endpoint"`
	SkipTLSVerify             types.Bool   `tfsdk:"skip_tls_verify"`
	AuthType                  types.String `tfsdk:"auth_type"`
	ClusterApplicationID      types.String `tfsdk:"cluster_application_id"`
	TenantID                  types.String `tfsdk:"tenant_id"`
	ClientID                  types.String `tfsdk:"client_id"`
	ClientSecret              types.String `tfsdk:"client_secret"`
	ClientCertificatePath     types.String `tfsdk:"client_certificate_path"`
	ClientCertificatePassword types.String `tfsdk:"client_certificate_password"`
}

type serviceFabricProvider struct{}

// Metadata returns the provider type name.
func (p *serviceFabricProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "servicefabric"
}

// Schema defines provider-level configuration options.
func (p *serviceFabricProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = providerschema.Schema{
		Attributes: map[string]providerschema.Attribute{
			"endpoint": providerschema.StringAttribute{
				Required:    true,
				Description: "Service Fabric cluster HTTPS management endpoint, e.g. https://cluster:19080.",
			},
			"skip_tls_verify": providerschema.BoolAttribute{
				Optional:    true,
				Description: "Skip verification of the server's TLS certificate. Use only for development.",
			},
			"auth_type": providerschema.StringAttribute{
				Optional:    true,
				Description: "Authentication type for the Service Fabric REST API. Supported values: \"certificate\", \"entra\".",
			},
			"cluster_application_id": providerschema.StringAttribute{
				Optional:    true,
				Description: "Service Fabric server application ID used when requesting Entra tokens.",
			},
			"tenant_id": providerschema.StringAttribute{
				Optional:    true,
				Description: "Entra tenant ID. Required for auth_type \"entra\" when default credentials are insufficient.",
			},
			"client_id": providerschema.StringAttribute{
				Optional:    true,
				Description: "Entra client ID for application or user-assigned managed identity.",
			},
			"client_secret": providerschema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Entra client secret for the specified client_id.",
			},
			"client_certificate_path": providerschema.StringAttribute{
				Optional:    true,
				Description: "Path to a client certificate in PFX/PKCS#12 format used for certificate authentication.",
			},
			"client_certificate_password": providerschema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Password for the client certificate when using certificate authentication.",
			},
		},
	}
}

// Configure creates the Service Fabric API client shared by all resources/data sources.
func (p *serviceFabricProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config serviceFabricProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Determine authentication mode.
	authType := "certificate"
	if !config.AuthType.IsNull() {
		authType = config.AuthType.ValueString()
	}

	if config.Endpoint.IsNull() || config.Endpoint.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("endpoint"),
			"Missing endpoint",
			"The provider requires an endpoint value.",
		)
		return
	}

	httpClient := &http.Client{
		Timeout: 60 * time.Second,
	}

	if !config.SkipTLSVerify.IsNull() && config.SkipTLSVerify.ValueBool() {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}
		transport.TLSClientConfig.InsecureSkipVerify = true
		httpClient.Transport = transport
	}

	var auth servicefabric.Authenticator
	var err error

	switch authType {
	case "certificate":
		if config.ClientCertificatePath.IsNull() || config.ClientCertificatePath.ValueString() == "" {
			resp.Diagnostics.AddAttributeError(
				path.Root("client_certificate_path"),
				"Missing client certificate",
				"The provider requires client_certificate_path when auth_type is \"certificate\".",
			)
			return
		}
		password := ""
		if !config.ClientCertificatePassword.IsNull() {
			password = config.ClientCertificatePassword.ValueString()
		}
		auth, err = servicefabric.NewCertificateAuthenticator(config.ClientCertificatePath.ValueString(), password)
		if err != nil {
			resp.Diagnostics.AddError(
				"Failed to load client certificate",
				fmt.Sprintf("Unable to read certificate at %q: %s", config.ClientCertificatePath.ValueString(), err),
			)
			return
		}
	case "entra":
		if config.ClusterApplicationID.IsNull() || config.ClusterApplicationID.ValueString() == "" {
			resp.Diagnostics.AddAttributeError(
				path.Root("cluster_application_id"),
				"Missing cluster application ID",
				"The provider requires cluster_application_id when auth_type is \"entra\".",
			)
			return
		}
		options := servicefabric.EntraOptions{
			ClusterApplicationID: config.ClusterApplicationID.ValueString(),
		}
		if !config.TenantID.IsNull() {
			options.TenantID = config.TenantID.ValueString()
		}
		if !config.ClientID.IsNull() {
			options.ClientID = config.ClientID.ValueString()
		}
		if !config.ClientSecret.IsNull() {
			options.ClientSecret = config.ClientSecret.ValueString()
		}

		auth, err = servicefabric.NewEntraAuthenticator(options)
		if err != nil {
			resp.Diagnostics.AddError(
				"Failed to initialize Entra authentication",
				err.Error(),
			)
			return
		}
	default:
		resp.Diagnostics.AddAttributeError(
			path.Root("auth_type"),
			"Unsupported authentication type",
			fmt.Sprintf("Auth type %q is not supported. Valid values: \"certificate\", \"entra\".", authType),
		)
		return
	}

	if err := auth.ConfigureHTTPClient(httpClient); err != nil {
		resp.Diagnostics.AddError(
			"Failed to configure HTTP client",
			err.Error(),
		)
		return
	}

	client, err := servicefabric.NewClient(servicefabric.ClientConfig{
		Endpoint:      config.Endpoint.ValueString(),
		HTTPClient:    httpClient,
		Authenticator: auth,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Client initialization failed",
			err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Service Fabric client configured", map[string]any{
		"endpoint": config.Endpoint.ValueString(),
		"authType": authType,
	})

	resp.DataSourceData = client
	resp.ResourceData = client
}

// Resources returns the resources implemented by the provider.
func (p *serviceFabricProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewApplicationTypeResource,
		NewApplicationResource,
	}
}

// DataSources returns data sources implemented by the provider.
func (p *serviceFabricProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewApplicationTypeDataSource,
		NewApplicationDataSource,
	}
}
