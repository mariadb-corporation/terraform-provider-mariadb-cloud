package skysql

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/logging"

	"github.com/skysqlinc/terraform-provider-skysql/internal/skysql/autonomous"
	"github.com/skysqlinc/terraform-provider-skysql/internal/skysql/organization"
	"github.com/skysqlinc/terraform-provider-skysql/internal/skysql/provisioning"
)

type Client struct {
	HTTPClient *resty.Client

	// pendingRetryInterval and pendingRetryTimeout control how long mutating
	// operations wait out a transient "service is in a pending state" rejection
	// from the backend. Defaulted in New; overridable in tests. See
	// doWithPendingRetry.
	pendingRetryInterval time.Duration
	pendingRetryTimeout  time.Duration
}

func New(baseURL string, apiKey string, orgID string) *Client {
	transport := logging.NewLoggingHTTPTransport(http.DefaultTransport)

	clientName, _ := os.Executable()

	httpClient := resty.NewWithClient(&http.Client{Transport: transport}).
		SetHeader("User-Agent", filepath.Base(clientName)).
		SetHeader("X-API-Key", apiKey).
		SetBaseURL(baseURL)

	if orgID != "" {
		httpClient.SetHeader("X-MDB-Org", orgID)
	}

	return &Client{
		HTTPClient: httpClient.
			// Set retry count to non-zero to enable retries.
			SetRetryCount(3).
			// Default is 100 milliseconds.
			SetRetryWaitTime(5 * time.Second).
			// MaxWaitTime can be overridden as well.
			// Default is 2 seconds.
			SetRetryMaxWaitTime(20 * time.Second).
			// Honor Retry-After header on 429 responses;
			// fall back to default exponential backoff with jitter.
			SetRetryAfter(func(client *resty.Client, resp *resty.Response) (time.Duration, error) {
				if resp.StatusCode() == http.StatusTooManyRequests {
					if ra := resp.Header().Get("Retry-After"); ra != "" {
						if seconds, err := strconv.Atoi(ra); err == nil {
							return time.Duration(seconds) * time.Second, nil
						}
					}
				}
				// Return 0 to let resty use its default backoff.
				return 0, nil
			}).
			AddRetryCondition(
				func(r *resty.Response, err error) bool {
					if err != nil {
						return true
					}
					switch r.StatusCode() {
					case http.StatusTooManyRequests,
						http.StatusInternalServerError,
						http.StatusBadGateway,
						http.StatusServiceUnavailable,
						http.StatusGatewayTimeout:
						return true
					}
					return false
				}).
			EnableTrace(),
		pendingRetryInterval: defaultPendingRetryInterval,
		pendingRetryTimeout:  defaultPendingRetryTimeout,
	}
}

func (c *Client) GetProjects(ctx context.Context) ([]organization.Project, error) {
	resp, err := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetResult([]organization.Project{}).
		SetError(&ErrorResponse{}).
		SetContext(ctx).
		Get("/organization/v1/projects")
	if resp.IsError() {
		return nil, handleError(resp)
	}
	return *resp.Result().(*[]organization.Project), err
}

func WithPageSize(value uint) func(url.Values) {
	return func(values url.Values) {
		values.Set("page_size", strconv.Itoa(int(value)))
	}
}

func (c *Client) GetVersions(ctx context.Context, options ...func(url.Values)) ([]provisioning.Version, error) {
	request := c.HTTPClient.R()
	for _, option := range options {
		option(request.QueryParam)
	}
	resp, err := request.
		SetHeader("Accept", "application/json").
		SetResult([]provisioning.Version{}).
		SetError(&ErrorResponse{}).
		SetContext(ctx).
		Get("/provisioning/v1/versions")
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, handleError(resp)
	}
	return *resp.Result().(*[]provisioning.Version), err
}

func (c *Client) GetTopologies(ctx context.Context, options ...func(url.Values)) ([]provisioning.Topology, error) {
	request := c.HTTPClient.R()
	for _, option := range options {
		option(request.QueryParam)
	}
	resp, err := request.
		SetHeader("Accept", "application/json").
		SetResult([]provisioning.Topology{}).
		SetError(&ErrorResponse{}).
		SetContext(ctx).
		Get("/provisioning/v1/topologies")
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, handleError(resp)
	}
	return *resp.Result().(*[]provisioning.Topology), err
}

