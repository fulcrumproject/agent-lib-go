package stdagent

import (
	"context"
	"testing"
	"time"

	"github.com/fulcrumproject/agent-lib-go/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type TestPayload struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

type TestResource struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// TestAgentInterface verifies that Agent implements the agent.Agent interface
func TestAgentInterface(t *testing.T) {
	mockClient := NewMockFulcrumClient[TestPayload](t)
	stdAgent, err := New[TestPayload, TestResource](mockClient)
	assert.NoError(t, err)

	// Verify that the standard agent implements the agent.Agent interface
	var _ agent.Agent[TestPayload, TestResource] = stdAgent
}

func TestNew(t *testing.T) {
	mockClient := NewMockFulcrumClient[TestPayload](t)

	t.Run("creates agent with default options", func(t *testing.T) {
		stdAgent, err := New[TestPayload, TestResource](mockClient)
		assert.NoError(t, err)
		assert.NotNil(t, stdAgent)
		assert.Equal(t, DefaultHeartbeatInterval, stdAgent.heartbeatInterval)
		assert.Equal(t, DefaultJobPollInterval, stdAgent.jobPollInterval)
		assert.Equal(t, DefaultMetricsReportInterval, stdAgent.metricsReportInterval)
	})

	t.Run("creates agent with custom options", func(t *testing.T) {
		customHeartbeat := 30 * time.Second
		customJobPoll := 10 * time.Second
		customMetrics := 60 * time.Second

		stdAgent, err := New(
			mockClient,
			WithHeartbeatInterval[TestPayload, TestResource](customHeartbeat),
			WithJobPollInterval[TestPayload, TestResource](customJobPoll),
			WithMetricsReportInterval[TestPayload, TestResource](customMetrics),
		)

		assert.NoError(t, err)
		assert.NotNil(t, stdAgent)
		assert.Equal(t, customHeartbeat, stdAgent.heartbeatInterval)
		assert.Equal(t, customJobPoll, stdAgent.jobPollInterval)
		assert.Equal(t, customMetrics, stdAgent.metricsReportInterval)
	})

	t.Run("fails with invalid options", func(t *testing.T) {
		_, err := New(
			mockClient,
			WithHeartbeatInterval[TestPayload, TestResource](-1*time.Second),
		)
		assert.Error(t, err)
	})
}

func TestAgent_OnJob(t *testing.T) {
	mockClient := NewMockFulcrumClient[TestPayload](t)
	stdAgent, err := New[TestPayload, TestResource](mockClient)
	assert.NoError(t, err)

	handler := func(ctx context.Context, job *agent.Job[TestPayload]) (*agent.JobResponse[TestResource], error) {
		return &agent.JobResponse[TestResource]{
			Resources: &TestResource{ID: "test-id", URL: "http://test.com"},
		}, nil
	}

	err = stdAgent.OnJob(agent.JobActionServiceCreate, handler)
	assert.NoError(t, err)
	assert.Contains(t, stdAgent.jobHandlers, agent.JobActionServiceCreate)
}

func TestAgent_OnMetricsReport(t *testing.T) {
	mockClient := NewMockFulcrumClient[TestPayload](t)
	stdAgent, err := New[TestPayload, TestResource](mockClient)
	assert.NoError(t, err)

	reporter := func(ctx context.Context) ([]agent.MetricEntry, error) {
		return []agent.MetricEntry{
			{ExternalID: "test", ResourceID: "res-1", Value: 100.0, TypeName: "cpu"},
		}, nil
	}

	err = stdAgent.OnMetricsReport(reporter)
	assert.NoError(t, err)
	assert.NotNil(t, stdAgent.metricsReporter)
}

func TestAgent_OnHeartbeat(t *testing.T) {
	mockClient := NewMockFulcrumClient[TestPayload](t)
	stdAgent, err := New[TestPayload, TestResource](mockClient)
	assert.NoError(t, err)

	handler := func(ctx context.Context) error {
		return nil
	}

	err = stdAgent.OnHeartbeat(handler)
	assert.NoError(t, err)
	assert.NotNil(t, stdAgent.heartbeatHandler)
}

func TestAgent_Run(t *testing.T) {
	t.Run("successful startup", func(t *testing.T) {
		mockClient := NewMockFulcrumClient[TestPayload](t)
		stdAgent, err := New[TestPayload, TestResource](mockClient)
		assert.NoError(t, err)

		// Mock the expected calls during startup
		mockClient.EXPECT().GetAgentInfo().Return(&agent.AgentInfo{
			ID:     "test-agent-123",
			Name:   "test-agent",
			Status: agent.AgentStatusConnected,
		}, nil).Once()

		mockClient.EXPECT().UpdateAgentStatus(agent.AgentStatusConnected).Return(nil).Once()

		ctx := context.Background()
		err = stdAgent.Run(ctx)
		assert.NoError(t, err)
		assert.Equal(t, "test-agent-123", stdAgent.GetAgentID())
		assert.Equal(t, agent.AgentStatusConnected, stdAgent.GetStatus())
	})

	t.Run("fails when GetAgentInfo returns error", func(t *testing.T) {
		mockClient := NewMockFulcrumClient[TestPayload](t)
		stdAgent, err := New[TestPayload, TestResource](mockClient)
		assert.NoError(t, err)

		mockClient.EXPECT().GetAgentInfo().Return(nil, assert.AnError).Once()

		ctx := context.Background()
		err = stdAgent.Run(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get agent information")
	})

	t.Run("fails when agent info is invalid", func(t *testing.T) {
		mockClient := NewMockFulcrumClient[TestPayload](t)
		stdAgent, err := New[TestPayload, TestResource](mockClient)
		assert.NoError(t, err)

		mockClient.EXPECT().GetAgentInfo().Return(&agent.AgentInfo{
			Name: "test-agent",
			// missing ID field to test error case
		}, nil).Once()

		ctx := context.Background()
		err = stdAgent.Run(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid agent information received")
	})

	t.Run("fails when UpdateAgentStatus returns error", func(t *testing.T) {
		mockClient := NewMockFulcrumClient[TestPayload](t)
		stdAgent, err := New[TestPayload, TestResource](mockClient)
		assert.NoError(t, err)

		mockClient.EXPECT().GetAgentInfo().Return(&agent.AgentInfo{
			ID: "test-agent-123",
		}, nil).Once()

		mockClient.EXPECT().UpdateAgentStatus(agent.AgentStatusConnected).Return(assert.AnError).Once()

		ctx := context.Background()
		err = stdAgent.Run(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update agent status")
	})
}

func TestAgent_Shutdown(t *testing.T) {
	t.Run("successful shutdown when connected", func(t *testing.T) {
		mockClient := NewMockFulcrumClient[TestPayload](t)
		stdAgent, err := New[TestPayload, TestResource](mockClient)
		assert.NoError(t, err)

		// Simulate connected state
		stdAgent.setStatus(agent.AgentStatusConnected)
		stdAgent.agentID = "test-agent-123"

		mockClient.EXPECT().UpdateAgentStatus(agent.AgentStatusDisconnected).Return(nil).Once()

		ctx := context.Background()
		err = stdAgent.Shutdown(ctx)
		assert.NoError(t, err)
		assert.Equal(t, agent.AgentStatusDisconnected, stdAgent.GetStatus())
	})

	t.Run("shutdown when not connected", func(t *testing.T) {
		mockClient := NewMockFulcrumClient[TestPayload](t)
		stdAgent, err := New[TestPayload, TestResource](mockClient)
		assert.NoError(t, err)

		// Agent is not connected, so no status update should occur
		ctx := context.Background()
		err = stdAgent.Shutdown(ctx)
		assert.NoError(t, err)
	})

	t.Run("fails when UpdateAgentStatus returns error", func(t *testing.T) {
		mockClient := NewMockFulcrumClient[TestPayload](t)
		stdAgent, err := New[TestPayload, TestResource](mockClient)
		assert.NoError(t, err)

		stdAgent.setStatus(agent.AgentStatusConnected)
		mockClient.EXPECT().UpdateAgentStatus(agent.AgentStatusDisconnected).Return(assert.AnError).Once()

		ctx := context.Background()
		err = stdAgent.Shutdown(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update agent status on shutdown")
	})
}

func TestAgent_PollAndProcessJobs(t *testing.T) {
	t.Run("processes job successfully", func(t *testing.T) {
		mockClient := NewMockFulcrumClient[TestPayload](t)
		stdAgent, err := New[TestPayload, TestResource](mockClient)
		assert.NoError(t, err)

		// Register a job handler
		jobResponse := &agent.JobResponse[TestResource]{
			Resources:  &TestResource{ID: "test-resource", URL: "http://test.com"},
			ExternalID: stringPtr("ext-123"),
		}

		handler := func(ctx context.Context, job *agent.Job[TestPayload]) (*agent.JobResponse[TestResource], error) {
			return jobResponse, nil
		}
		stdAgent.OnJob(agent.JobActionServiceCreate, handler)

		// Mock job data
		testJob := &agent.Job[TestPayload]{
			ID:     "job-123",
			Action: agent.JobActionServiceCreate,
			Status: agent.JobStatusPending,
			Service: agent.Service[TestPayload]{
				ID:   "service-123",
				Name: "test-service",
				TargetProperties: &TestPayload{
					Name: "test",
					Port: 8080,
				},
			},
		}

		mockClient.EXPECT().GetPendingJobs().Return([]*agent.Job[TestPayload]{testJob}, nil).Once()
		mockClient.EXPECT().ClaimJob("job-123").Return(nil).Once()
		mockClient.EXPECT().CompleteJob("job-123", jobResponse).Return(nil).Once()

		ctx := context.Background()
		err = stdAgent.pollAndProcessJobs(ctx)
		assert.NoError(t, err)

		processed, succeeded, failed := stdAgent.GetJobStats()
		assert.Equal(t, 1, processed)
		assert.Equal(t, 1, succeeded)
		assert.Equal(t, 0, failed)
	})

	t.Run("handles job failure", func(t *testing.T) {
		mockClient := NewMockFulcrumClient[TestPayload](t)
		stdAgent, err := New[TestPayload, TestResource](mockClient)
		assert.NoError(t, err)

		// Register a job handler that fails
		handler := func(ctx context.Context, job *agent.Job[TestPayload]) (*agent.JobResponse[TestResource], error) {
			return nil, assert.AnError
		}
		stdAgent.OnJob(agent.JobActionServiceCreate, handler)

		testJob := &agent.Job[TestPayload]{
			ID:     "job-123",
			Action: agent.JobActionServiceCreate,
			Status: agent.JobStatusPending,
		}

		mockClient.EXPECT().GetPendingJobs().Return([]*agent.Job[TestPayload]{testJob}, nil).Once()
		mockClient.EXPECT().ClaimJob("job-123").Return(nil).Once()
		mockClient.EXPECT().FailJob("job-123", mock.AnythingOfType("string")).Return(nil).Once()

		ctx := context.Background()
		err = stdAgent.pollAndProcessJobs(ctx)
		assert.NoError(t, err)

		processed, succeeded, failed := stdAgent.GetJobStats()
		assert.Equal(t, 1, processed)
		assert.Equal(t, 0, succeeded)
		assert.Equal(t, 1, failed)
	})

	t.Run("no pending jobs", func(t *testing.T) {
		mockClient := NewMockFulcrumClient[TestPayload](t)
		stdAgent, err := New[TestPayload, TestResource](mockClient)
		assert.NoError(t, err)

		mockClient.EXPECT().GetPendingJobs().Return([]*agent.Job[TestPayload]{}, nil).Once()

		ctx := context.Background()
		err = stdAgent.pollAndProcessJobs(ctx)
		assert.NoError(t, err)

		processed, _, _ := stdAgent.GetJobStats()
		assert.Equal(t, 0, processed)
	})

	t.Run("no handler for job action", func(t *testing.T) {
		mockClient := NewMockFulcrumClient[TestPayload](t)
		stdAgent, err := New[TestPayload, TestResource](mockClient)
		assert.NoError(t, err)

		testJob := &agent.Job[TestPayload]{
			ID:     "job-123",
			Action: agent.JobActionServiceCreate, // No handler registered for this action
			Status: agent.JobStatusPending,
		}

		mockClient.EXPECT().GetPendingJobs().Return([]*agent.Job[TestPayload]{testJob}, nil).Once()

		ctx := context.Background()
		err = stdAgent.pollAndProcessJobs(ctx)
		assert.NoError(t, err)

		processed, _, _ := stdAgent.GetJobStats()
		assert.Equal(t, 1, processed) // Job is counted as processed even if no handler
	})

	t.Run("fails to get pending jobs", func(t *testing.T) {
		mockClient := NewMockFulcrumClient[TestPayload](t)
		stdAgent, err := New[TestPayload, TestResource](mockClient)
		assert.NoError(t, err)

		mockClient.EXPECT().GetPendingJobs().Return(nil, assert.AnError).Once()

		ctx := context.Background()
		err = stdAgent.pollAndProcessJobs(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get pending jobs")
	})

	t.Run("fails to claim job", func(t *testing.T) {
		mockClient := NewMockFulcrumClient[TestPayload](t)
		stdAgent, err := New[TestPayload, TestResource](mockClient)
		assert.NoError(t, err)

		handler := func(ctx context.Context, job *agent.Job[TestPayload]) (*agent.JobResponse[TestResource], error) {
			return &agent.JobResponse[TestResource]{}, nil
		}
		stdAgent.OnJob(agent.JobActionServiceCreate, handler)

		testJob := &agent.Job[TestPayload]{
			ID:     "job-123",
			Action: agent.JobActionServiceCreate,
			Status: agent.JobStatusPending,
		}

		mockClient.EXPECT().GetPendingJobs().Return([]*agent.Job[TestPayload]{testJob}, nil).Once()
		mockClient.EXPECT().ClaimJob("job-123").Return(assert.AnError).Once()

		ctx := context.Background()
		err = stdAgent.pollAndProcessJobs(ctx)
		assert.Error(t, err)
	})
}

func TestAgent_GetMethods(t *testing.T) {
	mockClient := NewMockFulcrumClient[TestPayload](t)
	stdAgent, err := New[TestPayload, TestResource](mockClient)
	assert.NoError(t, err)

	// Test GetAgentID
	stdAgent.agentID = "test-agent-123"
	assert.Equal(t, "test-agent-123", stdAgent.GetAgentID())

	// Test GetUptime
	stdAgent.startTime = time.Now().Add(-5 * time.Minute)
	uptime := stdAgent.GetUptime()
	assert.True(t, uptime >= 5*time.Minute)
	assert.True(t, uptime < 6*time.Minute)

	// Test GetJobStats
	stdAgent.jobStats.processed = 10
	stdAgent.jobStats.succeeded = 8
	stdAgent.jobStats.failed = 2

	processed, succeeded, failed := stdAgent.GetJobStats()
	assert.Equal(t, 10, processed)
	assert.Equal(t, 8, succeeded)
	assert.Equal(t, 2, failed)
}

func TestAgent_IntegrationWithHeartbeat(t *testing.T) {
	mockClient := NewMockFulcrumClient[TestPayload](t)

	// Create agent with very short intervals for testing
	stdAgent, err := New(
		mockClient,
		WithHeartbeatInterval[TestPayload, TestResource](50*time.Millisecond),
	)
	assert.NoError(t, err)

	// Set up heartbeat handler
	heartbeatCalled := false
	stdAgent.OnHeartbeat(func(ctx context.Context) error {
		heartbeatCalled = true
		return nil
	})

	// Mock startup sequence
	mockClient.EXPECT().GetAgentInfo().Return(&agent.AgentInfo{
		ID: "test-agent-123",
	}, nil).Once()

	mockClient.EXPECT().UpdateAgentStatus(agent.AgentStatusConnected).Return(nil).Once()

	// Mock heartbeat calls (allow multiple calls)
	mockClient.EXPECT().UpdateAgentStatus(agent.AgentStatusConnected).Return(nil).Maybe()

	// Mock shutdown
	mockClient.EXPECT().UpdateAgentStatus(agent.AgentStatusDisconnected).Return(nil).Once()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Start the agent
	err = stdAgent.Run(ctx)
	assert.NoError(t, err)

	// Wait a bit for heartbeat to be called
	time.Sleep(100 * time.Millisecond)

	// Shutdown the agent
	shutdownCtx := context.Background()
	err = stdAgent.Shutdown(shutdownCtx)
	assert.NoError(t, err)

	assert.True(t, heartbeatCalled, "Heartbeat handler should have been called")
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
