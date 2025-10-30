package stdagent

import (
	"encoding/json"
	"fmt"

	"github.com/fulcrumproject/agent-lib-go/pkg/agent"
)

// FulcrumClient defines the interface for communication with the Fulcrum Core API
type FulcrumClient interface {
	UpdateAgentStatus(status agent.AgentStatus) error
	GetAgentInfo() (*agent.AgentInfo[json.RawMessage], error)
	GetPendingJobs() ([]*agent.RawJob, error)
	ClaimJob(jobID string) error
	CompleteJob(jobID string, resources any) error
	FailJob(jobID string, errorMessage string) error
	ReportMetric(metrics *agent.MetricEntry) error
	ListServices(pagination *agent.PaginationOptions) (*agent.PageResponse[*agent.RawService], error)
	GetSecret(reference string) (any, error) // Returns (value, nil) on success, (nil, ErrSecretNotFound) on 404, (nil, error) on other errors
}

// ErrSecretNotFound is returned when a secret is not found in the vault
var ErrSecretNotFound = fmt.Errorf("secret not found")
