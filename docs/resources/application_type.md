# servicefabric_application_type

Registers a Service Fabric application type version from a package stored in an
external image store (such as Azure Blob Storage). Provisioning is asynchronous
and the resource waits for completion before returning.

## Example Usage

```terraform
resource "servicefabric_application_type" "sample" {
  name        = "Contoso.SampleAppType"
  version     = "1.0.0"
  package_uri = "https://storage.example.net/apps/Contoso.SampleAppType_1.0.0.sfpkg?sig=..."
}
```

## Argument Reference

The following arguments are supported:

- `name` (Required) - Application type name as defined in the application
  manifest. Changing this recreates the resource.
- `version` (Required) - Application type version. Changing this recreates the
  resource unless the provider option `allow_application_type_version_updates`
  is enabled.
- `package_uri` (Required) - HTTPS URI to the `.sfpkg` package. Usually a SAS
  URL in Azure Blob Storage. Changing this recreates the resource.
- `retain_versions` (Optional) - Defaults to `false`. When enabled the resource
  skips unprovisioning older versions so Service Fabric can retire them after
  application upgrades complete.

## Attribute Reference

In addition to the arguments above, the following attributes are exported:

- `id` - Combination of `name/version`.
- `status` - Provisioning status reported by the cluster.

> **Note:** Enabling the provider option `allow_application_type_version_updates`
> allows Terraform to update the `version` and `package_uri` in place. The
> previous version remains registered in the cluster unless you unprovision it
> manually or disable `retain_versions` and destroy the resource.

## Import

Application types can be imported using `name/version`, for example:

```shell
terraform import servicefabric_application_type.sample Contoso.SampleAppType/1.0.0
```

