---
page_title: "Scaling Instance Type and Storage"
description: |-
  How to scale instance type (size) and storage for SkySQL services
---

# Scaling Instance Type and Storage

The SkySQL Terraform provider supports in-place scaling of instance type and storage size without destroying and recreating the service.

## Scaling Instance Type

To change the instance type, update the `size` attribute on your `skysql_service` resource.

For example, to scale from `sky-2x8` to `sky-4x16`:

```hcl
resource "skysql_service" "default" {
  service_type      = "transactional"
  topology          = "es-single"
  cloud_provider    = "aws"
  region            = "us-east-1"
  name              = "myservice"
  architecture      = "amd64"
  nodes             = 1
  size              = "sky-4x16" # was "sky-2x8"
  storage           = 100
  ssl_enabled       = true
  version           = "10.6"
  wait_for_creation = true
  wait_for_update   = true
}
```

Running `terraform apply` will issue a size change request to the SkySQL API. The service will briefly enter a scaling state before returning to `ready`.

### Available Sizes

Instance sizes follow the naming convention `sky-<vCPUs>x<RAM_GB>`. Common values include:

- `sky-2x4`
- `sky-2x8`
- `sky-4x16`
- `sky-4x32`
- `sky-8x32`
- `sky-8x64`
- `sky-16x64`
- `sky-16x128`
- `sky-32x128`

Use the [SkySQL Portal](https://app.skysql.com) or the SkySQL API to list all available sizes for your cloud provider and region.

## Scaling Storage

To change the storage size, update the `storage` attribute. On AWS, you can also adjust `volume_iops` and `volume_throughput` at the same time.

For example, to scale storage from 100 GB to 500 GB with increased IOPS:

```hcl
resource "skysql_service" "default" {
  service_type      = "transactional"
  topology          = "es-single"
  cloud_provider    = "aws"
  region            = "us-east-1"
  name              = "myservice"
  architecture      = "amd64"
  nodes             = 1
  size              = "sky-2x8"
  storage           = 500  # was 100
  ssl_enabled       = true
  version           = "10.6"
  volume_type       = "gp3"
  volume_iops       = 6000 # was 3000
  volume_throughput = 250  # was 125
  wait_for_creation = true
  wait_for_update   = true
}
```

### Storage Attributes

- `storage` — Storage size in GB. Valid values: 100, 200, 300, 400, 500, 600, 700, 800, 900, 1000, 2000, 3000, 4000, 5000, 6000, 7000, 8000, 9000, 10000.
- `volume_iops` — Volume IOPS (AWS only). Applies to `gp3`, `io1` and `io2` volume types.
- `volume_throughput` — Volume throughput in MB/s (AWS only). Applies to `gp3` volume type.

~> **Note:** Storage can only be scaled up, not down. Attempting to reduce storage size will result in an error from the SkySQL API.

## Scaling Both at Once

You can change `size` and `storage` in the same `terraform apply`. The provider applies changes sequentially, waiting for the service to return to `ready` between each operation:

1. Instance type (`size`) is updated first
2. Storage (`storage`, `volume_iops`, `volume_throughput`) is updated second

```hcl
resource "skysql_service" "default" {
  service_type      = "transactional"
  topology          = "es-single"
  cloud_provider    = "aws"
  region            = "us-east-1"
  name              = "myservice"
  architecture      = "amd64"
  nodes             = 1
  size              = "sky-4x16" # scaled up from sky-2x8
  storage           = 500        # scaled up from 100
  ssl_enabled       = true
  version           = "10.6"
  volume_type       = "gp3"
  volume_iops       = 6000
  volume_throughput = 250
  wait_for_creation = true
  wait_for_update   = true
}
```

## Scaling MaxScale Nodes

Replicated topologies that include MaxScale (for example `es-replica` and `galera`) accept a `maxscale_nodes` attribute. Changing its value updates the service in place — for example, going from a single MaxScale node to a redundant pair:

```hcl
resource "skysql_service" "default" {
  service_type      = "transactional"
  topology          = "galera"
  cloud_provider    = "aws"
  region            = "us-east-1"
  name              = "myservice"
  architecture      = "amd64"
  nodes             = 3
  maxscale_nodes    = 2 # was 1
  maxscale_size     = "sky-2x4"
  size              = "sky-4x16"
  storage           = 100
  ssl_enabled       = true
  version           = "10.6"
  wait_for_creation = true
  wait_for_update   = true
}
```

A `maxscale_nodes` change can be combined with a `nodes` change in the same apply; the provider submits them as a single scaling request.

~> **Note:** Valid values are 1 or 2, and MaxScale changes require the Power tier — the same rules that apply when configuring MaxScale at creation. Removing `maxscale_nodes` from the configuration entirely forces the service to be replaced, as does changing `maxscale_size`.

## Important Notes

- **Set `wait_for_update = true`** to ensure Terraform waits for each scaling operation to complete before proceeding. Without this, Terraform will return immediately after submitting the request.
- **Scaling is an online operation** — the service remains accessible during scaling, though brief performance impact may occur.
- **Node count scaling** is also supported via the `nodes` attribute and can be combined with size, storage, and MaxScale node changes in the same apply.