func (c *Client) GetServiceByID(ctx context.Context, serviceID string) (*provisioning.Service, error) {
	resp, err := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetResult(provisioning.Service{}).
		SetError(&ErrorResponse{}).
		SetContext(ctx).
		Get("/provisioning/v1/services/" + serviceID)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, handleError(resp)
	}
	return resp.Result().(*provisioning.Service), err
}

func (c *Client) CreateService(ctx context.Context, req *provisioning.CreateServiceRequest) (*provisioning.Service, error) {
	resp, err := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetResult(provisioning.Service{}).
		SetError(&ErrorResponse{}).
		SetContext(ctx).
		SetBody(req).
		Post("/provisioning/v1/services")
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, handleError(resp)
	}

	return resp.Result().(*provisioning.Service), err
}

func (c *Client) DeleteServiceByID(ctx context.Context, serviceID string) error {
	resp, err := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetError(&ErrorResponse{}).
		SetContext(ctx).
		Delete("/provisioning/v1/services/" + serviceID)
	if err != nil {
		return err
	}

	if resp.IsError() {
		return handleError(resp)
	}

	return nil
}

func (c *Client) GetServiceCredentialsByID(ctx context.Context, serviceID string) (*provisioning.Credentials, error) {
	resp, err := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetResult(provisioning.Credentials{}).
		SetError(&ErrorResponse{}).
		SetContext(ctx).
		Get("/provisioning/v1/services/" + serviceID + "/security/credentials")
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, handleError(resp)
	}
	return resp.Result().(*provisioning.Credentials), err
}

func (c *Client) UpdateServiceAllowListByID(ctx context.Context, serviceID string, allowlist []provisioning.AllowListItem) ([]provisioning.AllowListItem, error) {
	var result []provisioning.AllowListItem
	err := c.doWithPendingRetry(ctx, func() error {
		resp, err := c.HTTPClient.R().
			SetHeader("Accept", "application/json").
			SetResult(provisioning.ReadAllowListResponse{}).
			SetError(&ErrorResponse{}).
			SetContext(ctx).
			SetBody(allowlist).
			Put("/provisioning/v1/services/" + serviceID + "/security/allowlist")
		if err != nil {
			return err
		}
		if resp.IsError() {
			return handleError(resp)
		}
		response := *resp.Result().(*provisioning.ReadAllowListResponse)
		result = response[0].AllowList
		return nil
	})

	return result, err
}

func (c *Client) ReadServiceAllowListByID(ctx context.Context, serviceID string) (provisioning.ReadAllowListResponse, error) {
	resp, err := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetContext(ctx).
		SetResult(provisioning.ReadAllowListResponse{}).
		SetError(&ErrorResponse{}).
		Get("/provisioning/v1/services/" + serviceID + "/security/allowlist")
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, handleError(resp)
	}
	response := *resp.Result().(*provisioning.ReadAllowListResponse)
	if response == nil {
		response = make(provisioning.ReadAllowListResponse, 0)
	}
	return response, err
}

func handleError(resp *resty.Response) error {
	if resp.StatusCode() == http.StatusNotFound {
		return ErrorServiceNotFound
	}
	if resp.StatusCode() == http.StatusUnauthorized {
		return ErrorUnauthorized
	}

	detail := ""
	if resp.Error() != nil {
		if errResp, ok := resp.Error().(*ErrorResponse); ok && len(errResp.Errors) > 0 {
			detail = errResp.Errors[0].Message
		}
	}

	// Classify the DPS status-gate rejection (service is mid-operation) into a
	// single sentinel so mutating calls can wait it out and retry (MCDEV-3899).
	// Current DPS returns it as a 400 with a distinctive message; newer DPS
	// returns a 409 Conflict. Matching the status (and the legacy message for
	// backward compatibility) keeps detection off the fragile message string.
	if resp.StatusCode() == http.StatusConflict ||
		strings.Contains(strings.ToLower(detail), pendingStateMarker) {
		if detail != "" {
			return fmt.Errorf("%w (SkySQL API %d: %s)", ErrorServiceInPendingState, resp.StatusCode(), detail)
		}
		return fmt.Errorf("%w (SkySQL API %d)", ErrorServiceInPendingState, resp.StatusCode())
	}

	if detail != "" {
		return fmt.Errorf("SkySQL API %d: %s", resp.StatusCode(), detail)
	}
	return fmt.Errorf("SkySQL API error: %s", resp.Status())
}

