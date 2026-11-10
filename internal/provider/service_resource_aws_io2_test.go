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

const awsIO2Config = `
	resource "skysql_service" default {
		service_type        = "transactional"
		topology            = "es-single"
		cloud_provider      = "aws"
		region              = "us-east-1"
		name                = "test-aws"
		architecture        = "amd64"
		nodes               = 1
		size                = "sky-2x8"
		storage             = 100
		ssl_enabled         = true
		version             = "10.6.11-6-1"
		deletion_protection = false
		volume_type         = "io2"
		%s
	}
`

// An io2 service is created with the IOPS from the configuration, and a later IOPS
// change is applied in place, like on io1.
func TestServiceResourceAWSIO2VolumeType(t *testing.T) {
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
		payload := provisioning.CreateServiceRequest{}
		r.NoError(json.NewDecoder(req.Body).Decode(&payload))
		r.Equal("io2", payload.VolumeType)
		r.Equal(uint(20000), payload.VolumeIOPS)
		r.Zero(payload.VolumeThroughput)

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
		service.StorageVolume.VolumeType = payload.VolumeType
		service.StorageVolume.IOPS = int(payload.VolumeIOPS)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		r.NoError(json.NewEncoder(w).Encode(service))
	})
	// Get service: during the create and the refreshes after it
	for i := 0; i < 4; i++ {
		expectRequest(getService)
	}
	// Change the IOPS in place
	expectRequest(func(w http.ResponseWriter, req *http.Request) {
		r.Equal(
			fmt.Sprintf("%s %s/%s/storage", http.MethodPatch, "/provisioning/v1/services", serviceID),
			fmt.Sprintf("%s %s", req.Method, req.URL.Path))
		payload := &provisioning.UpdateStorageRequest{}
		r.NoError(json.NewDecoder(req.Body).Decode(payload))
		r.Equal(int64(100), payload.Size)
		r.Equal(int64(32000), payload.IOPS)
		r.Zero(payload.Throughput)
		service.StorageVolume.IOPS = int(payload.IOPS)
		w.Header().Set("Content-Type", "application/json")
	})
	// Get service: waiting for the update and the refreshes after it
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
				Config: fmt.Sprintf(awsIO2Config, `volume_iops = 20000`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("skysql_service.default", "id", serviceID),
					resource.TestCheckResourceAttr("skysql_service.default", "volume_type", "io2"),
					resource.TestCheckResourceAttr("skysql_service.default", "volume_iops", "20000"),
					resource.TestCheckNoResourceAttr("skysql_service.default", "volume_throughput"),
				),
			},
			{
				Config: fmt.Sprintf(awsIO2Config, `volume_iops = 32000`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("skysql_service.default", "id", serviceID),
					resource.TestCheckResourceAttr("skysql_service.default", "volume_type", "io2"),
					resource.TestCheckResourceAttr("skysql_service.default", "volume_iops", "32000"),
				),
			},
		},
	})
}

// io2 follows the io1 rules: IOPS are required and throughput can't be set.
func TestServiceResourceAWSIO2VolumeOptions(t *testing.T) {
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

	resource.Test(t, resource.TestCase{
		IsUnitTest: true,
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"skysql": providerserver.NewProtocol6WithError(New("")()),
		},
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(awsIO2Config, ``),
				ExpectError: regexp.MustCompile(`volume_iops are required for AWS`),
			},
			{
				Config: fmt.Sprintf(awsIO2Config, `
					volume_iops       = 20000
					volume_throughput = 125
				`),
				ExpectError: regexp.MustCompile(`volume_throughput is not supported for io2`),
			},
		},
	})
}
