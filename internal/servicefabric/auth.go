package servicefabric

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
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

	privateKey, certificate, caCerts, err := decodePKCS12(raw, password)
	if err != nil {
		return nil, err
	}

	chain := make([][]byte, 0, 1+len(caCerts))
	chain = append(chain, certificate.Raw)
	for _, ca := range caCerts {
		chain = append(chain, ca.Raw)
	}

	cert := tls.Certificate{
		Certificate: chain,
		PrivateKey:  privateKey,
		Leaf:        certificate,
	}

	return &CertificateAuthenticator{cert: cert}, nil
}

func decodePKCS12(raw []byte, password string) (any, *x509.Certificate, []*x509.Certificate, error) {
	// First try the simple decode for the common case.
	privateKey, certificate, err := pkcs12.Decode(raw, password)
	if err == nil {
		return privateKey, certificate, nil, nil
	}

	// Fall back to parsing PEM blocks so we can handle chains/multiple bags.
	blocks, err := pkcs12.ToPEM(raw, password)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("pkcs12 decode: %w", err)
	}

	var (
		key        any
		leaf       *x509.Certificate
		caCerts    []*x509.Certificate
		certBlocks [][]byte
	)

	for _, block := range blocks {
		if block == nil {
			continue
		}
		switch block.Type {
		case "PRIVATE KEY", "RSA PRIVATE KEY", "EC PRIVATE KEY":
			parsedKey, err := parsePrivateKey(block.Bytes)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("pkcs12 decode: %w", err)
			}
			key = parsedKey
		case "CERTIFICATE":
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("pkcs12 decode: %w", err)
			}
			certBlocks = append(certBlocks, cert.Raw)
			caCerts = append(caCerts, cert)
		}
	}

	if key == nil {
		return nil, nil, nil, fmt.Errorf("pkcs12 decode: no private key found")
	}
	if len(caCerts) == 0 {
		return nil, nil, nil, fmt.Errorf("pkcs12 decode: no certificate found")
	}

	// Pick the leaf certificate: prefer one that is not a CA.
	for _, cert := range caCerts {
		if !cert.IsCA {
			leaf = cert
			break
		}
	}
	if leaf == nil {
		leaf = caCerts[0]
	}

	// Remove the leaf from the CA list.
	remaining := make([]*x509.Certificate, 0, len(caCerts)-1)
	for _, cert := range caCerts {
		if cert != leaf {
			remaining = append(remaining, cert)
		}
	}

	return key, leaf, remaining, nil
}

func parsePrivateKey(der []byte) (any, error) {
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return key, nil
	}
	// Some PFX blobs are returned as PEM-encoded DER inside a PEM block.
	if block, _ := pem.Decode(der); block != nil {
		return parsePrivateKey(block.Bytes)
	}
	return nil, fmt.Errorf("unsupported private key format")
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
	// Force ALPN to HTTP/1.1 for cert auth.
	transport.TLSClientConfig.NextProtos = []string{"http/1.1"}
	// Some Service Fabric gateways request TLS renegotiation for client certs.
	transport.TLSClientConfig.Renegotiation = tls.RenegotiateOnceAsClient
	transport.ForceAttemptHTTP2 = false
	transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	return nil
}

// Apply does nothing per-request for certificate authentication.
func (c *CertificateAuthenticator) Apply(_ context.Context, _ *http.Request) error {
	return nil
}

// EntraOptions contains parameters for acquiring Entra ID tokens.
type EntraOptions struct {
	ClusterApplicationID  string
	TenantID              string
	ClientID              string
	ClientSecret          string
	DefaultCredentialType string
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
	if opts.ClientID != "" && opts.ClientSecret != "" {
		cred, err = azidentity.NewClientSecretCredential(opts.TenantID, opts.ClientID, opts.ClientSecret, nil)
	} else {
		cred, err = buildDefaultAzureCredential(opts)
	}
	if err != nil {
		return nil, err
	}

	return &EntraAuthenticator{
		cred:  cred,
		scope: scope,
	}, nil
}

func buildDefaultAzureCredential(opts EntraOptions) (azcore.TokenCredential, error) {
	switch opts.DefaultCredentialType {
	case "", "default":
		options := &azidentity.DefaultAzureCredentialOptions{}
		if opts.TenantID != "" {
			options.TenantID = opts.TenantID
		}
		return azidentity.NewDefaultAzureCredential(options)
	case "environment":
		return azidentity.NewEnvironmentCredential(nil)
	case "workload_identity":
		options := &azidentity.WorkloadIdentityCredentialOptions{
			ClientID: opts.ClientID,
			TenantID: opts.TenantID,
		}
		return azidentity.NewWorkloadIdentityCredential(options)
	case "managed_identity":
		options := &azidentity.ManagedIdentityCredentialOptions{}
		if opts.ClientID != "" {
			options.ID = azidentity.ClientID(opts.ClientID)
		}
		return azidentity.NewManagedIdentityCredential(options)
	case "azure_cli":
		options := &azidentity.AzureCLICredentialOptions{
			TenantID: opts.TenantID,
		}
		return azidentity.NewAzureCLICredential(options)
	case "azure_developer_cli":
		options := &azidentity.AzureDeveloperCLICredentialOptions{
			TenantID: opts.TenantID,
		}
		return azidentity.NewAzureDeveloperCLICredential(options)
	case "azure_powershell":
		options := &azidentity.AzurePowerShellCredentialOptions{
			TenantID: opts.TenantID,
		}
		return azidentity.NewAzurePowerShellCredential(options)
	default:
		return nil, fmt.Errorf("unsupported credential type %q", opts.DefaultCredentialType)
	}
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
