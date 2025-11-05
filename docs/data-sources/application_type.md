# servicefabric_application_type (Data Source)

Retrieves information about application types registered in a Service Fabric
cluster. When no arguments are supplied, all application types are returned via
the `application_types` attribute.

## Example Usage

```terraform
data "servicefabric_application_type" "all" {}

output "registered_types" {
  value = data.servicefabric_application_type.all.application_types
}

data "servicefabric_application_type" "sample" {
  name    = "Contoso.SampleAppType"
  version = "1.0.0"
}
```

## Argument Reference

- `name` (Optional) – Filters application types by name.
- `version` (Optional) – Filters by version. Requires `name`.

## Attribute Reference

- `application_types` – List of objects with the following attributes:
  - `name`
  - `version`
  - `status`
  - `default_parameters` – Map of default parameters.
- `id` – Populated for single results (`name/version`).
- `status` – Provisioning status when a single result is returned.
- `default_parameters` – Map of default parameters when a single result is
  returned.

When a single application type matches the filter, the top-level attributes
(`name`, `version`, `status`, `default_parameters`) are populated for
compatibility with resource-style lookups.
