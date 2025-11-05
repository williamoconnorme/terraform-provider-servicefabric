package servicefabric

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"golang.org/x/crypto/pkcs12"
)

// Authenticator configures the HTTP client and applies per-request credentials.
type Authenticator interface {
	ConfigureHTTPClient(client *http.Client) error
	Apply(ctx context.Context, req *http.Request) error
}

// CertificateAuthenticator implements TLS client certificate authentication.
type CertificateAuthenticator struct {
	cert tls.Certificate
}

// NewCertificateAuthenticator loads the certificate from a PKCS#12/PFX file.
func NewCertificateAuthenticator(path string, password string) (Authenticator, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	privateKey, certificate, err := pkcs12.Decode(raw, password)
	if err != nil {
		return nil, fmt.Errorf("pkcs12 decode: %w", err)
	}

	cert := tls.Certificate{
		Certificate: [][]byte{certificate.Raw},
		PrivateKey:  privateKey,
		Leaf:        certificate,
	}

	return &CertificateAuthenticator{cert: cert}, nil
}

// ConfigureHTTPClient attaches the client certificate to the TLS configuration.
func (c *CertificateAuthenticator) ConfigureHTTPClient(client *http.Client) error {
	transport, err := ensureTransport(client)
	if err != nil {
		return err
	}
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.Certificates = []tls.Certificate{c.cert}
	return nil
}

// Apply does nothing per-request for certificate authentication.
func (c *CertificateAuthenticator) Apply(_ context.Context, _ *http.Request) error {
	return nil
}

// EntraOptions contains parameters for acquiring Entra ID tokens.
type EntraOptions struct {
	ClusterApplicationID string
	TenantID             string
	ClientID             string
	ClientSecret         string
}

// EntraAuthenticator acquires bearer tokens using Azure Identity credentials.
type EntraAuthenticator struct {
	cred  azcore.TokenCredential
	scope string
}

// NewEntraAuthenticator builds an Entra authenticator using default or explicit credentials.
func NewEntraAuthenticator(opts EntraOptions) (Authenticator, error) {
	if opts.ClusterApplicationID == "" {
		return nil, fmt.Errorf("cluster application id required")
	}

	scope := fmt.Sprintf("%s/.default", opts.ClusterApplicationID)

	var cred azcore.TokenCredential
	var err error

	switch {
	case opts.ClientID != "" && opts.ClientSecret != "":
		cred, err = azidentity.NewClientSecretCredential(opts.TenantID, opts.ClientID, opts.ClientSecret, nil)
	default:
		options := &azidentity.DefaultAzureCredentialOptions{}
		if opts.TenantID != "" {
			options.TenantID = opts.TenantID
		}
		cred, err = azidentity.NewDefaultAzureCredential(options)
	}
	if err != nil {
		return nil, err
	}

	return &EntraAuthenticator{
		cred:  cred,
		scope: scope,
	}, nil
}

func (a *EntraAuthenticator) ConfigureHTTPClient(_ *http.Client) error {
	return nil
}

func (a *EntraAuthenticator) Apply(ctx context.Context, req *http.Request) error {
	token, err := a.cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{a.scope},
	})
	if err != nil {
		return fmt.Errorf("failed to get token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.Token)
	return nil
}

func ensureTransport(client *http.Client) (*http.Transport, error) {
	if client.Transport == nil {
		client.Transport = http.DefaultTransport.(*http.Transport).Clone()
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("unsupported transport type %T", client.Transport)
	}
	return transport, nil
}
