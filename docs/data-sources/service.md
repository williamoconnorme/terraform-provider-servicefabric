# servicefabric_service (Data Source)

Returns Service Fabric service information for a specific application. You can
lookup a single service by name or list every service within the application,
optionally filtering by service type.

## Example Usage

```terraform
data "servicefabric_service" "api" {
  application_name = "fabric:/Contoso.Sample"
  name             = "fabric:/Contoso.Sample/ApiService"
}

output "api_status" {
  value = data.servicefabric_service.api.service_status
}

data "servicefabric_service" "all_services" {
  application_name   = "fabric:/Contoso.Sample"
  service_type_name  = "MyWorkerServiceType"
}

output "services" {
  value = data.servicefabric_service.all_services.services
}
```

## Argument Reference

- `application_name` (Required) – Full Service Fabric application name
  (`fabric:/...`).
- `name` (Optional) – Full Service Fabric service name to retrieve. When
  omitted, all services for the application are returned via `services`.
- `service_type_name` (Optional) – Filters the service list by type when listing
  services.

## Attribute Reference

- `services` – List of service objects containing:
  - `id`
  - `name`
  - `service_kind`
  - `type_name`
  - `manifest_version`
  - `health_state`
  - `service_status`
  - `is_service_group`
  - `has_persisted_state`
  - `arm_resource_id`
- `type_name`, `manifest_version`, `service_kind`, `health_state`,
  `service_status`, `is_service_group`, `has_persisted_state`, `arm_resource_id`
  – Populated when a single service is returned.
- `id` – Uses the Service Fabric service ID when a single service is returned,
  otherwise set to the application name.
