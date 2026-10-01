# Changelog

## [3.5.8] - 2026-10-01
### Changed
- Release files are now named `terraform-provider-mariadb-cloud`, after the repository's new name (`mariadb-corporation/terraform-provider-mariadb-cloud`). The provider's behavior is unchanged.

## [3.5.7] - 2026-07-17
### Added
- `maxscale_nodes` can now be changed in place. The provider applies the change through the service nodes API instead of destroying and recreating the service. Removing the attribute from configuration still forces replacement.

## [3.5.6] - 2026-07-15
### Fixed
- Creating a `serverless-standalone` service now fails fast with a clear error when the topology is not offered to the calling organization (for example BYOA organizations), instead of surfacing a raw API error at apply time (MCDEV-4132).

### Added
- BYOA documentation: a [Bring Your Own Account guide](docs/guides/byoa.md) and a runnable [`examples/byoa`](examples/byoa) example covering topology restrictions, endpoint defaults, and region availability for BYOA organizations.

## [3.5.5] - 2026-06-17
### Fixed
- Concurrent service operations (for example a config change and a scaling operation) no longer conflict. When the API rejects a mutation because the service is in a transient `pending_*` state, the provider now waits for the service to become modifiable and retries instead of failing the apply (MCDEV-3899).

## [3.5.4] - 2026-04-09
### Fixed
- Fixed broken HTTP retry logic — transient 5xx errors were never retried despite retry configuration.
- Added 429 rate-limit handling with `Retry-After` header support.
- Improved error messages to include API response details instead of generic "500 Internal Server Error".

## [3.5.3] - 2026-04-08
### Reverted
- Reverted `ssl_enabled` changes from 3.5.2. The feature is not fully implemented yet and will be completed in a future release.

## [3.5.2] - 2026-04-06
### Fixed
- `ssl_enabled` can now be toggled on an existing service in-place without requiring destroy and recreate ([#45](https://github.com/skysqlinc/terraform-provider-skysql/issues/45)).
- Improved `ssl_enabled` documentation to explain that it controls TLS encryption for client database connections.

## [3.5.1] - 2026-04-06
### Fixed
- Fixed `tags` attribute causing perpetual diffs and "Provider produced inconsistent result after apply" errors ([#34](https://github.com/skysqlinc/terraform-provider-skysql/issues/34)).
- Removed `Computed: true` from `tags` schema — tags are now purely user-managed.
- Removed `tagsNamePlanModifier` that was injecting `tags["name"]` into the plan, violating Terraform's plan contract.
- `readServiceState()` now filters API-returned tags to only include keys present in the user's configuration, preventing API-injected tags (e.g. `name`) from leaking into state.
- `updateServiceTags()` now correctly handles null/unknown plan tags when users omit tags from config.

## [3.5.0] - 2026-02-26
### Changed
- **Breaking:** `org_id` moved from `skysql_service` resource attribute to provider-level attribute. Use `org_id` in the `provider "skysql"` block or the `TF_SKYSQL_ORG_ID` environment variable instead. All resources and data sources now inherit the organization context from the provider.
- Removed `RequestOption` / `WithOrgID` internals — org header is set globally on the HTTP client.

## [3.4.0] - 2026-02-21
### Added
- `skysql_config` resource for creating and managing custom configuration objects with server variable overrides.
- `config_id` attribute on `skysql_service` resource to apply a custom configuration to a service.

## [3.3.0] - 2026-01-16
### Added
- Galera Cluster topology support for high-availability multi-master replication
- Example configuration and documentation for Galera deployments

## [3.2.0] - 2025-10-23
### Added
- Multi-organization support.

## [3.1.5] - 2025-10-23
### Fixed
- prevent start/stop operations on serverless-standalone services.

## [3.1.4] - 2025-07-17
### Features
- service `tags` support added.

## [3.1.3] - 2024-07-25
### Package Updates
- upgraded go 1.19 => 1.21
- added toolchain go1.22.3
- upgraded github.com/golang/protobuf v1.5.2 => v1.5.4
- upgraded github.com/google/go-cmp v0.5.9 => v0.6.0
- upgraded github.com/google/uuid v1.3.0 => v1.6.0
- upgraded google.golang.org/grpc v1.54.0 => v1.65.0

## [3.1.2] - 2024-07-24
### Package Vulnerability Updates
- upgraded google.golang.org/protobuf v1.30.0 => v1.34.2
- upgraded golang.org/x/crypto v0.0.0-20220622213112-05595931fe9d => v0.25.0
- upgraded golang.org/x/net v0.8.0 => v0.27.0
- upgraded golang.org/x/sys v0.6.0 => v0.22.0
- upgraded golang.org/x/text v0.8.0 => v0.16.0

## [3.1.1] - 2024-07-16
### Fixed
- `gp3` volume type support for AWS.
- `volume_throughput` support for AWS GP3 storage volume type.

## [3.1.0] - 2024-07-15
### Features
- `azure` provider is now supported.

## [3.0.0] - 2024-06-05
### Breaking Change
- New API Key Access, `TF_SKYSQL_API_KEY` [API Access](https://app.skysql.com/user-profile/api-keys)

## [2.0.1] - 2024-02-24
**OBSOLETE - DO NOT USE**
### Fixed
- `volume_type` imports work as expected.
