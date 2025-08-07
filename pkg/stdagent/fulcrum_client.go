package stdagent

import "github.com/fulcrumproject/agent-lib-go/pkg/agent"

// FulcrumClient defines the interface for communication with the Fulcrum Core API
type FulcrumClient[P, C any] interface {
	UpdateAgentStatus(status agent.AgentStatus) error
	GetAgentInfo() (*agent.AgentInfo[C], error)
	GetPendingJobs() ([]*agent.Job[P], error)
	ClaimJob(jobID string) error
	CompleteJob(jobID string, resources any) error
	FailJob(jobID string, errorMessage string) error
	UnsupportedJob(jobID string, errorMessage string) error
	ReportMetric(metrics *agent.MetricEntry) error
}
