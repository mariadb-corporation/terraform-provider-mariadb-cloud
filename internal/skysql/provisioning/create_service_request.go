package provisioning

type CreateServiceRequest struct {
	Name             string            `json:"name"`
	ProjectID        string            `json:"project_id"`
	ServiceType      string            `json:"service_type"`
	Provider         string            `json:"provider"`
	Region           string            `json:"region"`
	Version          string            `json:"version"`
	Nodes            uint              `json:"nodes"`
	Architecture     string            `json:"architecture"`
	Size             string            `json:"size"`
	Topology         string            `json:"topology"`
	Storage          uint              `json:"storage"`
	VolumeIOPS       uint              `json:"volume_iops"`
	VolumeThroughput uint              `json:"volume_throughput"`
	SSLEnabled       bool              `json:"ssl_enabled"`
	NoSQLEnabled     bool              `json:"nosql_enabled"`
	VolumeType       string            `json:"volume_type,omitempty"`
	AllowedAccounts  []string          `json:"endpoint_allowed_accounts,omitempty"`
	Mechanism        string            `json:"endpoint_mechanism,omitempty"`
	AllowList        []AllowListItem   `json:"allow_list,omitempty"`
	MaxscaleNodes    uint              `json:"maxscale_nodes,omitempty"`
	MaxscaleSize     *string           `json:"maxscale_size,omitempty"`
	AvailabilityZone string            `json:"availability_zone,omitempty"`
	Tags             map[string]string `json:"tags,omitempty"`
}
