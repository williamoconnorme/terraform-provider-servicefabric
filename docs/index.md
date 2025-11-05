# Service Fabric Provider

The Service Fabric provider interacts directly with the Service Fabric
management REST APIs so you can manage application types and applications
without going through Azure Resource Manager. The provider supports both
certificate-based and Entra ID (Azure AD) authentication.

## Example Usage

```terraform
terraform {
  required_providers {
    servicefabric = {
      source = "williamoconnorme/servicefabric"
    }
  }
}

provider "servicefabric" {
  endpoint        = "https://cluster.example.com:19080"
  auth_type       = "certificate"
  client_certificate_path     = "C:\\certs\\sf-client.pfx"
  client_certificate_password = var.sf_client_password
}
```

## Authentication

Two authentication methods are supported:

- **certificate** (default): supply `client_certificate_path` and optionally
  `client_certificate_password`.
- **entra**: supply `auth_type = "entra"` and the cluster application ID via
  `cluster_application_id`. You may optionally specify `tenant_id`,
  `client_id`, `client_secret`. When those are omitted the provider falls back
  to Azure's `DefaultAzureCredential`.

Set `skip_tls_verify = true` only for development clusters.

## Argument Reference

The following arguments are supported in the provider block:

- `endpoint` (Required) HTTPS management endpoint for the cluster.
- `auth_type` (Optional) Either `certificate` (default) or `entra`.
- `skip_tls_verify` (Optional) Skip TLS validation.
- `client_certificate_path` / `client_certificate_password` (Optional) Required
  when using certificate authentication.
- `cluster_application_id` (Required for `entra`) Application ID used to request
  tokens from Entra ID.
- `tenant_id`, `client_id`, `client_secret` (Optional) Entra credential details.

## Resources

- [`servicefabric_application_type`](resources/application_type.md)
- [`servicefabric_application`](resources/application.md)

## Data Sources

- [`servicefabric_application_type`](data-sources/application_type.md)
- [`servicefabric_application`](data-sources/application.md)
