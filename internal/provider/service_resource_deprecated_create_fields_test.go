package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/skysqlinc/terraform-provider-skysql/internal/skysql"
	"github.com/skysqlinc/terraform-provider-skysql/internal/skysql/provisioning"
	"github.com/stretchr/testify/require"
)

// replication_enabled and primary_host stay in the schema so existing configurations
// still plan, but the create request leaves them out.
func TestServiceResourceDeprecatedCreateFields(t *testing.T) {
	const serviceID = "dbdgf42002418"

	testURL, expectRequest, closeAPI := mockSkySQLAPI(t)
	defer closeAPI()
	os.Setenv("TF_SKYSQL_API_KEY", "[api-key]")
	os.Setenv("TF_SKYSQL_API_BASE_URL", testURL)

	r := require.New(t)

	configureOnce.Reset()
	var service *provisioning.Service
	getService := func(w http.ResponseWriter, req *http.Request) {
		r.Equal(
			fmt.Sprintf("%s %s/%s", http.MethodGet, "/provisioning/v1/services", serviceID),
			fmt.Sprintf("%s %s", req.Method, req.URL.Path))
		w.Header().Set("Content-Type", "application/json")
		service.Status = "ready"
		json.NewEncoder(w).Encode(service)
	}
	// Check API connectivity
	expectRequest(func(w http.ResponseWriter, req *http.Request) {
		r.Equal("/provisioning/v1/versions", req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]provisioning.Version{})
	})
	// Create service
	expectRequest(func(w http.ResponseWriter, req *http.Request) {
		r.Equal(http.MethodPost, req.Method)
		r.Equal("/provisioning/v1/services", req.URL.Path)
		body := map[string]any{}
		r.NoError(json.NewDecoder(req.Body).Decode(&body))
		r.NotContains(body, "replication_enabled")
		r.NotContains(body, "primary_host")

		service = &provisioning.Service{
			ID:           serviceID,
			Name:         body["name"].(string),
			Region:       body["region"].(string),
			Provider:     body["provider"].(string),
			Tier:         "power",
			Topology:     body["topology"].(string),
			Version:      body["version"].(string),
			Architecture: body["architecture"].(string),
			Size:         body["size"].(string),
			Nodes:        int(body["nodes"].(float64)),
			SSLEnabled:   body["ssl_enabled"].(bool),
			Status:       "pending_create",
			CreatedOn:    int(time.Now().Unix()),
			UpdatedOn:    int(time.Now().Unix()),
			CreatedBy:    uuid.New().String(),
			UpdatedBy:    uuid.New().String(),
			Endpoints: []provisioning.Endpoint{
				{
					Name:  "primary",
					Ports: []provisioning.Port{{Name: "readwrite", Port: 3306, Purpose: "readwrite"}},
				},
			},
			IsActive:    true,
			ServiceType: body["service_type"].(string),
			// Echo the configured values back, so the test only checks the request.
			ReplicationEnabled: true,
			PrimaryHost:        "dbpgf00000001",
		}
		service.StorageVolume.Size = int(body["storage"].(float64))
		service.StorageVolume.VolumeType = "pd-ssd"

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		r.NoError(json.NewEncoder(w).Encode(service))
	})
	// Get service: during the create and the refreshes after it
	for i := 0; i < 3; i++ {
		expectRequest(getService)
	}
	expectRequest(func(w http.ResponseWriter, req *http.Request) {
		r.Equal(
			fmt.Sprintf("%s %s/%s", http.MethodDelete, "/provisioning/v1/services", serviceID),
			fmt.Sprintf("%s %s", req.Method, req.URL.Path))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(service)
	})
	expectRequest(func(w http.ResponseWriter, req *http.Request) {
		r.Equal(
			fmt.Sprintf("%s %s/%s", http.MethodGet, "/provisioning/v1/services", serviceID),
			fmt.Sprintf("%s %s", req.Method, req.URL.Path))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(&skysql.ErrorResponse{Code: http.StatusNotFound})
	})

	resource.Test(t, resource.TestCase{
		IsUnitTest: true,
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"skysql": providerserver.NewProtocol6WithError(New("")()),
		},
		Steps: []resource.TestStep{
			{
				Config: `
					resource "skysql_service" default {
						service_type        = "transactional"
						topology            = "es-single"
						cloud_provider      = "gcp"
						region              = "us-central1"
						name                = "test-deprecated"
						architecture        = "amd64"
						nodes               = 1
						size                = "sky-2x8"
						storage             = 100
						ssl_enabled         = true
						version             = "10.6.11-6-1"
						deletion_protection = false
						replication_enabled = true
						primary_host        = "dbpgf00000001"
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("skysql_service.default", "id", serviceID),
					resource.TestCheckResourceAttr("skysql_service.default", "replication_enabled", "true"),
					resource.TestCheckResourceAttr("skysql_service.default", "primary_host", "dbpgf00000001"),
				),
			},
		},
	})
}
