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
  client_certificate_path     = "C:\\certs\\sf-client.pfx"
  client_certificate_password = var.sf_client_password

  # For Entra (DefaultAzureCredential) authentication instead of certificates:
  # cluster_application_id  = "00000000-0000-0000-0000-000000000000"
  # tenant_id               = "11111111-1111-1111-1111-111111111111"
  # default_credential_type = "azure_cli" # optional override
}
```

## Authentication

Two authentication methods are supported:

- **certificate**: supply `client_certificate_path` and optionally
  `client_certificate_password`.
- **entra** (default when no certificate is provided): set `cluster_application_id`.
  You may optionally specify `tenant_id`, `client_id`, `client_secret`, and
  `default_credential_type`. When those are omitted the provider falls back
  to Azure's `DefaultAzureCredential` chain.

Set `skip_tls_verify = true` only for development clusters.

## Argument Reference

The following arguments are supported in the provider block:

- `endpoint` (Required) HTTPS management endpoint for the cluster.
- `skip_tls_verify` (Optional) Skip TLS validation.
- `client_certificate_path` / `client_certificate_password` (Optional) Required
  when using certificate authentication.
- `cluster_application_id` (Required when not using certificates) Application ID used to request
  tokens from Entra ID.
- `tenant_id`, `client_id`, `client_secret` (Optional) Entra credential details.
- `default_credential_type` (Optional) Restrict the DefaultAzureCredential chain to a single credential (`default`, `environment`, `workload_identity`, `managed_identity`, `azure_cli`, `azure_developer_cli`, `azure_powershell`).
- `application_recreate_on_upgrade` (Optional) When true, replacements of existing applications trigger an upgrade with ForceRestart instead of deleting and recreating the application.
- `allow_application_type_version_updates` (Optional) Permit in-place updates to `servicefabric_application_type` versions. When true, Terraform will show an update instead of a replacement, even though the previous version remains registered unless manually unprovisioned.

## Resources

- [`servicefabric_application_type`](resources/application_type.md)
- [`servicefabric_application`](resources/application.md)
- [`servicefabric_service`](resources/service.md)

## Data Sources

- [`servicefabric_application_type`](data-sources/application_type.md)
- [`servicefabric_application`](data-sources/application.md)
- [`servicefabric_service_type`](data-sources/service_type.md)
- [`servicefabric_service`](data-sources/service.md)