func (c *Client) SetServicePowerState(ctx context.Context, serviceID string, isActive bool) error {
	return c.doWithPendingRetry(ctx, func() error {
		resp, err := c.HTTPClient.R().
			SetHeader("Accept", "application/json").
			SetContext(ctx).
			SetBody(&provisioning.PowerStateRequest{IsActive: isActive}).
			SetError(&ErrorResponse{}).
			Post("/provisioning/v1/services/" + serviceID + "/power")
		if err != nil {
			return err
		}
		if resp.IsError() {
			return handleError(resp)
		}

		return nil
	})
}

func (c *Client) ModifyServiceEndpoints(
	ctx context.Context,
	serviceID string,
	mechanism string,
	allowedAccounts []string,
	visibility string,
) (*provisioning.ServiceEndpoint, error) {
	var result *provisioning.ServiceEndpoint
	err := c.doWithPendingRetry(ctx, func() error {
		resp, err := c.HTTPClient.R().
			SetHeader("Accept", "application/json").
			SetContext(ctx).
			SetBody(&provisioning.PatchServiceEndpointsRequest{
				{Mechanism: mechanism,
					AllowedAccounts: allowedAccounts,
					Visibility:      visibility},
			}).
			SetResult(provisioning.PatchServiceEndpointsResponse{}).
			SetError(&ErrorResponse{}).
			Patch("/provisioning/v1/services/" + serviceID + "/endpoints")
		if err != nil {
			return err
		}
		if resp.IsError() {
			return handleError(resp)
		}
		response := *resp.Result().(*provisioning.PatchServiceEndpointsResponse)
		if response == nil {
			response = make(provisioning.PatchServiceEndpointsResponse, 0)
		}
		result = &response[0]
		return nil
	})

	return result, err
}

func (c *Client) ModifyServiceSize(ctx context.Context, serviceID string, size string) error {
	return c.doWithPendingRetry(ctx, func() error {
		resp, err := c.HTTPClient.R().
			SetHeader("Accept", "application/json").
			SetContext(ctx).
			SetBody(&provisioning.UpdateServiceSizeRequest{Size: size}).
			SetError(&ErrorResponse{}).
			Post("/provisioning/v1/services/" + serviceID + "/size")
		if err != nil {
			return err
		}
		if resp.IsError() {
			return handleError(resp)
		}

		return nil
	})
}

func (c *Client) ModifyServiceNodeNumber(ctx context.Context, serviceID string, req *provisioning.UpdateServiceNodesNumberRequest) error {
	return c.doWithPendingRetry(ctx, func() error {
		resp, err := c.HTTPClient.R().
			SetHeader("Accept", "application/json").
			SetContext(ctx).
			SetBody(req).
			SetError(&ErrorResponse{}).
			Post("/provisioning/v1/services/" + serviceID + "/nodes")
		if err != nil {
			return err
		}
		if resp.IsError() {
			return handleError(resp)
		}

		return nil
	})
}

func (c *Client) ModifyServiceStorage(ctx context.Context, serviceID string, size int64, iops int64, throughput int64) error {
	return c.doWithPendingRetry(ctx, func() error {
		resp, err := c.HTTPClient.R().
			SetHeader("Accept", "application/json").
			SetContext(ctx).
			SetBody(&provisioning.UpdateStorageRequest{Size: size, IOPS: iops, Throughput: throughput}).
			SetError(&ErrorResponse{}).
			Patch("/provisioning/v1/services/" + serviceID + "/storage")
		if err != nil {
			return err
		}
		if resp.IsError() {
			return handleError(resp)
		}

		return nil
	})
}

