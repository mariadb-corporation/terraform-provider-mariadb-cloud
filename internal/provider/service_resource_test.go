package provider

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
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

func mockSkySQLAPI(t *testing.T) (string, func(http.HandlerFunc), func()) {
	var (
		receivedCalls   int
		expectedCalls   []http.HandlerFunc
		addExpectedCall = func(h http.HandlerFunc) {
			expectedCalls = append(expectedCalls, h)
		}
		processedRequests = make([]string, 0)
		r                 = require.New(t)
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		reqDump, err := httputil.DumpRequest(req, true)
		if err != nil {
			log.Fatal(err)
		}
		if receivedCalls >= len(expectedCalls) {
			w.WriteHeader(http.StatusNotFound)
			r.Failf("unexpected call",
				"we have already received %d calls from expected %d.\nunexpected request: %s",
				receivedCalls,
				len(expectedCalls),
				string(reqDump),
			)
		}

		expectedCalls[receivedCalls](w, req)
		processedRequests = append(processedRequests, string(reqDump))
		receivedCalls++
	}))

	return ts.URL, addExpectedCall, func() {
		ts.Close()
		r.Equal(
			len(expectedCalls),
			receivedCalls,
			"expected one more request. processed: \n %v",
			processedRequests,
		)
	}
}

func TestServiceResource(t *testing.T) {
	const serviceID = "dbdgf42002418"

	testUrl, expectRequest, close := mockSkySQLAPI(t)
	defer close()
	os.Setenv("TF_SKYSQL_API_KEY", "[api-key]")
	os.Setenv("TF_SKYSQL_API_BASE_URL", testUrl)
	tests := []struct {
		name         string
		testResource string
		before       func(r *require.Assertions)
		checks       []resource.TestCheckFunc
		expectError  *regexp.Regexp
	}{
		{
			name: "create service",
			testResource: `
resource "skysql_service" default {
  service_type   = "transactional"
  topology       = "es-single"
  cloud_provider = "gcp"
  region         = "us-central1"
  name           = "test-gcp"
  architecture   = "amd64"
  nodes          = 1
  size           = "sky-2x8"
  storage        = 100
  ssl_enabled    = true
  version        = "10.6.11-6-1"
  wait_for_creation = true
  wait_for_deletion = true
  deletion_protection = false
}
	            `,
			before: func(r *require.Assertions) {
				configureOnce.Reset()
				var service *provisioning.Service
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodGet, req.Method)
					r.Equal("/provisioning/v1/versions", req.URL.Path)
					r.Equal("page_size=1", req.URL.RawQuery)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode([]provisioning.Version{})
				})
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodPost, req.Method)
					r.Equal("/provisioning/v1/services", req.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					payload := provisioning.CreateServiceRequest{}
					err := json.NewDecoder(req.Body).Decode(&payload)
					r.NoError(err)
					service = &provisioning.Service{
						ID:           serviceID,
						Name:         payload.Name,
						Region:       payload.Region,
						Provider:     payload.Provider,
						Tier:         "foundation",
						Topology:     payload.Topology,
						Version:      payload.Version,
						Architecture: payload.Architecture,
						Size:         payload.Size,
						Nodes:        int(payload.Nodes),
						SSLEnabled:   payload.SSLEnabled,
						NosqlEnabled: payload.NoSQLEnabled,
						FQDN:         "",
						Status:       "pending_create",
						CreatedOn:    int(time.Now().Unix()),
						UpdatedOn:    int(time.Now().Unix()),
						CreatedBy:    uuid.New().String(),
						UpdatedBy:    uuid.New().String(),
						Endpoints: []provisioning.Endpoint{
							{
								Name: "primary",
								Ports: []provisioning.Port{
									{
										Name:    "readwrite",
										Port:    3306,
										Purpose: "readwrite",
									},
								},
							},
						},
						StorageVolume: struct {
							Size       int    `json:"size"`
							VolumeType string `json:"volume_type"`
							IOPS       int    `json:"iops"`
							Throughput int    `json:"throughput"`
						}{
							Size:       int(payload.Storage),
							VolumeType: payload.VolumeType,
							IOPS:       int(payload.VolumeIOPS),
							Throughput: int(payload.VolumeThroughput),
						},
						OutboundIps:        nil,
						IsActive:           true,
						ServiceType:        payload.ServiceType,
						ReplicationEnabled: false,
						PrimaryHost:        "",
					}
					if service.Provider == "gcp" {
						service.StorageVolume.VolumeType = "pd-ssd"
					}
					r.NoError(json.NewEncoder(w).Encode(service))
					w.WriteHeader(http.StatusCreated)
				})
				for i := 0; i <= 2; i++ {
					expectRequest(func(w http.ResponseWriter, req *http.Request) {
						r.Equal(http.MethodGet, req.Method)
						r.Equal("/provisioning/v1/services/"+serviceID, req.URL.Path)
						w.Header().Set("Content-Type", "application/json")
						service.Status = "ready"
						r.NoError(json.NewEncoder(w).Encode(service))
						w.WriteHeader(http.StatusOK)
					})
				}
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodDelete, req.Method)
					r.Equal("/provisioning/v1/services/"+serviceID, req.URL.Path)
					w.WriteHeader(http.StatusAccepted)
					w.Header().Set("Content-Type", "application/json")
				})
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodGet, req.Method)
					r.Equal("/provisioning/v1/services/"+serviceID, req.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					json.NewEncoder(w).Encode(&skysql.ErrorResponse{
						Code: http.StatusNotFound,
					})
				})
			},
			checks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("skysql_service.default", "id", serviceID),
			},
		},
		{
			name: "create service when skysql api returns error",
			testResource: `
				resource "skysql_service" default {
				 service_type   = "transactional"
				 topology       = "es-single"
				 cloud_provider = "gcp"
				 region         = "us-central1"
				 name           = "test-gcp"
				 architecture   = "amd64"
				 nodes          = 1
				 size           = "sky-2x8"
				 storage        = 100
				 ssl_enabled    = true
				 version        = "10.6.11-6-1"
				 wait_for_creation = true
				 wait_for_deletion = true
				 deletion_protection = false
				}
					            `,
			before: func(r *require.Assertions) {
				configureOnce.Reset()
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodGet, req.Method)
					r.Equal("/provisioning/v1/versions", req.URL.Path)
					r.Equal("page_size=1", req.URL.RawQuery)
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode([]provisioning.Version{})
					w.WriteHeader(http.StatusOK)
				})
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodPost, req.Method)
					r.Equal("/provisioning/v1/services", req.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					payload := &skysql.ErrorResponse{
						Code: http.StatusBadRequest,
						Errors: []skysql.ErrorDetails{
							{
								Message: "boom",
							},
						},
					}
					json.NewEncoder(w).Encode(payload)
				})
			},
			checks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("skysql_service.default", "id", serviceID),
			},
			expectError: regexp.MustCompile(`Error creating service`),
		},
		{
			name: "create service when skysql api returns unexpected error",
			testResource: `
				resource "skysql_service" default {
				 service_type   = "transactional"
				 topology       = "es-single"
				 cloud_provider = "gcp"
				 region         = "us-central1"
				 name           = "test-gcp"
				 architecture   = "amd64"
				 nodes          = 1
				 size           = "sky-2x8"
				 storage        = 100
				 ssl_enabled    = true
				 version        = "10.6.11-6-1"
				 wait_for_creation = true
				 wait_for_deletion = true
				 deletion_protection = false
				}
					            `,
			before: func(r *require.Assertions) {
				configureOnce.Reset()
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodGet, req.Method)
					r.Equal("/provisioning/v1/versions", req.URL.Path)
					r.Equal("page_size=1", req.URL.RawQuery)
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode([]provisioning.Version{})
					w.WriteHeader(http.StatusOK)
				})
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodPost, req.Method)
					r.Equal("/provisioning/v1/services", req.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
				})
				// The client retries 500s up to 3 times.
				for range 3 {
					expectRequest(func(w http.ResponseWriter, req *http.Request) {
						r.Equal(http.MethodPost, req.Method)
						r.Equal("/provisioning/v1/services", req.URL.Path)
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusInternalServerError)
					})
				}
			},
			checks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("skysql_service.default", "id", serviceID),
			},
			expectError: regexp.MustCompile(`Error creating service`),
		},
		{
			name: "create service with allowlist",
			testResource: `
		resource "skysql_service" default {
		 service_type   = "transactional"
		 topology       = "es-single"
		 cloud_provider = "gcp"
		 region         = "us-central1"
		 name           = "test-gcp"
		 architecture   = "amd64"
		 nodes          = 1
		 size           = "sky-2x8"
		 storage        = 100
		 ssl_enabled    = true
		 version        = "10.6.11-6-1"
		 wait_for_creation = true
		 wait_for_deletion = true
		 deletion_protection = false
		 allow_list = [
		   {
		     "ip": "192.158.1.38/32",
		     "comment": "homeoffice"
		   }
		 ]
		}
			            `,
			before: func(r *require.Assertions) {
				configureOnce.Reset()
				var service *provisioning.Service
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodGet, req.Method)
					r.Equal("/provisioning/v1/versions", req.URL.Path)
					r.Equal("page_size=1", req.URL.RawQuery)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode([]provisioning.Version{})
				})
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodPost, req.Method)
					r.Equal("/provisioning/v1/services", req.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					payload := provisioning.CreateServiceRequest{}
					err := json.NewDecoder(req.Body).Decode(&payload)
					r.NoError(err)
					r.NotEmpty(payload.AllowList)
					service = &provisioning.Service{
						ID:           serviceID,
						Name:         payload.Name,
						Region:       payload.Region,
						Provider:     payload.Provider,
						Tier:         "foundation",
						Topology:     payload.Topology,
						Version:      payload.Version,
						Architecture: payload.Architecture,
						Size:         payload.Size,
						Nodes:        int(payload.Nodes),
						SSLEnabled:   payload.SSLEnabled,
						NosqlEnabled: payload.NoSQLEnabled,
						FQDN:         "",
						Status:       "pending_create",
						CreatedOn:    int(time.Now().Unix()),
						UpdatedOn:    int(time.Now().Unix()),
						CreatedBy:    uuid.New().String(),
						UpdatedBy:    uuid.New().String(),
						Endpoints: []provisioning.Endpoint{
							{
								Name: "primary",
								Ports: []provisioning.Port{
									{
										Name:    "readwrite",
										Port:    3306,
										Purpose: "readwrite",
									},
								},
								AllowList: []provisioning.AllowListItem{
									{
										IPAddress: "192.158.1.38/32",
										Comment:   "homeoffice",
									},
								},
							},
						},
						StorageVolume: struct {
							Size       int    `json:"size"`
							VolumeType string `json:"volume_type"`
							IOPS       int    `json:"iops"`
							Throughput int    `json:"throughput"`
						}{
							Size:       int(payload.Storage),
							VolumeType: payload.VolumeType,
							IOPS:       int(payload.VolumeIOPS),
							Throughput: int(payload.VolumeThroughput),
						},
						OutboundIps:        nil,
						IsActive:           false,
						ServiceType:        payload.ServiceType,
						ReplicationEnabled: false,
						PrimaryHost:        "",
					}
					json.NewEncoder(w).Encode(service)
					w.WriteHeader(http.StatusCreated)
				})
				for i := 0; i < 3; i++ {
					expectRequest(func(w http.ResponseWriter, req *http.Request) {
						r.Equal(http.MethodGet, req.Method)
						r.Equal("/provisioning/v1/services/"+serviceID, req.URL.Path)
						w.Header().Set("Content-Type", "application/json")
						service.Status = "ready"
						json.NewEncoder(w).Encode(service)
						w.WriteHeader(http.StatusOK)
					})
				}
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodDelete, req.Method)
					r.Equal("/provisioning/v1/services/"+serviceID, req.URL.Path)
					w.WriteHeader(http.StatusAccepted)
					w.Header().Set("Content-Type", "application/json")
				})
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodGet, req.Method)
					r.Equal("/provisioning/v1/services/"+serviceID, req.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					json.NewEncoder(w).Encode(&skysql.ErrorResponse{
						Code: http.StatusNotFound,
					})
				})
			},
			checks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("skysql_service.default", "id", serviceID),
			},
		},
		{
			name: "fail when service has the privateconnect mechanism and allowlist is not empty",
			testResource: `
		resource "skysql_service" "default" {
		 service_type      = "transactional"
		 topology          = "es-single"
		 cloud_provider    = "gcp"
		 region            = "europe-west4"
		 name              = "test-service"
		 nodes             = 1
		 size              = "sky-2x8"
		 storage           = 100
		 endpoint_mechanism = "privateconnect"
		 ssl_enabled       = true
		 architecture      = "amd64"
		 wait_for_creation = true
		 wait_for_deletion = true
		 wait_for_update   = true
		 deletion_protection = false
		 allow_list = [
		   {
		     ip : "10.100.0.0/16"
		   }
		 ]
		}`,
			before: func(r *require.Assertions) {},
			checks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("skysql_service.default", "id", ""),
			},
			expectError: regexp.MustCompile(" You can not set allow_list when mechanism has"),
		},
		{
			name: "create service when version is not specified",
			testResource: `
		resource "skysql_service" default {
		 service_type   = "transactional"
		 topology       = "es-single"
		 cloud_provider = "gcp"
		 region         = "us-central1"
		 name           = "test-gcp"
		 architecture   = "amd64"
		 nodes          = 1
		 size           = "sky-2x8"
		 storage        = 100
		 ssl_enabled    = true
		 wait_for_creation = true
		 wait_for_deletion = true
		 deletion_protection = false
		}
			            `,
			before: func(r *require.Assertions) {
				configureOnce.Reset()
				var service *provisioning.Service
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodGet, req.Method)
					r.Equal("/provisioning/v1/versions", req.URL.Path)
					r.Equal("page_size=1", req.URL.RawQuery)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode([]provisioning.Version{})
				})
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodPost, req.Method)
					r.Equal("/provisioning/v1/services", req.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					payload := provisioning.CreateServiceRequest{}
					err := json.NewDecoder(req.Body).Decode(&payload)
					r.NoError(err)
					service = &provisioning.Service{
						ID:           serviceID,
						Name:         payload.Name,
						Region:       payload.Region,
						Provider:     payload.Provider,
						Tier:         "foundation",
						Topology:     payload.Topology,
						Version:      payload.Version,
						Architecture: payload.Architecture,
						Size:         payload.Size,
						Nodes:        int(payload.Nodes),
						SSLEnabled:   payload.SSLEnabled,
						NosqlEnabled: payload.NoSQLEnabled,
						FQDN:         "",
						Status:       "pending_create",
						CreatedOn:    int(time.Now().Unix()),
						UpdatedOn:    int(time.Now().Unix()),
						CreatedBy:    uuid.New().String(),
						UpdatedBy:    uuid.New().String(),
						Endpoints: []provisioning.Endpoint{
							{
								Name: "primary",
								Ports: []provisioning.Port{
									{
										Name:    "readwrite",
										Port:    3306,
										Purpose: "readwrite",
									},
								},
							},
						},
						StorageVolume: struct {
							Size       int    `json:"size"`
							VolumeType string `json:"volume_type"`
							IOPS       int    `json:"iops"`
							Throughput int    `json:"throughput"`
						}{
							Size:       int(payload.Storage),
							VolumeType: payload.VolumeType,
							IOPS:       int(payload.VolumeIOPS),
							Throughput: int(payload.VolumeThroughput),
						},
						OutboundIps:        nil,
						IsActive:           true,
						ServiceType:        payload.ServiceType,
						ReplicationEnabled: false,
						PrimaryHost:        "",
					}
					json.NewEncoder(w).Encode(service)
					w.WriteHeader(http.StatusCreated)
				})
				for i := 0; i < 3; i++ {
					expectRequest(func(w http.ResponseWriter, req *http.Request) {
						r.Equal(http.MethodGet, req.Method)
						r.Equal("/provisioning/v1/services/"+serviceID, req.URL.Path)
						w.Header().Set("Content-Type", "application/json")
						service.Status = "ready"
						json.NewEncoder(w).Encode(service)
						w.WriteHeader(http.StatusOK)
					})
				}
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodDelete, req.Method)
					r.Equal("/provisioning/v1/services/"+serviceID, req.URL.Path)
					w.WriteHeader(http.StatusAccepted)
					w.Header().Set("Content-Type", "application/json")
				})
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodGet, req.Method)
					r.Equal("/provisioning/v1/services/"+serviceID, req.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					json.NewEncoder(w).Encode(&skysql.ErrorResponse{
						Code: http.StatusNotFound,
					})
				})
			},
			checks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("skysql_service.default", "id", serviceID),
			},
		},
		{
			name: "create service when unexpected provider value",
			testResource: fmt.Sprintf(`
				resource "skysql_service" default {
				 service_type   = "transactional"
				 topology       = "es-single"
				 cloud_provider = "boom!"
				 region         = "us-central1"
				 name           = "%s"
				 architecture   = "amd64"
				 nodes          = 1
				 size           = "sky-2x8"
				 storage        = 100
				 ssl_enabled    = true
				 version        = "10.6.11-6-1"
				 wait_for_creation = true
				 wait_for_deletion = true
				 deletion_protection = false
				}
					            `, GenerateServiceName(t)),
			before: func(r *require.Assertions) {
				configureOnce.Reset()
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodGet, req.Method)
					r.Equal("/provisioning/v1/versions", req.URL.Path)
					r.Equal("page_size=1", req.URL.RawQuery)
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode([]provisioning.Version{})
					w.WriteHeader(http.StatusOK)
				})
			},
			expectError: regexp.MustCompile(`Invalid provider value`),
		},
		{
			name: "volume_type required for aws",
			testResource: fmt.Sprintf(`
				resource "skysql_service" default {
				 service_type   = "transactional"
				 topology       = "es-single"
				 cloud_provider = "aws"
				 region         = "us-central1"
				 name           = "%s"
				 architecture   = "amd64"
				 nodes          = 1
				 size           = "sky-2x8"
				 storage        = 100
				 ssl_enabled    = true
				 version        = "10.6.11-6-1"
				 wait_for_creation = true
				 wait_for_deletion = true
				 deletion_protection = false
				}
					            `, GenerateServiceName(t)),
			before: func(r *require.Assertions) {
				configureOnce.Reset()
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodGet, req.Method)
					r.Equal("/provisioning/v1/versions", req.URL.Path)
					r.Equal("page_size=1", req.URL.RawQuery)
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode([]provisioning.Version{})
					w.WriteHeader(http.StatusOK)
				})
			},
			expectError: regexp.MustCompile(`volume_type provided is not supported. Use: io1, io2 or gp3 for volume_type.`),
		},
		{
			name: "invalid volume_type for aws",
			testResource: fmt.Sprintf(`
				resource "skysql_service" default {
				 service_type   = "transactional"
				 topology       = "es-single"
				 cloud_provider = "aws"
				 region         = "us-central1"
				 name           = "%s"
				 architecture   = "amd64"
				 nodes          = 1
				 size           = "sky-2x8"
				 storage        = 100
				 ssl_enabled    = true
				 version        = "10.6.11-6-1"
				 wait_for_creation = true
				 wait_for_deletion = true
				 deletion_protection = false
				 volume_type = "gp2"
				}
					            `, GenerateServiceName(t)),
			before: func(r *require.Assertions) {
				configureOnce.Reset()
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodGet, req.Method)
					r.Equal("/provisioning/v1/versions", req.URL.Path)
					r.Equal("page_size=1", req.URL.RawQuery)
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode([]provisioning.Version{})
					w.WriteHeader(http.StatusOK)
				})
			},
			expectError: regexp.MustCompile(`volume_type provided is not supported. Use: io1, io2 or gp3 for volume_type.`),
		},
		{
			name: "volume_iops are required for aws",
			testResource: fmt.Sprintf(`
				resource "skysql_service" default {
				 service_type   = "transactional"
				 topology       = "es-single"
				 cloud_provider = "aws"
				 region         = "us-central1"
				 name           = "%s"
				 architecture   = "amd64"
				 nodes          = 1
				 size           = "sky-2x8"
				 storage        = 100
				 ssl_enabled    = true
				 version        = "10.6.11-6-1"
				 wait_for_creation = true
				 wait_for_deletion = true
				 deletion_protection = false
				 volume_type    = "io1"
				}
					            `, GenerateServiceName(t)),
			before: func(r *require.Assertions) {
				configureOnce.Reset()
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodGet, req.Method)
					r.Equal("/provisioning/v1/versions", req.URL.Path)
					r.Equal("page_size=1", req.URL.RawQuery)
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode([]provisioning.Version{})
					w.WriteHeader(http.StatusOK)
				})
			},
			expectError: regexp.MustCompile(`volume_iops are required for AWS`),
		},
		{
			name: "volume_throughput is not supported for io1",
			testResource: fmt.Sprintf(`
				resource "skysql_service" default {
				 service_type   = "transactional"
				 topology       = "es-single"
				 cloud_provider = "aws"
				 region         = "us-central1"
				 name           = "%s"
				 architecture   = "amd64"
				 nodes          = 1
				 size           = "sky-2x8"
				 storage        = 100
				 ssl_enabled    = true
				 version        = "10.6.11-6-1"
				 wait_for_creation = true
				 wait_for_deletion = true
				 deletion_protection = false
				 volume_type    = "io1"
                 volume_iops    = 3000
  				 volume_throughput = 125
				}
					            `, GenerateServiceName(t)),
			before: func(r *require.Assertions) {
				configureOnce.Reset()
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodGet, req.Method)
					r.Equal("/provisioning/v1/versions", req.URL.Path)
					r.Equal("page_size=1", req.URL.RawQuery)
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode([]provisioning.Version{})
					w.WriteHeader(http.StatusOK)
				})
			},
			expectError: regexp.MustCompile(`volume_throughput is supported only for gp3 volume_type for AWS`),
		},
		{
			name: "volume_throughput is required for gp3",
			testResource: fmt.Sprintf(`
				resource "skysql_service" default {
				 service_type   = "transactional"
				 topology       = "es-single"
				 cloud_provider = "aws"
				 region         = "us-central1"
				 name           = "%s"
				 architecture   = "amd64"
				 nodes          = 1
				 size           = "sky-2x8"
				 storage        = 100
				 ssl_enabled    = true
				 version        = "10.6.11-6-1"
				 wait_for_creation = true
				 wait_for_deletion = true
				 deletion_protection = false
				 volume_type    = "gp3"
                 volume_iops    = 3000
				}
					            `, GenerateServiceName(t)),
			before: func(r *require.Assertions) {
				configureOnce.Reset()
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodGet, req.Method)
					r.Equal("/provisioning/v1/versions", req.URL.Path)
					r.Equal("page_size=1", req.URL.RawQuery)
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode([]provisioning.Version{})
					w.WriteHeader(http.StatusOK)
				})
			},
			expectError: regexp.MustCompile(`volume_throughput is required for gp3 volume_type for AWS`),
		},
		{
			name: "create galera service",
			testResource: fmt.Sprintf(`
				resource "skysql_service" default {
				 service_type   = "transactional"
				 topology       = "galera"
				 cloud_provider = "gcp"
				 region         = "us-central1"
				 name           = "%s"
				 architecture   = "amd64"
				 nodes          = 3
				 maxscale_nodes = 1
				 size           = "sky-4x16"
				 storage        = 100
				 ssl_enabled    = true
				 version        = "10.6.11-6-1"
				 wait_for_creation = true
				 wait_for_deletion = true
				 deletion_protection = false
				}
					            `, GenerateServiceName(t)),
			before: func(r *require.Assertions) {
				configureOnce.Reset()
				var service *provisioning.Service
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodGet, req.Method)
					r.Equal("/provisioning/v1/versions", req.URL.Path)
					r.Equal("page_size=1", req.URL.RawQuery)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode([]provisioning.Version{})
				})
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodPost, req.Method)
					r.Equal("/provisioning/v1/services", req.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					payload := provisioning.CreateServiceRequest{}
					err := json.NewDecoder(req.Body).Decode(&payload)
					r.NoError(err)
					r.Equal("galera", payload.Topology)
					r.Equal(uint(3), payload.Nodes)
					r.Equal(uint(1), payload.MaxscaleNodes)
					r.Equal("sky-4x16", payload.Size)
					service = &provisioning.Service{
						ID:            serviceID,
						Name:          payload.Name,
						Region:        payload.Region,
						Provider:      payload.Provider,
						Tier:          "powerplus",
						Topology:      payload.Topology,
						Version:       payload.Version,
						Architecture:  payload.Architecture,
						Size:          payload.Size,
						Nodes:         int(payload.Nodes),
						MaxscaleNodes: payload.MaxscaleNodes,
						SSLEnabled:    payload.SSLEnabled,
						NosqlEnabled:  payload.NoSQLEnabled,
						FQDN:          "",
						Status:        "pending_create",
						CreatedOn:     int(time.Now().Unix()),
						UpdatedOn:     int(time.Now().Unix()),
						CreatedBy:     uuid.New().String(),
						UpdatedBy:     uuid.New().String(),
						Endpoints: []provisioning.Endpoint{
							{
								Name: "primary",
								Ports: []provisioning.Port{
									{
										Name:    "readwrite",
										Port:    3306,
										Purpose: "readwrite",
									},
									{
										Name:    "readonly",
										Port:    3307,
										Purpose: "readonly",
									},
								},
							},
						},
						StorageVolume: struct {
							Size       int    `json:"size"`
							VolumeType string `json:"volume_type"`
							IOPS       int    `json:"iops"`
							Throughput int    `json:"throughput"`
						}{
							Size:       int(payload.Storage),
							VolumeType: "pd-ssd",
							IOPS:       int(payload.VolumeIOPS),
							Throughput: int(payload.VolumeThroughput),
						},
						OutboundIps:        nil,
						IsActive:           true,
						ServiceType:        payload.ServiceType,
						ReplicationEnabled: false,
						PrimaryHost:        "",
					}
					r.NoError(json.NewEncoder(w).Encode(service))
					w.WriteHeader(http.StatusCreated)
				})
				for i := 0; i <= 2; i++ {
					expectRequest(func(w http.ResponseWriter, req *http.Request) {
						r.Equal(http.MethodGet, req.Method)
						r.Equal("/provisioning/v1/services/"+serviceID, req.URL.Path)
						w.Header().Set("Content-Type", "application/json")
						service.Status = "ready"
						r.NoError(json.NewEncoder(w).Encode(service))
						w.WriteHeader(http.StatusOK)
					})
				}
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodDelete, req.Method)
					r.Equal("/provisioning/v1/services/"+serviceID, req.URL.Path)
					w.WriteHeader(http.StatusAccepted)
					w.Header().Set("Content-Type", "application/json")
				})
				expectRequest(func(w http.ResponseWriter, req *http.Request) {
					r.Equal(http.MethodGet, req.Method)
					r.Equal("/provisioning/v1/services/"+serviceID, req.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					json.NewEncoder(w).Encode(&skysql.ErrorResponse{
						Code: http.StatusNotFound,
					})
				})
			},
			checks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("skysql_service.default", "id", serviceID),
				resource.TestCheckResourceAttr("skysql_service.default", "topology", "galera"),
				resource.TestCheckResourceAttr("skysql_service.default", "nodes", "3"),
				resource.TestCheckResourceAttr("skysql_service.default", "maxscale_nodes", "1"),
			},
		},
	}

	for _, test := range tests {
		{
			t.Run(test.name, func(t *testing.T) {
				r := require.New(t)
				test.before(r)
				resource.Test(t, resource.TestCase{
					IsUnitTest: true,
					ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
						"skysql": providerserver.NewProtocol6WithError(New("")()),
					},
					Steps: []resource.TestStep{
						{
							Config:      test.testResource,
							Check:       resource.ComposeAggregateTestCheckFunc(test.checks...),
							ExpectError: test.expectError,
						},
					},
				})
			})
		}
	}
}
