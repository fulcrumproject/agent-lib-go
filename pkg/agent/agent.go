package agent

import (
	"context"
)

// AgentStatus represents the possible statuss of an Agent
type AgentStatus string

const (
	AgentStatusNew          AgentStatus = "New"
	AgentStatusConnected    AgentStatus = "Connected"
	AgentStatusDisconnected AgentStatus = "Disconnected"
	AgentStatusError        AgentStatus = "Error"
	AgentStatusDisabled     AgentStatus = "Disabled"
)

// JobAction represents the type of job
type JobAction string

const (
	JobActionServiceCreate     JobAction = "ServiceCreate"
	JobActionServiceStart      JobAction = "ServiceStart"
	JobActionServiceStop       JobAction = "ServiceStop"
	JobActionServiceHotUpdate  JobAction = "ServiceHotUpdate"
	JobActionServiceColdUpdate JobAction = "ServiceColdUpdate"
	JobActionServiceDelete     JobAction = "ServiceDelete"
)

// JobStatus represents the status of a job
type JobStatus string

const (
	JobStatusPending    JobStatus = "Pending"
	JobStatusProcessing JobStatus = "Processing"
	JobStatusCompleted  JobStatus = "Completed"
	JobStatusFailed     JobStatus = "Failed"
)

// UnsupportedJobError represents an error when a job cannot be supported by the agent
type UnsupportedJobError struct {
	Msg string
}

func (e *UnsupportedJobError) Error() string {
	return e.Msg
}

type Service[P any] struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	ExternalID        *string `json:"externalId"`
	CurrentProperties *P      `json:"currentProperties"`
	TargetProperties  *P      `json:"targetProperties"`
}

// Job represents a job from the Fulcrum Core job queue
type Job[P any] struct {
	ID       string     `json:"id"`
	Action   JobAction  `json:"action"`
	Status   JobStatus  `json:"status"`
	Priority int        `json:"priority"`
	Service  Service[P] `json:"service"`
}

// MetricEntry represents a single metric measurement
type MetricEntry struct {
	ExternalID string  `json:"externalId"`
	ResourceID string  `json:"resourceId"`
	Value      float64 `json:"value"`
	TypeName   string  `json:"typeName"`
}

// AgentInfo represents the agent information returned by the Fulcrum Core API
type AgentInfo[C any] struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Status      AgentStatus `json:"status"`
	AgentTypeID string      `json:"agentTypeId"`
	Config      *C          `json:"configuration"`
}

type JobResponse[R any] struct {
	Resources  *R      `json:"resources"`
	ExternalID *string `json:"externalId"`
}

type JobHandler[P any, R any] func(ctx context.Context, job *Job[P]) (*JobResponse[R], error)

type MetricsReporter[P any] func(ctx context.Context) (metrics []MetricEntry, err error)

type HeartbeatHandler func(ctx context.Context) error

type ConnectHandler[C any] func(ctx context.Context, info *AgentInfo[C]) error

type Agent[P, R, C any] interface {
	Run(ctx context.Context) error
	Shutdown(ctx context.Context) error
	OnConnect(handler ConnectHandler[C]) error
	OnHeartbeat(handler HeartbeatHandler) error
	OnJob(action JobAction, handler JobHandler[P, R]) error
	OnMetricsReport(handler MetricsReporter[P]) error
}