func (c *Client) UpdateServiceTags(ctx context.Context, serviceID string, tags map[string]string) error {
	return c.doWithPendingRetry(ctx, func() error {
		resp, err := c.HTTPClient.R().
			SetHeader("Accept", "application/json").
			SetContext(ctx).
			SetBody(&provisioning.UpdateServiceTagsRequest{Tags: tags}).
			SetError(&ErrorResponse{}).
			Patch("/provisioning/v1/services/" + serviceID + "/tags")
		if err != nil {
			return err
		}
		if resp.IsError() {
			return handleError(resp)
		}

		return nil
	})
}

func (c *Client) SetAutonomousActions(
	ctx context.Context,
	value autonomous.SetAutonomousActionsRequest,
) ([]autonomous.ActionResponse, error) {
	resp, err := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetContext(ctx).
		SetBody(value).
		SetResult([]autonomous.ActionResponse{}).
		SetError(&ErrorResponse{}).
		Post("/als/v1/actions")
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, handleError(resp)
	}

	response := *resp.Result().(*[]autonomous.ActionResponse)
	if response == nil {
		response = make([]autonomous.ActionResponse, 0)
	}
	return response, err
}

func (c *Client) GetAutonomousActions(ctx context.Context, serviceID string) ([]autonomous.ActionResponse, error) {
	resp, err := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetContext(ctx).
		SetResult([]autonomous.ActionResponse{}).
		SetError(&ErrorResponse{}).
		SetQueryParam("service_id", serviceID).
		Get("/als/v1/actions")
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, handleError(resp)
	}

	response := *resp.Result().(*[]autonomous.ActionResponse)
	if response == nil {
		response = make([]autonomous.ActionResponse, 0)
	}
	return response, err
}

func (c *Client) DeleteAutonomousAction(ctx context.Context, actionID string) error {
	resp, err := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetContext(ctx).
		SetError(&ErrorResponse{}).
		Delete("/als/v1/actions/" + actionID)
	if err != nil {
		return err
	}
	if resp.IsError() {
		return handleError(resp)
	}

	return err
}

func (c *Client) GetAvailabilityZones(ctx context.Context, region string, options ...func(url.Values)) ([]provisioning.AvailabilityZone, error) {
	request := c.HTTPClient.R()
	for _, option := range options {
		option(request.QueryParam)
	}
	resp, err := request.
		SetHeader("Accept", "application/json").
		SetResult([]provisioning.AvailabilityZone{}).
		SetError(&ErrorResponse{}).
		SetContext(ctx).
		Get("/provisioning/v1/regions/" + region + "/zones")
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, handleError(resp)
	}
	return *resp.Result().(*[]provisioning.AvailabilityZone), err
}

func (c *Client) CreateConfig(ctx context.Context, req *provisioning.CreateConfigRequest) (*provisioning.Config, error) {
	resp, err := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetResult(provisioning.Config{}).
		SetError(&ErrorResponse{}).
		SetContext(ctx).
		SetBody(req).
		Post("/provisioning/v1/configs")
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, handleError(resp)
	}
	return resp.Result().(*provisioning.Config), nil
}

func (c *Client) GetConfigByID(ctx context.Context, configID string) (*provisioning.Config, error) {
	resp, err := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetResult(provisioning.Config{}).
		SetError(&ErrorResponse{}).
		SetContext(ctx).
		Get("/provisioning/v1/configs/" + configID)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, handleError(resp)
	}
	return resp.Result().(*provisioning.Config), nil
}

func (c *Client) UpdateConfig(ctx context.Context, configID string, req *provisioning.UpdateConfigRequest) (*provisioning.Config, error) {
	resp, err := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetResult(provisioning.Config{}).
		SetError(&ErrorResponse{}).
		SetContext(ctx).
		SetBody(req).
		Patch("/provisioning/v1/configs/" + configID)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, handleError(resp)
	}
	return resp.Result().(*provisioning.Config), nil
}

func (c *Client) DeleteConfig(ctx context.Context, configID string) error {
	resp, err := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetError(&ErrorResponse{}).
		SetContext(ctx).
		Delete("/provisioning/v1/configs/" + configID)
	if err != nil {
		return err
	}
	if resp.IsError() {
		return handleError(resp)
	}
	return nil
}

