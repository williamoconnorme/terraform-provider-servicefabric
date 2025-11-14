# Service Fabric Terraform Provider

This repository implements a Terraform provider for managing Service Fabric application types and applications directly via the Service Fabric management REST APIs.

## Features

- Configure Service Fabric client authentication using:
  - Mutual TLS with a cluster client certificate (`PFX/PKCS#12` file).
  - Entra ID (Azure AD) tokens using client secret credentials or the default Azure credential chain (Azure CLI, Managed Identity, workload identity, etc.).
- Manage application types by provisioning and unprovisioning `.sfpkg` packages.
- Deploy and manage Service Fabric applications, including parameter updates.
- Configure application capacity constraints and managed identities for applications.
- Automatically orchestrate Service Fabric upgrades (with optional force-recreate behavior) when replacing existing applications.
- Query existing application types, services, and applications via Terraform data sources.

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

  # Certificate-based auth (optional)
  # client_certificate_path     = "C:\\certs\\sf-client.pfx"
  # client_certificate_password = var.sf_cert_password

  # Entra ID (Azure AD) auth (used when no certificate path is provided)
  cluster_application_id  = "00000000-0000-0000-0000-000000000000"
  tenant_id               = "11111111-1111-1111-1111-111111111111"
  client_id               = "22222222-2222-2222-2222-222222222222"
  client_secret           = var.service_principal_secret
  # default_credential_type = "azure_cli" # Optional override of the DefaultAzureCredential chain
  # allow_application_type_version_updates = true
}
```

Optional provider argument `application_recreate_on_upgrade` (default `true`) controls whether replacing an existing application triggers a Service Fabric upgrade with ForceRestart instead of deleting the application.
Set `allow_application_type_version_updates = true` to enable in-place updates of `servicefabric_application_type` versions during Terraform apply (the previous version remains registered in the cluster unless you unprovision it manually).

### Authentication Notes

- **Certificate** authentication expects a PKCS#12 (`.pfx`) file containing the client certificate and key. Supplying `client_certificate_path` switches the provider to certificate mode.
- **Entra** authentication is used automatically when no certificate is configured. Provide the `cluster_application_id` and optionally `tenant_id`, `client_id`, and `client_secret`. When `client_secret` is omitted the provider falls back to `DefaultAzureCredential` (Azure CLI, Azure Developer CLI, Managed Identity, workload identity, Azure PowerShell, environment credentials, etc.). Set `default_credential_type` to force a specific credential from that chain.

## Managed Resources

### `servicefabric_application_type`

Registers an application type version from an external package URL:

```hcl
resource "servicefabric_application_type" "sample" {
  name        = "Contoso.SampleAppType"
  version     = "1.0.0"
  package_uri = "https://storage.example.net/apps/Contoso.SampleAppType_1.0.0.sfpkg?..."
}
```

Optional argument `retain_versions = true` keeps older versions registered with the cluster after destroy.


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

  application_capacity {
    minimum_nodes = 2
    maximum_nodes = 4

    application_metrics {
      name                       = "ApiBudget"
      maximum_capacity           = 25
      reservation_capacity       = 5
      total_application_capacity = 80
    }
  }

  managed_application_identity {
    token_service_endpoint = "https://cluster.example.com:19080/TokenService"

    identities = [
      "MyUserAssignedIdentity",
      "00000000-0000-0000-0000-000000000000",
    ]
  }

  upgrade_policy {
    force_restart = false
    upgrade_mode  = "Monitored"

    monitoring_policy {
      failure_action              = "Rollback"
      health_check_wait_duration  = "PT60S"
      health_check_stable_duration = "PT120S"
    }

    application_health_policy {
      consider_warning_as_error                   = true
      max_percent_unhealthy_deployed_applications = 20
    }
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

data "servicefabric_service_type" "actor" {
  application_type_name    = "Contoso.SampleAppType"
  application_type_version = "1.0.0"
  service_type_name        = "Actor1ActorServiceType"
}

data "servicefabric_service" "api" {
  application_name = "fabric:/Contoso.Sample"
  name             = "fabric:/Contoso.Sample/ApiService"
}

resource "servicefabric_service" "api" {
  name              = "fabric:/Contoso.Sample/ApiService"
  application_name  = servicefabric_application.sample.name
  service_type_name = "Contoso.Sample.ApiServiceType"
  service_kind      = "Stateless"

  partition {
    scheme = "Singleton"
  }

  stateless {
    instance_count = 3
  }
}
```

### `servicefabric_service`

Deploys a stateful or stateless Service Fabric service within an application:

```hcl
resource "servicefabric_service" "api" {
  name              = "fabric:/Contoso.Sample/ApiService"
  application_name  = servicefabric_application.sample.name
  service_type_name = "Contoso.Sample.ApiServiceType"
  service_kind      = "Stateless"

  partition {
    scheme = "Singleton"
  }

  stateless {
    instance_count = 3
  }
}
```
Supports singleton, named, and uniform int64 range partitions plus common mutable properties such as instance/replica counts, placement constraints, DNS names, and default move cost.

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

  # Certificate auth (optional)
  # client_certificate_path     = "C:\\certs\\service-fabric-client.pfx"
  # client_certificate_password = var.sf_client_cert_password

  # Entra auth (default when no certificate is supplied)
  cluster_application_id  = "00000000-0000-0000-0000-000000000000"
  tenant_id               = "11111111-1111-1111-1111-111111111111"
  client_id               = "22222222-2222-2222-2222-222222222222"
  client_secret           = var.service_principal_secret
  # default_credential_type = "managed_identity"
  # allow_application_type_version_updates = true
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

  application_capacity {
    minimum_nodes = 1
    maximum_nodes = 3

    application_metrics {
      name                       = "WorkerLoad"
      maximum_capacity           = 10
      reservation_capacity       = 4
      total_application_capacity = 20
    }
  }

  managed_application_identity {
    identities = ["MySystemAssignedIdentity"]
  }
}

data "servicefabric_application_type" "all_types" {}

output "application_types" {
  value = data.servicefabric_application_type.all_types.application_types
}
```
