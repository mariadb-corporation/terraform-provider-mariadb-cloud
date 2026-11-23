package provider

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// replication_enabled and primary_host were removed, so a configuration that still
// sets them fails to plan instead of being silently ignored.
func TestServiceResourceRemovedAttributes(t *testing.T) {
	testURL, _, closeAPI := mockSkySQLAPI(t)
	defer closeAPI()
	os.Setenv("TF_SKYSQL_API_KEY", "[api-key]")
	os.Setenv("TF_SKYSQL_API_BASE_URL", testURL)

	configureOnce.Reset()

	config := `
		resource "skysql_service" default {
			service_type   = "transactional"
			topology       = "es-single"
			cloud_provider = "gcp"
			region         = "us-central1"
			name           = "test-removed"
			architecture   = "amd64"
			nodes          = 1
			size           = "sky-2x8"
			storage        = 100
			ssl_enabled    = true
			version        = "10.6.11-6-1"
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
				Config:      fmt.Sprintf(config, `replication_enabled = false`),
				ExpectError: regexp.MustCompile(`An argument named "replication_enabled" is not expected here`),
			},
			{
				Config:      fmt.Sprintf(config, `primary_host = "dbpgf00000001"`),
				ExpectError: regexp.MustCompile(`An argument named "primary_host" is not expected here`),
			},
		},
	})
}