func (c *Client) SetConfigValue(ctx context.Context, configID string, variableName string, value string, allowRestart bool) error {
	r := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetError(&ErrorResponse{}).
		SetContext(ctx).
		SetBody(&provisioning.ConfigValueRequest{Value: value})

	if allowRestart {
		r.SetQueryParam("allow_restart", "true")
	}

	resp, err := r.Post("/provisioning/v1/configs/" + configID + "/values/" + variableName)
	if err != nil {
		return err
	}
	if resp.IsError() {
		return handleError(resp)
	}
	return nil
}

func (c *Client) ApplyConfigToService(ctx context.Context, serviceID string, configID string) error {
	return c.doWithPendingRetry(ctx, func() error {
		resp, err := c.HTTPClient.R().
			SetHeader("Accept", "application/json").
			SetError(&ErrorResponse{}).
			SetContext(ctx).
			SetBody(&provisioning.ServiceConfigState{ConfigID: configID}).
			Post("/provisioning/v1/services/" + serviceID + "/config")
		if err != nil {
			return err
		}
		if resp.IsError() {
			return handleError(resp)
		}
		return nil
	})
}

func (c *Client) RemoveConfigFromService(ctx context.Context, serviceID string) error {
	return c.doWithPendingRetry(ctx, func() error {
		resp, err := c.HTTPClient.R().
			SetHeader("Accept", "application/json").
			SetError(&ErrorResponse{}).
			SetContext(ctx).
			Delete("/provisioning/v1/services/" + serviceID + "/config")
		if err != nil {
			return err
		}
		if resp.IsError() {
			return handleError(resp)
		}
		return nil
	})
}

func (c *Client) GetConfigKeysByTopology(ctx context.Context, topologyName string, version string) ([]provisioning.ConfigKey, error) {
	var result []provisioning.ConfigKey
	r := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetResult(&result).
		SetError(&ErrorResponse{}).
		SetContext(ctx)

	if version != "" {
		r.SetQueryParam("version", version)
	}

	resp, err := r.Get("/provisioning/v1/topologies/" + topologyName + "/configs")
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, handleError(resp)
	}
	return result, nil
}

func (c *Client) UnsetConfigValue(ctx context.Context, configID string, variableName string, allowRestart bool) error {
	r := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetError(&ErrorResponse{}).
		SetContext(ctx)

	if allowRestart {
		r.SetQueryParam("allow_restart", "true")
	}

	resp, err := r.Delete("/provisioning/v1/configs/" + configID + "/values/" + variableName)
	if err != nil {
		return err
	}
	if resp.IsError() {
		return handleError(resp)
	}
	return nil
}

// SetConfigReplicaValue sets the value of a variable for the replicas of a config. It calls
// POST /configs/{id}/replica-values/{variable_name}. DPS answers 404 when the replica values are
// not turned on, which this client reports as ErrorServiceNotFound.
func (c *Client) SetConfigReplicaValue(ctx context.Context, configID string, variableName string, value string) error {
	resp, err := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetError(&ErrorResponse{}).
		SetContext(ctx).
		SetBody(&provisioning.ConfigValueRequest{Value: value}).
		Post("/provisioning/v1/configs/" + configID + "/replica-values/" + variableName)
	if err != nil {
		return err
	}
	if resp.IsError() {
		return handleError(resp)
	}
	return nil
}

// UnsetConfigReplicaValue removes the replica value of a variable, so that the replicas follow the
// primary again. It calls DELETE /configs/{id}/replica-values/{variable_name}.
func (c *Client) UnsetConfigReplicaValue(ctx context.Context, configID string, variableName string) error {
	resp, err := c.HTTPClient.R().
		SetHeader("Accept", "application/json").
		SetError(&ErrorResponse{}).
		SetContext(ctx).
		Delete("/provisioning/v1/configs/" + configID + "/replica-values/" + variableName)
	if err != nil {
		return err
	}
	if resp.IsError() {
		return handleError(resp)
	}
	return nil
}
