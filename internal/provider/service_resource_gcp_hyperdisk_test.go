package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
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

// A GCP service on n4 machines, which can only attach Hyperdisk, comes back from the
// API as hyperdisk-balanced with 3000 IOPS and 140 MiB/s although the config set no
// volume. The apply must still match the plan. MCDEV-5319.
func TestServiceResourceGCPHyperdiskFromAPI(t *testing.T) {
	const serviceID = "dbdgf42002418"

	testURL, expectRequest, closeAPI := mockSkySQLAPI(t)
	defer closeAPI()
	os.Setenv("TF_SKYSQL_API_KEY", "[api-key]")
	os.Setenv("TF_SKYSQL_API_BASE_URL", testURL)

	r := require.New(t)

	configureOnce.Reset()
	var service *provisioning.Service
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
		payload := provisioning.CreateServiceRequest{}
		r.NoError(json.NewDecoder(req.Body).Decode(&payload))
		r.Empty(payload.VolumeType, "the API picks the GCP disk")

		service = &provisioning.Service{
			ID:           serviceID,
			Name:         payload.Name,
			Region:       payload.Region,
			Provider:     payload.Provider,
			Tier:         "power",
			Topology:     payload.Topology,
			Version:      payload.Version,
			Architecture: payload.Architecture,
			Size:         payload.Size,
			Nodes:        int(payload.Nodes),
			SSLEnabled:   payload.SSLEnabled,
			Status:       "pending_create",
			CreatedOn:    int(time.Now().Unix()),
			UpdatedOn:    int(time.Now().Unix()),
			CreatedBy:    uuid.New().String(),
			UpdatedBy:    uuid.New().String(),
			Endpoints: []provisioning.Endpoint{
				{
					Name:      "primary",
					Ports:     []provisioning.Port{{Name: "readwrite", Port: 3306, Purpose: "readwrite"}},
					Mechanism: payload.Mechanism,
				},
			},
			IsActive:    true,
			ServiceType: payload.ServiceType,
		}
		service.StorageVolume.Size = int(payload.Storage)
		service.StorageVolume.VolumeType = "hyperdisk-balanced"
		service.StorageVolume.IOPS = 3000
		service.StorageVolume.Throughput = 140

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		r.NoError(json.NewEncoder(w).Encode(service))
	})
	for i := 0; i < 3; i++ {
		expectRequest(func(w http.ResponseWriter, req *http.Request) {
			r.Equal(
				fmt.Sprintf("%s %s/%s", http.MethodGet, "/provisioning/v1/services", serviceID),
				fmt.Sprintf("%s %s", req.Method, req.URL.Path))
			w.Header().Set("Content-Type", "application/json")
			service.Status = "ready"
			json.NewEncoder(w).Encode(service)
		})
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
						name                = "test-gcp"
						architecture        = "amd64"
						nodes               = 1
						size                = "sky-2x8"
						storage             = 100
						ssl_enabled         = true
						version             = "10.6.11-6-1"
						deletion_protection = false
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("skysql_service.default", "id", serviceID),
					resource.TestCheckResourceAttr("skysql_service.default", "volume_type", "hyperdisk-balanced"),
					resource.TestCheckNoResourceAttr("skysql_service.default", "volume_iops"),
					resource.TestCheckNoResourceAttr("skysql_service.default", "volume_throughput"),
				),
			},
		},
	})
}

// Users still can't choose a GCP disk other than pd-ssd or set its IOPS. MCDEV-5319.
func TestServiceResourceGCPVolumeOptions(t *testing.T) {
	testURL, expectRequest, closeAPI := mockSkySQLAPI(t)
	defer closeAPI()
	os.Setenv("TF_SKYSQL_API_KEY", "[api-key]")
	os.Setenv("TF_SKYSQL_API_BASE_URL", testURL)

	r := require.New(t)

	configureOnce.Reset()
	// Check API connectivity
	expectRequest(func(w http.ResponseWriter, req *http.Request) {
		r.Equal("/provisioning/v1/versions", req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]provisioning.Version{})
	})

	const config = `
		resource "skysql_service" default {
			service_type        = "transactional"
			topology            = "es-single"
			cloud_provider      = "gcp"
			region              = "us-central1"
			name                = "test-gcp"
			nodes               = 1
			size                = "sky-2x8"
			storage             = 100
			ssl_enabled         = true
			deletion_protection = false
			%s
		}
	`
	resource.Test(t, resource.TestCase{
		IsUnitTest: true,
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"skysql": providerserver.NewProtocol6WithError(New("")()),
		},
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(config, `volume_type = "hyperdisk-balanced"`),
				ExpectError: regexp.MustCompile(`volume_type = "hyperdisk-balanced" is not supported for "gcp" provider`),
			},
			{
				Config:      fmt.Sprintf(config, `volume_iops = 5000`),
				ExpectError: regexp.MustCompile(`volume_iops is not supported for "gcp" provider`),
			},
		},
	})
}
