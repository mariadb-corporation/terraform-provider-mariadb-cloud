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

const azureServiceConfig = `
	resource "skysql_service" default {
		service_type        = "transactional"
		topology            = "es-single"
		cloud_provider      = "azure"
		region              = "eastus"
		name                = "test-azure"
		architecture        = "amd64"
		nodes               = 1
		size                = "sky-2x8"
		storage             = 100
		ssl_enabled         = true
		version             = "10.6.11-6-1"
		deletion_protection = false
		%s
	}
`

// azureServiceFromCreate answers a create the way the API does for Azure: an unset
// volume_type gets PremiumV2_LRS, and a PremiumV2_LRS disk gets the 3000 IOPS and
// 125 MB/s baseline unless the request sets them.
func azureServiceFromCreate(serviceID string, payload provisioning.CreateServiceRequest) *provisioning.Service {
	service := &provisioning.Service{
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
	if service.StorageVolume.VolumeType == "" {
		service.StorageVolume.VolumeType = "PremiumV2_LRS"
	}
	if service.StorageVolume.VolumeType == "PremiumV2_LRS" {
		service.StorageVolume.IOPS = max(int(payload.VolumeIOPS), 3000)
		service.StorageVolume.Throughput = max(int(payload.VolumeThroughput), 125)
	}
	return service
}

// An Azure service with no volume_type gets the API's PremiumV2_LRS disk and plans
// clean afterwards. Naming PremiumV2_LRS later, to set IOPS and throughput, changes
// the disk in place instead of replacing the service.
func TestServiceResourceAzurePremiumV2Default(t *testing.T) {
	const serviceID = "dbdgf42002419"

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
		r.Empty(payload.VolumeType)
		r.Zero(payload.VolumeIOPS)
		r.Zero(payload.VolumeThroughput)

		service = azureServiceFromCreate(serviceID, payload)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		r.NoError(json.NewEncoder(w).Encode(service))
	})
	// Get service: during the create and the refreshes after it
	for i := 0; i < 4; i++ {
		expectRequest(getService)
	}
	// Set IOPS and throughput in place
	expectRequest(func(w http.ResponseWriter, req *http.Request) {
		r.Equal(
			fmt.Sprintf("%s %s/%s/storage", http.MethodPatch, "/provisioning/v1/services", serviceID),
			fmt.Sprintf("%s %s", req.Method, req.URL.Path))
		payload := &provisioning.UpdateStorageRequest{}
		r.NoError(json.NewDecoder(req.Body).Decode(payload))
		r.Equal(int64(100), payload.Size)
		r.Equal(int64(6000), payload.IOPS)
		r.Equal(int64(300), payload.Throughput)
		service.StorageVolume.IOPS = int(payload.IOPS)
		service.StorageVolume.Throughput = int(payload.Throughput)
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
				Config: fmt.Sprintf(azureServiceConfig, ``),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("skysql_service.default", "id", serviceID),
					resource.TestCheckResourceAttr("skysql_service.default", "volume_type", "PremiumV2_LRS"),
					resource.TestCheckNoResourceAttr("skysql_service.default", "volume_iops"),
					resource.TestCheckNoResourceAttr("skysql_service.default", "volume_throughput"),
				),
			},
			{
				Config: fmt.Sprintf(azureServiceConfig, `
					volume_type       = "PremiumV2_LRS"
					volume_iops       = 6000
					volume_throughput = 300
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("skysql_service.default", "id", serviceID),
					resource.TestCheckResourceAttr("skysql_service.default", "volume_type", "PremiumV2_LRS"),
					resource.TestCheckResourceAttr("skysql_service.default", "volume_iops", "6000"),
					resource.TestCheckResourceAttr("skysql_service.default", "volume_throughput", "300"),
				),
			},
		},
	})
}

// A PremiumV2_LRS service is created with the IOPS and throughput from the
// configuration, and changes them in place. Taking them out of the configuration
// leaves the disk as it is.
func TestServiceResourceAzurePremiumV2VolumeType(t *testing.T) {
	const serviceID = "dbdgf42002420"

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
		r.Equal("PremiumV2_LRS", payload.VolumeType)
		r.Equal(uint(5000), payload.VolumeIOPS)
		r.Equal(uint(200), payload.VolumeThroughput)

		service = azureServiceFromCreate(serviceID, payload)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		r.NoError(json.NewEncoder(w).Encode(service))
	})
	// Get service: during the create and the refreshes after it
	for i := 0; i < 4; i++ {
		expectRequest(getService)
	}
	// Change the throughput in place
	expectRequest(func(w http.ResponseWriter, req *http.Request) {
		r.Equal(
			fmt.Sprintf("%s %s/%s/storage", http.MethodPatch, "/provisioning/v1/services", serviceID),
			fmt.Sprintf("%s %s", req.Method, req.URL.Path))
		payload := &provisioning.UpdateStorageRequest{}
		r.NoError(json.NewDecoder(req.Body).Decode(payload))
		r.Equal(int64(100), payload.Size)
		r.Equal(int64(5000), payload.IOPS)
		r.Equal(int64(400), payload.Throughput)
		service.StorageVolume.Throughput = int(payload.Throughput)
		w.Header().Set("Content-Type", "application/json")
	})
	// Get service: waiting for the update and the refreshes after it
	for i := 0; i < 3; i++ {
		expectRequest(getService)
	}
	// No storage call when IOPS and throughput leave the configuration: refreshes only
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
				Config: fmt.Sprintf(azureServiceConfig, `
					volume_type       = "PremiumV2_LRS"
					volume_iops       = 5000
					volume_throughput = 200
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("skysql_service.default", "id", serviceID),
					resource.TestCheckResourceAttr("skysql_service.default", "volume_type", "PremiumV2_LRS"),
					resource.TestCheckResourceAttr("skysql_service.default", "volume_iops", "5000"),
					resource.TestCheckResourceAttr("skysql_service.default", "volume_throughput", "200"),
				),
			},
			{
				Config: fmt.Sprintf(azureServiceConfig, `
					volume_type       = "PremiumV2_LRS"
					volume_iops       = 5000
					volume_throughput = 400
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("skysql_service.default", "id", serviceID),
					resource.TestCheckResourceAttr("skysql_service.default", "volume_iops", "5000"),
					resource.TestCheckResourceAttr("skysql_service.default", "volume_throughput", "400"),
				),
			},
			{
				Config: fmt.Sprintf(azureServiceConfig, `
					volume_type = "PremiumV2_LRS"
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("skysql_service.default", "id", serviceID),
					resource.TestCheckResourceAttr("skysql_service.default", "volume_type", "PremiumV2_LRS"),
					resource.TestCheckNoResourceAttr("skysql_service.default", "volume_iops"),
					resource.TestCheckNoResourceAttr("skysql_service.default", "volume_throughput"),
				),
			},
		},
	})
}

// On Azure, volume_type is StandardSSD_LRS or PremiumV2_LRS, and only PremiumV2_LRS
// takes IOPS and throughput.
func TestServiceResourceAzureVolumeOptions(t *testing.T) {
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
				Config:      fmt.Sprintf(azureServiceConfig, `volume_type = "Premium_LRS"`),
				ExpectError: regexp.MustCompile(`Use: StandardSSD_LRS or PremiumV2_LRS\s+for volume_type`),
			},
			{
				Config: fmt.Sprintf(azureServiceConfig, `
					volume_type = "StandardSSD_LRS"
					volume_iops = 3000
				`),
				ExpectError: regexp.MustCompile(`volume_iops is supported only for PremiumV2_LRS\s+volume_type`),
			},
			{
				Config:      fmt.Sprintf(azureServiceConfig, `volume_throughput = 125`),
				ExpectError: regexp.MustCompile(`volume_throughput is supported only for PremiumV2_LRS\s+volume_type`),
			},
		},
	})
}
