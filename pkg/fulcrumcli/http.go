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
type HTTPClient[P, C any] struct {
	client *resty.Client
	token  string // Agent authentication token
}

// NewHTTPClient creates a new Fulcrum API client
func NewHTTPClient[P, C any](baseURL string, token string) *HTTPClient[P, C] {
	client := resty.New().
		SetBaseURL(baseURL).
		SetTimeout(30*time.Second).
		SetHeader("Content-Type", "application/json").
		SetAuthToken(token).
		SetDisableWarn(true)

	return &HTTPClient[P, C]{
		client: client,
		token:  token,
	}
}

// UpdateAgentStatus updates the agent's status in Fulcrum Core
func (c *HTTPClient[P, C]) UpdateAgentStatus(status agent.AgentStatus) error {
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
func (c *HTTPClient[P, C]) GetAgentInfo() (*agent.AgentInfo[C], error) {
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

	var result agent.AgentInfo[C]
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to decode agent info response: %w", err)
	}

	return &result, nil
}

// GetPendingJobs retrieves pending jobs for this agent
func (c *HTTPClient[P, C]) GetPendingJobs() ([]*agent.Job[P], error) {
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

	var jobs []*agent.Job[P]
	if err := json.Unmarshal(bodyBytes, &jobs); err != nil {
		return nil, fmt.Errorf("failed to decode jobs response: %w", err)
	}

	return jobs, nil
}

// ClaimJob claims a job for processing
func (c *HTTPClient[P, C]) ClaimJob(jobID string) error {
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
func (c *HTTPClient[P, C]) CompleteJob(jobID string, response any) error {
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
func (c *HTTPClient[P, C]) FailJob(jobID string, errorMessage string) error {
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

// ReportMetric sends collected metrics to Fulcrum Core
func (c *HTTPClient[P, C]) ReportMetric(metric *agent.MetricEntry) error {
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
