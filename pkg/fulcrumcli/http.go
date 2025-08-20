package fulcrumcli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/fulcrumproject/agent-lib-go/pkg/agent"
	"resty.dev/v3"
)

// HTTPClient implements FulcrumClient interface using Resty v3
type HTTPClient struct {
	client *resty.Client
	token  string // Agent authentication token
}

// NewHTTPClient creates a new Fulcrum API client
func NewHTTPClient(baseURL string, token string) *HTTPClient {
	client := resty.New().
		SetBaseURL(baseURL).
		SetTimeout(30*time.Second).
		SetHeader("Content-Type", "application/json").
		SetAuthToken(token).
		SetDisableWarn(true)

	return &HTTPClient{
		client: client,
		token:  token,
	}
}

// UpdateAgentStatus updates the agent's status in Fulcrum Core
func (c *HTTPClient) UpdateAgentStatus(status agent.AgentStatus) error {
	resp, err := c.client.R().
		SetBody(map[string]any{
			"status": status,
		}).
		Put("/api/v1/agents/me/status")

	if err != nil {
		return fmt.Errorf("failed to update agent status: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("failed to update agent status, status: %d", resp.StatusCode())
	}

	return nil
}

// GetAgentInfo retrieves the agent's information from Fulcrum Core
func (c *HTTPClient) GetAgentInfo() (*agent.AgentInfo[json.RawMessage], error) {
	resp, err := c.client.R().
		Get("/api/v1/agents/me")

	if err != nil {
		return nil, fmt.Errorf("failed to get agent info: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to get agent info, status: %d", resp.StatusCode())
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result agent.AgentInfo[json.RawMessage]
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to decode agent info response: %w", err)
	}

	return &result, nil
}

// GetPendingJobs retrieves pending jobs for this agent
func (c *HTTPClient) GetPendingJobs() ([]*agent.RawJob, error) {
	resp, err := c.client.R().
		Get("/api/v1/jobs/pending")

	if err != nil {
		return nil, fmt.Errorf("failed to get pending jobs: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to get pending jobs, status: %d", resp.StatusCode())
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var jobs []*agent.RawJob
	if err := json.Unmarshal(bodyBytes, &jobs); err != nil {
		return nil, fmt.Errorf("failed to decode jobs response: %w", err)
	}

	return jobs, nil
}

// ClaimJob claims a job for processing
func (c *HTTPClient) ClaimJob(jobID string) error {
	resp, err := c.client.R().
		Post(fmt.Sprintf("/api/v1/jobs/%s/claim", jobID))

	if err != nil {
		return fmt.Errorf("failed to claim job: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("failed to claim job, status: %d", resp.StatusCode())
	}

	return nil
}

// CompleteJob marks a job as completed with results
func (c *HTTPClient) CompleteJob(jobID string, response any) error {
	resp, err := c.client.R().
		SetBody(response).
		Post(fmt.Sprintf("/api/v1/jobs/%s/complete", jobID))

	if err != nil {
		return fmt.Errorf("failed to complete job: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("failed to complete job, status: %d", resp.StatusCode())
	}

	return nil
}

// FailJob marks a job as failed with an error message
func (c *HTTPClient) FailJob(jobID string, errorMessage string) error {
	resp, err := c.client.R().
		SetBody(map[string]any{
			"errorMessage": errorMessage,
		}).
		Post(fmt.Sprintf("/api/v1/jobs/%s/fail", jobID))

	if err != nil {
		return fmt.Errorf("failed to mark job as failed: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("failed to mark job as failed, status: %d", resp.StatusCode())
	}

	return nil
}

// UnsupportedJob marks a job as unsupported by this agent
func (c *HTTPClient) UnsupportedJob(jobID string, errorMessage string) error {
	resp, err := c.client.R().
		SetBody(map[string]any{
			"errorMessage": errorMessage,
		}).
		Post(fmt.Sprintf("/api/v1/jobs/%s/unsupported", jobID))

	if err != nil {
		return fmt.Errorf("failed to mark job as unsupported: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("failed to mark job as unsupported, status: %d", resp.StatusCode())
	}

	return nil
}

// ReportMetric sends collected metrics to Fulcrum Core
func (c *HTTPClient) ReportMetric(metric *agent.MetricEntry) error {
	resp, err := c.client.R().
		SetBody(metric).
		Post("/api/v1/metric-entries")

	if err != nil {
		return fmt.Errorf("failed to report metrics: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("failed to report metrics, status: %d", resp.StatusCode())
	}

	return nil
}

// ListServices retrieves services for this agent (filtered by API using agent token)
func (c *HTTPClient) ListServices(pagination *agent.PaginationOptions) (*agent.PageResponse[*agent.RawService], error) {
	request := c.client.R()

	// Add pagination query parameters if provided
	if pagination != nil {
		if pagination.Page > 0 {
			request = request.SetQueryParam("page", fmt.Sprintf("%d", pagination.Page))
		}
		if pagination.PageSize > 0 {
			request = request.SetQueryParam("pageSize", fmt.Sprintf("%d", pagination.PageSize))
		}
	}

	resp, err := request.Get("/api/v1/services")
	if err != nil {
		return nil, fmt.Errorf("failed to get services: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to get services, status: %d", resp.StatusCode())
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// The API returns a paginated response matching the PageResponse structure
	var response agent.PageResponse[*agent.RawService]
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("failed to decode services response: %w", err)
	}

	return &response, nil
}
