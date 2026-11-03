package provisioning

// Config represents a DPS configuration object.
type Config struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	CreatedDate int64    `json:"created_date"`
	UpdatedDate int64    `json:"updated_date"`
	CreatedBy   string   `json:"created_by,omitempty"`
	UpdatedBy   string   `json:"updated_by,omitempty"`
	Public      bool     `json:"public"`
	TopologyID  string   `json:"topology_id"`
	VersionID   string   `json:"version_id"`
	Services    []string `json:"services,omitempty"`
}

// CreateConfigRequest is the request body for POST /configs.
// Uses name-based topology and version resolution (Phase 1 DPS enhancement).
type CreateConfigRequest struct {
	Name     string `json:"name"`
	Topology string `json:"topology"`
	Version  string `json:"version"`
}

// UpdateConfigRequest is the request body for PATCH /configs/{id}.
type UpdateConfigRequest struct {
	Name string `json:"name"`
}

// ConfigValueRequest is the request body for POST /configs/{id}/values/{variable_name}.
type ConfigValueRequest struct {
	Value string `json:"value"`
}

// ServiceConfigState is the request body for POST /services/{service_id}/config.
type ServiceConfigState struct {
	ConfigID string `json:"config_id"`
}

// ConfigKey represents a configurable parameter returned by GET /topologies/{topology}/configs.
type ConfigKey struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Component       string   `json:"component"`
	Flags           uint     `json:"flags"`
	ConfigSection   string   `json:"config_section,omitempty"`
	Tags            []string `json:"tags"`
	AllowedValues   []string `json:"allowed_values"`
	DefaultValues   []string `json:"default_value"`
	DocURL          string   `json:"documentation_url,omitempty"`
	Readonly        bool     `json:"readonly"`
	Multiselect     bool     `json:"multiselect"`
	RequiresRestart bool     `json:"requires_restart"`
	MinimumValue    string   `json:"minimum_value,omitempty"`
	MaximumValue    string   `json:"maximum_value,omitempty"`
	// The replica fields are only in the answer when DPS has the replica values turned on, and
	// only for a variable that can have a value of its own on the replicas.
	ReplicaConfigurable *bool   `json:"replica_configurable,omitempty"`
	ReplicaReadonly     *bool   `json:"replica_readonly,omitempty"`
	ReplicaDefault      *string `json:"replica_default,omitempty"`
	PrimaryReadonly     *bool   `json:"primary_readonly,omitempty"`
}
