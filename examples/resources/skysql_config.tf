# Example 1: Create a new service with a custom configuration
#
# Create a custom configuration object with server variable overrides.
# The topology and version must match the target service.
#
# By default, allow_restart is false — any variable that requires a service
# restart (e.g. innodb_buffer_pool_size) will be rejected unless you set
# allow_restart = true.
resource "skysql_config" "tuned" {
  name          = "my-tuned-config"
  topology      = "es-replica"
  version       = "10.6.7-3-1"
  allow_restart = true

  values = {
    "max_connections"         = "500"
    "innodb_buffer_pool_size" = "2G"
  }
}

# Apply the configuration to a new service at creation time.
# Requires wait_for_creation = true so the service is ready before the config is applied.
resource "skysql_service" "default" {
  service_type      = "transactional"
  topology          = "es-replica"
  cloud_provider    = "aws"
  region            = "us-east-1"
  name              = "myservice"
  architecture      = "amd64"
  nodes             = 1
  size              = "sky-2x8"
  storage           = 100
  ssl_enabled       = true
  version           = "10.6.7-3-1"
  wait_for_creation = true
  config_id         = skysql_config.tuned.id
}

# Example 2: Apply a custom configuration to an existing service
#
# Step 1: Create a config matching the service's topology and version.
#
# Step 2: Add config_id to the service resource block.
#
# Step 3: Run terraform plan / apply.
#   Terraform will detect that config_id changed from empty to the new config ID
#   and apply it to the service.
#
# Day 2 operations:
#   - Change config_id to a different config → applies the new config.
#   - Remove config_id from the resource block → reverts to the default config.

resource "skysql_config" "custom" {
  name     = "custom-config"
  topology = "es-single"
  version  = "10.6.11-6-1"

  values = {
    "max_connections" = "1000"
  }
}

resource "skysql_service" "existing" {
  # ... all service attributes...
  config_id = skysql_config.custom.id
}

# Example 3: Share a single configuration across multiple services
#
# A config can be referenced by multiple services. When you change values
# in the config, ALL services using it are automatically reconfigured.
#
# Important:
#   - You cannot delete a config that is assigned to any service.
#     Terraform handles this automatically — it removes config_id from
#     services before destroying the config resource.
#   - Changing values in a shared config triggers a rolling reconfiguration
#     on every service that references it.

resource "skysql_config" "shared" {
  name     = "shared-config"
  topology = "es-single"
  version  = "10.6.11-6-1"

  values = {
    "max_connections" = "500"
  }
}

resource "skysql_service" "service_a" {
  # ... service attributes (topology and version must match the config) ...
  wait_for_creation = true
  config_id         = skysql_config.shared.id
}

resource "skysql_service" "service_b" {
  # ... service attributes (topology and version must match the config) ...
  wait_for_creation = true
  config_id         = skysql_config.shared.id
}

# Example 4: Use another value on the replicas
#
# In an es-replica configuration, selected variables can have a value on the
# replicas that is different from the value on the primary. A variable that is
# not in replica_values has the value of the primary on the replicas too.
# To make the replicas follow the primary again, remove the variable from
# replica_values.
#
# This needs a SkySQL environment that has the replica values turned on.
resource "skysql_config" "replica_tuned" {
  name     = "replica-tuned-config"
  topology = "es-replica"
  version  = "10.6.7-3-1"

  values = {
    "sync_binlog" = "1"
  }

  replica_values = {
    "sync_binlog" = "100"
  }
}
