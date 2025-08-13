package agent

import (
	"context"
	"encoding/json"
	"time"
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
	JobActionServiceCreate JobAction = "Create"
	JobActionServiceStart  JobAction = "Start"
	JobActionServiceStop   JobAction = "Stop"
	JobActionServiceUpdate JobAction = "Update"
	JobActionServiceDelete JobAction = "Delete"
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

// Job represents a job from the Fulcrum Core job queue
type Job[P any] struct {
	ID       string    `json:"id"`
	Action   JobAction `json:"action"`
	Status   JobStatus `json:"status"`
	Params   *P        `json:"params"`
	Priority int       `json:"priority"`
	Service  *Service  `json:"service"`
}

// ServiceStatus represents the possible status of a Service
type ServiceStatus string

const (
	ServiceStatusNew     ServiceStatus = "New"
	ServiceStatusStarted ServiceStatus = "Started"
	ServiceStatusStopped ServiceStatus = "Stopped"
	ServiceStatusDeleted ServiceStatus = "Deleted"
)

type Service struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Status        ServiceStatus          `json:"status"`
	Properties    map[string]interface{} `json:"properties"`
	Resources     map[string]interface{} `json:"resources"`
	ExternalID    *string                `json:"externalId"`
	ProviderID    string                 `json:"providerId"`
	ConsumerID    string                 `json:"consumerId"`
	AgentID       string                 `json:"agentId"`
	ServiceTypeID string                 `json:"serviceTypeId"`
	GroupID       string                 `json:"groupId"`
	CreatedAt     time.Time              `json:"createdAt"`
	UpdatedAt     time.Time              `json:"updatedAt"`
}

// PaginationOptions represents options for paginated requests
type PaginationOptions struct {
	Page     int `json:"page,omitempty"`
	PageSize int `json:"pageSize,omitempty"`
}

// PageResponse represents a paginated response
type PageResponse[T any] struct {
	Items      []T `json:"items"`
	TotalItems int `json:"totalItems"`
	TotalPages int `json:"totalPages"`
	PageSize   int `json:"pageSize"`
	Page       int `json:"page"`
}

type RawJob Job[json.RawMessage]

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

type RawJobResponse JobResponse[json.RawMessage]

// JobHandler is the function type for the job handler
// that is called when a job is received from the Fulcrum Core API
// and is used to process the job
type JobHandler[P any, R any] func(ctx context.Context, job *Job[P]) (*JobResponse[R], error)

type RawJobHandler func(ctx context.Context, job *RawJob) (*RawJobResponse, error)

// MetricsReporter is the function type for the metrics reporter
// that is called periodically to report metrics
type MetricsReporter func(ctx context.Context, service *Service) (metrics []MetricEntry, err error)

// HealthHandler is the function type for the health handler
// that is called periodically to check the health of the agent
type HealthHandler func(ctx context.Context) error

// ConnectHandler is the function type for the connect handler
// that is called when the agent is connected to the Fulcrum Core API
// and is used to initialize the agent with the remote configuration
// if it's available
type ConnectHandler[C any] func(ctx context.Context, info *AgentInfo[C]) error

type RawConnectHandler ConnectHandler[json.RawMessage]

// Agent is the interface for the agent
type Agent interface {
	Run(ctx context.Context) error
	Shutdown(ctx context.Context) error
	OnConnect(handler RawConnectHandler) error
	OnHealth(handler HealthHandler) error
	OnJob(action JobAction, handler RawJobHandler) error
	OnMetrics(handler MetricsReporter) error
}

// JobHandlerWrapper wraps a typed job handler to return a raw job handler
func JobHandlerWrapper[P any, R any](handler JobHandler[P, R]) RawJobHandler {
	return func(ctx context.Context, job *RawJob) (*RawJobResponse, error) {
		var params P
		if job.Params != nil {
			if err := json.Unmarshal(*job.Params, &params); err != nil {
				return nil, err
			}
		}

		resp, err := handler(ctx, &Job[P]{
			ID:       job.ID,
			Action:   job.Action,
			Status:   job.Status,
			Params:   &params,
			Priority: job.Priority,
			Service:  job.Service,
		})
		if err != nil {
			return nil, err
		}

		// Convert the typed response back to raw JSON
		var resourcesRaw json.RawMessage
		if resp.Resources != nil {
			resourcesBytes, err := json.Marshal(resp.Resources)
			if err != nil {
				return nil, err
			}
			resourcesRaw = json.RawMessage(resourcesBytes)
		}

		return &RawJobResponse{
			Resources:  &resourcesRaw,
			ExternalID: resp.ExternalID,
		}, nil
	}
}

// ConnectHandlerWrapper wraps a typed connect handler to return a raw connect handler
func ConnectHandlerWrapper[C any](handler ConnectHandler[C]) RawConnectHandler {
	return func(ctx context.Context, info *AgentInfo[json.RawMessage]) error {
		var config C
		if info.Config != nil {
			if err := json.Unmarshal(*info.Config, &config); err != nil {
				return err
			}
		}

		return handler(ctx, &AgentInfo[C]{
			ID:          info.ID,
			Name:        info.Name,
			Status:      info.Status,
			AgentTypeID: info.AgentTypeID,
			Config:      &config,
		})
	}
}
