# Service Fabric Terraform Provider

This repository implements a Terraform provider for managing Service Fabric application types and applications directly via the Service Fabric management REST APIs.

## Features

- Configure Service Fabric client authentication using:
  - Mutual TLS with a cluster client certificate (`PFX/PKCS#12` file).
  - Entra ID (Azure AD) tokens using client secret credentials or the default Azure credential chain (Azure CLI, Managed Identity, workload identity, etc.).
- Manage application types by provisioning and unprovisioning `.sfpkg` packages.
- Deploy and manage Service Fabric applications, including parameter updates.
- Query existing application types and applications via Terraform data sources.

## Building

```powershell
go build -o "$env:GOPATH\bin\terraform-provider-servicefabric.exe"
```

> Adjust the output path to match your environment. Terraform expects the binary name to follow the pattern `terraform-provider-<name>`.

## Provider Configuration

```hcl
terraform {
  required_providers {
    servicefabric = {
      source = "williamoconnorme/servicefabric"
    }
  }
}

provider "servicefabric" {
  endpoint        = "https://my-sf-cluster.contoso.com:19080"
  skip_tls_verify = false

  # Choose one authentication option:

  # 1. Certificate-based auth
  # auth_type                = "certificate" # default
  # client_certificate_path  = "C:\\certs\\sf-client.pfx"
  # client_certificate_password = var.sf_cert_password

  # 2. Entra ID (Azure AD) auth
  auth_type              = "entra"
  cluster_application_id = "00000000-0000-0000-0000-000000000000"
  tenant_id              = "11111111-1111-1111-1111-111111111111"
  client_id              = "22222222-2222-2222-2222-222222222222"
  client_secret          = var.service_principal_secret
}
```

### Authentication Notes

- **Certificate** authentication expects a PKCS#12 (`.pfx`) file containing the client certificate and key.
- **Entra** authentication requests a token for the cluster's server application ID. When `client_id`/`client_secret` are omitted the provider falls back to `DefaultAzureCredential`, which supports Azure CLI login, Managed Identity, and workload identity.

## Managed Resources

### `servicefabric_application_type`

Registers an application type version from an external package URL:

```hcl
resource "servicefabric_application_type" "sample" {
  name        = "Contoso.SampleAppType"
  version     = "1.0.0"
  package_uri = "https://storage.example.net/apps/Contoso.SampleAppType_1.0.0.sfpkg?..."
}

- Optional argument `retain_versions` can be set to `true` when you want to keep
  older versions registered with the cluster after destroy.
```

### `servicefabric_application`

Creates or updates an application instance:

```hcl
resource "servicefabric_application" "sample" {
  name         = "fabric:/Contoso.Sample"
  type_name    = servicefabric_application_type.sample.name
  type_version = servicefabric_application_type.sample.version

  parameters = {
    ApplicationInsightsConnectionString = "InstrumentationKey=..."
    AzureSubscriptionId                 = "00000000-0000-0000-0000-000000000000"
    ServiceFabricClusterName            = "sf-contoso-dev"
  }
}
```

## Data Sources

```hcl
data "servicefabric_application_type" "sample" {
  name    = "Contoso.SampleAppType"
  version = "1.0.0"
}

data "servicefabric_application" "sample" {
  name = "fabric:/Contoso.Sample"
}
```

## Example Configuration

```hcl
terraform {
  required_providers {
    servicefabric = {
      source = "williamoconnorme/servicefabric"
    }
  }
}

provider "servicefabric" {
  endpoint        = "https://cluster.example.com:19080"
  skip_tls_verify = false

  # Certificate auth (default)
  # client_certificate_path     = "C:\\certs\\service-fabric-client.pfx"
  # client_certificate_password = var.sf_client_cert_password

  # Entra auth
  auth_type              = "entra"
  cluster_application_id = "00000000-0000-0000-0000-000000000000"
  tenant_id              = "11111111-1111-1111-1111-111111111111"
  client_id              = "22222222-2222-2222-2222-222222222222"
  client_secret          = var.service_principal_secret
}

resource "servicefabric_application_type" "sample" {
  name        = "Contoso.SampleAppType"
  version     = "1.0.0"
  package_uri = "https://storage.example.net/sample-apps/Contoso.SampleAppType_1.0.0.sfpkg?sig=..."
}

resource "servicefabric_application" "sample" {
  name         = "fabric:/Contoso.Sample"
  type_name    = servicefabric_application_type.sample.name
  type_version = servicefabric_application_type.sample.version

  parameters = {
    Environment          = "dev"
    InstanceCount        = "3"
    MonitoringConnection = "InstrumentationKey=..."
  }
}

data "servicefabric_application_type" "all_types" {}

output "application_types" {
  value = data.servicefabric_application_type.all_types.application_types
}
```

## Next Steps

- Implement acceptance tests using the Terraform Plugin Testing framework and a mock cluster endpoint.
- Publish the provider to the Terraform Registry once ready for distribution.
