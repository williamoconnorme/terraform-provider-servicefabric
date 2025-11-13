# servicefabric_service_type (Data Source)

Retrieves service type metadata declared within a specific application type
version. You can either request a single service type by name or list every
service type exposed by the application type version.

## Example Usage

```terraform
data "servicefabric_service_type" "all_worker_types" {
  application_type_name    = "Contoso.SampleAppType"
  application_type_version = "1.0.0"
}

output "service_types" {
  value = data.servicefabric_service_type.all_worker_types.service_types
}

data "servicefabric_service_type" "actor" {
  application_type_name    = "Contoso.SampleAppType"
  application_type_version = "1.0.0"
  service_type_name        = "Actor1ActorServiceType"
}
```

## Argument Reference

- `application_type_name` (Required) – Application type that declares the
  service types.
- `application_type_version` (Required) – Application type version that declares
  the service types.
- `service_type_name` (Optional) – When set, returns the matching service type.
  When omitted, all service types in the version are returned through the
  `service_types` attribute.

## Attribute Reference

- `service_types` – List of service types with the following attributes:
  - `service_type_name`
  - `kind`
  - `service_manifest_name`
  - `service_manifest_version`
  - `is_service_group`
  - `has_persisted_state`
  - `service_type_description_json` – Raw JSON payload returned by the Service
    Fabric API describing the service type.
- `service_type_name`, `kind`, `service_manifest_name`,
  `service_manifest_version`, `is_service_group`,
  `has_persisted_state`, `service_type_description_json` – populated when a
  single service type is returned.
- `id` – Composite identifier of `application_type_name/application_type_version`
  or `application_type_name/application_type_version/service_type_name` when a
  single match exists.
