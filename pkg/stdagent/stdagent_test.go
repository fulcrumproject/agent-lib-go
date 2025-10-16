package stdagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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

type TestConfig struct {
	TestProperty string `json:"testProperty"`
}

type TestProperties struct {
	DatabaseURL    string `json:"database_url"`
	MaxConnections int    `json:"max_connections"`
	Enabled        bool   `json:"enabled"`
}

// TestAgentInterface verifies that Agent implements the agent.Agent interface
func TestAgentInterface(t *testing.T) {
	mockClient := NewMockFulcrumClient(t)
	stdAgent, err := New(mockClient)
	assert.NoError(t, err)

	// Verify that the standard agent implements the agent.Agent interface
	var _ agent.Agent = stdAgent
}

func TestNew(t *testing.T) {
	mockClient := NewMockFulcrumClient(t)

	t.Run("creates agent with default options", func(t *testing.T) {
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)
		assert.NotNil(t, stdAgent)
		assert.Equal(t, DefaultHealthInterval, stdAgent.healthInterval)
		assert.Equal(t, DefaultJobPollInterval, stdAgent.jobPollInterval)
		assert.Equal(t, DefaultMetricsReportInterval, stdAgent.metricsReportInterval)
	})

	t.Run("creates agent with custom options", func(t *testing.T) {
		customHealth := 30 * time.Second
		customJobPoll := 10 * time.Second
		customMetrics := 60 * time.Second

		stdAgent, err := New(
			mockClient,
			WithHealthInterval(customHealth),
			WithJobPollInterval(customJobPoll),
			WithMetricsReportInterval(customMetrics),
		)

		assert.NoError(t, err)
		assert.NotNil(t, stdAgent)
		assert.Equal(t, customHealth, stdAgent.healthInterval)
		assert.Equal(t, customJobPoll, stdAgent.jobPollInterval)
		assert.Equal(t, customMetrics, stdAgent.metricsReportInterval)
	})

	t.Run("fails with invalid options", func(t *testing.T) {
		_, err := New(
			mockClient,
			WithHealthInterval(-1*time.Second),
		)
		assert.Error(t, err)
	})
}

func TestAgent_OnJob(t *testing.T) {
	mockClient := NewMockFulcrumClient(t)
	stdAgent, err := New(mockClient)
	assert.NoError(t, err)

	handler := func(ctx context.Context, job *agent.Job[TestPayload, TestProperties, TestResource]) (*agent.JobResponse[TestResource], error) {
		return &agent.JobResponse[TestResource]{
			AgentData: &TestResource{ID: "test-id", URL: "http://test.com"},
		}, nil
	}

	err = stdAgent.OnJob("create", agent.JobHandlerWrapper(handler))
	assert.NoError(t, err)
	assert.Contains(t, stdAgent.jobHandlers, "create")
}

func TestAgent_OnMetricsReport(t *testing.T) {
	mockClient := NewMockFulcrumClient(t)
	stdAgent, err := New(mockClient)
	assert.NoError(t, err)

	t.Run("with raw metrics reporter", func(t *testing.T) {
		reporter := func(ctx context.Context, service *agent.RawService) ([]agent.MetricEntry, error) {
			return []agent.MetricEntry{
				{AgentInstanceID: "test", ResourceID: "res-1", Value: 100.0, TypeName: "cpu"},
			}, nil
		}

		err = stdAgent.OnMetrics(reporter)
		assert.NoError(t, err)
		assert.NotNil(t, stdAgent.metricsReporter)
	})

	t.Run("with typed metrics reporter using wrapper", func(t *testing.T) {
		// Define a typed metrics reporter
		reporter := func(ctx context.Context, service *agent.Service[TestProperties, TestResource]) ([]agent.MetricEntry, error) {
			return []agent.MetricEntry{
				{AgentInstanceID: "test", ResourceID: "res-1", Value: 100.0, TypeName: "cpu"},
			}, nil
		}

		// Use the wrapper to convert it to a raw reporter
		err = stdAgent.OnMetrics(agent.MetricsReporterWrapper(reporter))
		assert.NoError(t, err)
		assert.NotNil(t, stdAgent.metricsReporter)
	})
}

func TestAgent_OnHealth(t *testing.T) {
	mockClient := NewMockFulcrumClient(t)
	stdAgent, err := New(mockClient)
	assert.NoError(t, err)

	handler := func(ctx context.Context) error {
		return nil
	}

	err = stdAgent.OnHealth(handler)
	assert.NoError(t, err)
	assert.NotNil(t, stdAgent.healthbeatHandler)
}

func TestAgent_OnConnect(t *testing.T) {
	mockClient := NewMockFulcrumClient(t)
	stdAgent, err := New(mockClient)
	assert.NoError(t, err)

	t.Run("with raw connect handler", func(t *testing.T) {
		connectHandler := func(ctx context.Context, info *agent.AgentInfo[json.RawMessage]) error {
			return nil
		}

		err = stdAgent.OnConnect(connectHandler)
		assert.NoError(t, err)
		assert.NotNil(t, stdAgent.connectHandler)
	})

	t.Run("with typed connect handler using wrapper", func(t *testing.T) {
		// Define a typed connect handler
		typedConnectHandler := func(ctx context.Context, info *agent.AgentInfo[TestConfig]) error {
			// This handler can work with strongly typed configuration
			if info.Config != nil {
				assert.NotEmpty(t, info.Config.TestProperty)
			}
			return nil
		}

		// Use the wrapper to convert it to a raw handler
		err = stdAgent.OnConnect(agent.ConnectHandlerWrapper(typedConnectHandler))
		assert.NoError(t, err)
		assert.NotNil(t, stdAgent.connectHandler)
	})
}

func TestAgent_RunWithConnect(t *testing.T) {
	t.Run("runs successfully with connect handler", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		// Track if connect handler was called
		connectHandled := false
		var receivedInfo *agent.AgentInfo[TestConfig]

		// Use a typed connect handler
		typedConnectHandler := func(ctx context.Context, info *agent.AgentInfo[TestConfig]) error {
			connectHandled = true
			receivedInfo = info
			// Verify we can access the typed configuration
			if info.Config != nil {
				assert.Equal(t, "test-value", info.Config.TestProperty)
			}
			return nil
		}

		// Set the connect handler using the wrapper
		stdAgent.OnConnect(agent.ConnectHandlerWrapper(typedConnectHandler))

		// Mock the expected calls during startup
		configBytes, _ := json.Marshal(&TestConfig{TestProperty: "test-value"})
		configRaw := json.RawMessage(configBytes)
		expectedInfo := &agent.AgentInfo[json.RawMessage]{
			ID:          "test-agent-123",
			Name:        "test-agent",
			Status:      agent.AgentStatusConnected,
			AgentTypeID: "test-type",
			Config:      &configRaw,
		}
		mockClient.EXPECT().GetAgentInfo().Return(expectedInfo, nil).Once()

		mockClient.EXPECT().UpdateAgentStatus(agent.AgentStatusConnected).Return(nil).Once()

		ctx := context.Background()
		err = stdAgent.Run(ctx)
		assert.NoError(t, err)
		assert.Equal(t, "test-agent-123", stdAgent.GetAgentID())
		assert.Equal(t, agent.AgentStatusConnected, stdAgent.GetStatus())
		assert.True(t, connectHandled, "Connect handler should have been called")

		// Verify the typed handler received the correct typed agent info
		assert.NotNil(t, receivedInfo, "Connect handler should receive agent info")
		assert.Equal(t, "test-agent-123", receivedInfo.ID)
		assert.Equal(t, "test-agent", receivedInfo.Name)
		assert.Equal(t, agent.AgentStatusConnected, receivedInfo.Status)
		assert.Equal(t, "test-type", receivedInfo.AgentTypeID)
		assert.NotNil(t, receivedInfo.Config, "Config should not be nil")
		assert.Equal(t, "test-value", receivedInfo.Config.TestProperty)
	})

	t.Run("runs successfully without connect handler", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		// Mock the expected calls during startup
		configBytes, _ := json.Marshal(&TestConfig{TestProperty: "test-value"})
		configRaw := json.RawMessage(configBytes)
		mockClient.EXPECT().GetAgentInfo().Return(&agent.AgentInfo[json.RawMessage]{
			ID:     "test-agent-123",
			Name:   "test-agent",
			Status: agent.AgentStatusConnected,
			Config: &configRaw,
		}, nil).Once()

		mockClient.EXPECT().UpdateAgentStatus(agent.AgentStatusConnected).Return(nil).Once()

		ctx := context.Background()
		err = stdAgent.Run(ctx)
		assert.NoError(t, err)
		assert.Equal(t, "test-agent-123", stdAgent.GetAgentID())
		assert.Equal(t, agent.AgentStatusConnected, stdAgent.GetStatus())
	})

	t.Run("fails when connect handler returns error", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		// Use a typed connect handler that returns an error
		typedConnectHandler := func(ctx context.Context, info *agent.AgentInfo[TestConfig]) error {
			return assert.AnError
		}

		// Set the connect handler using the wrapper
		stdAgent.OnConnect(agent.ConnectHandlerWrapper(typedConnectHandler))

		// Mock the expected calls during startup
		configBytes, _ := json.Marshal(&TestConfig{TestProperty: "test-value"})
		configRaw := json.RawMessage(configBytes)
		mockClient.EXPECT().GetAgentInfo().Return(&agent.AgentInfo[json.RawMessage]{
			ID:     "test-agent-123",
			Name:   "test-agent",
			Status: agent.AgentStatusConnected,
			Config: &configRaw,
		}, nil).Once()

		ctx := context.Background()
		err = stdAgent.Run(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to handle connect")
	})

	t.Run("handles nil config gracefully", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		connectHandled := false
		var receivedInfo *agent.AgentInfo[TestConfig]

		// Use a typed connect handler that handles nil config
		typedConnectHandler := func(ctx context.Context, info *agent.AgentInfo[TestConfig]) error {
			connectHandled = true
			receivedInfo = info
			return nil
		}

		// Set the connect handler using the wrapper
		stdAgent.OnConnect(agent.ConnectHandlerWrapper(typedConnectHandler))

		// Mock the expected calls during startup with nil config
		expectedInfo := &agent.AgentInfo[json.RawMessage]{
			ID:     "test-agent-123",
			Name:   "test-agent",
			Status: agent.AgentStatusConnected,
			Config: nil,
		}
		mockClient.EXPECT().GetAgentInfo().Return(expectedInfo, nil).Once()

		mockClient.EXPECT().UpdateAgentStatus(agent.AgentStatusConnected).Return(nil).Once()

		ctx := context.Background()
		err = stdAgent.Run(ctx)
		assert.NoError(t, err)
		assert.True(t, connectHandled, "Connect handler should have been called even with nil config")

		// Verify the typed handler received the correct typed agent info with nil config
		assert.NotNil(t, receivedInfo, "Connect handler should receive agent info")
		assert.Equal(t, "test-agent-123", receivedInfo.ID)
		assert.Equal(t, "test-agent", receivedInfo.Name)
		assert.Equal(t, agent.AgentStatusConnected, receivedInfo.Status)
		assert.NotNil(t, receivedInfo.Config, "Config pointer should not be nil, but should contain zero value")
		assert.Equal(t, "", receivedInfo.Config.TestProperty, "Config should have zero values when original was nil")
	})

	t.Run("fails when config JSON is invalid for typed handler", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		// Use a typed connect handler
		typedConnectHandler := func(ctx context.Context, info *agent.AgentInfo[TestConfig]) error {
			return nil
		}

		// Set the connect handler using the wrapper
		stdAgent.OnConnect(agent.ConnectHandlerWrapper(typedConnectHandler))

		// Mock the expected calls during startup with invalid JSON config
		invalidConfigRaw := json.RawMessage(`{"testProperty": 123}`) // Wrong type: should be string, not number
		mockClient.EXPECT().GetAgentInfo().Return(&agent.AgentInfo[json.RawMessage]{
			ID:     "test-agent-123",
			Name:   "test-agent",
			Status: agent.AgentStatusConnected,
			Config: &invalidConfigRaw,
		}, nil).Once()

		ctx := context.Background()
		err = stdAgent.Run(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to handle connect")
	})
}

func TestAgent_Run(t *testing.T) {
	t.Run("successful startup", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		// Mock the expected calls during startup
		mockClient.EXPECT().GetAgentInfo().Return(&agent.AgentInfo[json.RawMessage]{
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
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		mockClient.EXPECT().GetAgentInfo().Return(nil, assert.AnError).Once()

		ctx := context.Background()
		err = stdAgent.Run(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get agent information")
	})

	t.Run("fails when agent info is invalid", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		mockClient.EXPECT().GetAgentInfo().Return(&agent.AgentInfo[json.RawMessage]{
			Name: "test-agent",
			// missing ID field to test error case
		}, nil).Once()

		ctx := context.Background()
		err = stdAgent.Run(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid agent information received")
	})

	t.Run("fails when UpdateAgentStatus returns error", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		mockClient.EXPECT().GetAgentInfo().Return(&agent.AgentInfo[json.RawMessage]{
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
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
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
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		// Agent is not connected, so no status update should occur
		ctx := context.Background()
		err = stdAgent.Shutdown(ctx)
		assert.NoError(t, err)
	})

	t.Run("fails when UpdateAgentStatus returns error", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
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
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		// Register a job handler
		jobResponse := &agent.JobResponse[TestResource]{
			AgentData:       &TestResource{ID: "test-resource", URL: "http://test.com"},
			AgentInstanceID: stringPtr("ext-123"),
		}

		handler := func(ctx context.Context, job *agent.Job[TestPayload, TestProperties, TestResource]) (*agent.JobResponse[TestResource], error) {
			return jobResponse, nil
		}
		stdAgent.OnJob("create", agent.JobHandlerWrapper(handler))

		// Mock job data - create RawJob with marshaled params
		paramsBytes, _ := json.Marshal(&TestPayload{
			Name: "test",
			Port: 8080,
		})
		paramsRaw := json.RawMessage(paramsBytes)

		testJob := &agent.RawJob{
			ID:       "job-123",
			Action:   "create",
			Status:   agent.JobStatusPending,
			Priority: 1,
			Params:   &paramsRaw,
		}

		mockClient.EXPECT().GetPendingJobs().Return([]*agent.RawJob{testJob}, nil).Once()
		mockClient.EXPECT().ClaimJob("job-123").Return(nil).Once()
		// The CompleteJob will be called with a JobResponse[json.RawMessage] now
		mockClient.EXPECT().CompleteJob("job-123", mock.AnythingOfType("*agent.RawJobResponse")).Return(nil).Once()

		ctx := context.Background()
		err = stdAgent.pollAndProcessJobs(ctx)
		assert.NoError(t, err)

		processed, succeeded, failed := stdAgent.GetJobStats()
		assert.Equal(t, 1, processed)
		assert.Equal(t, 1, succeeded)
		assert.Equal(t, 0, failed)
	})

	t.Run("handles job failure", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		// Register a job handler that fails
		handler := func(ctx context.Context, job *agent.Job[TestPayload, TestProperties, TestResource]) (*agent.JobResponse[TestResource], error) {
			return nil, assert.AnError
		}
		stdAgent.OnJob("create", agent.JobHandlerWrapper(handler))

		// Create RawJob with marshaled params
		paramsBytes, _ := json.Marshal(&TestPayload{
			Name: "test",
			Port: 8080,
		})
		paramsRaw := json.RawMessage(paramsBytes)

		testJob := &agent.RawJob{
			ID:       "job-123",
			Action:   "create",
			Status:   agent.JobStatusPending,
			Priority: 1,
			Params:   &paramsRaw,
		}

		mockClient.EXPECT().GetPendingJobs().Return([]*agent.RawJob{testJob}, nil).Once()
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
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		mockClient.EXPECT().GetPendingJobs().Return([]*agent.RawJob{}, nil).Once()

		ctx := context.Background()
		err = stdAgent.pollAndProcessJobs(ctx)
		assert.NoError(t, err)

		processed, _, _ := stdAgent.GetJobStats()
		assert.Equal(t, 0, processed)
	})

	t.Run("no handler for job action", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		// Create RawJob with marshaled params
		paramsBytes, _ := json.Marshal(&TestPayload{
			Name: "test",
			Port: 8080,
		})
		paramsRaw := json.RawMessage(paramsBytes)

		testJob := &agent.RawJob{
			ID:       "job-123",
			Action:   "create", // No handler registered for this action
			Status:   agent.JobStatusPending,
			Priority: 1,
			Params:   &paramsRaw,
		}

		mockClient.EXPECT().GetPendingJobs().Return([]*agent.RawJob{testJob}, nil).Once()
		mockClient.EXPECT().FailJob("job-123", "unsupported job action 'create'").Return(nil).Once()

		ctx := context.Background()
		err = stdAgent.pollAndProcessJobs(ctx)
		assert.NoError(t, err)

		processed, succeeded, failed := stdAgent.GetJobStats()
		assert.Equal(t, 1, processed) // Job is counted as processed even if no handler
		assert.Equal(t, 0, succeeded)
		assert.Equal(t, 1, failed)
	})

	t.Run("handler returns unsupported job error", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		// Register a job handler that returns UnsupportedJobError
		handler := func(ctx context.Context, job *agent.Job[TestPayload, TestProperties, TestResource]) (*agent.JobResponse[TestResource], error) {
			return nil, errors.New("unsupported property value")
		}
		stdAgent.OnJob("create", agent.JobHandlerWrapper(handler))
		err = stdAgent.OnJob("create", agent.JobHandlerWrapper(handler))
		assert.NoError(t, err)

		// Create RawJob with marshaled params
		paramsBytes, _ := json.Marshal(&TestPayload{
			Name: "test",
			Port: 8080,
		})
		paramsRaw := json.RawMessage(paramsBytes)

		testJob := &agent.RawJob{
			ID:       "job-123",
			Action:   "create",
			Status:   agent.JobStatusPending,
			Priority: 1,
			Params:   &paramsRaw,
		}

		mockClient.EXPECT().GetPendingJobs().Return([]*agent.RawJob{testJob}, nil).Once()
		mockClient.EXPECT().ClaimJob("job-123").Return(nil).Once()
		mockClient.EXPECT().FailJob("job-123", "unsupported property value").Return(nil).Once()

		ctx := context.Background()
		err = stdAgent.pollAndProcessJobs(ctx)
		assert.NoError(t, err)

		processed, succeeded, failed := stdAgent.GetJobStats()
		assert.Equal(t, 1, processed)
		assert.Equal(t, 0, succeeded)
		assert.Equal(t, 1, failed)
	})

	t.Run("fails to get pending jobs", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		mockClient.EXPECT().GetPendingJobs().Return(nil, assert.AnError).Once()

		ctx := context.Background()
		err = stdAgent.pollAndProcessJobs(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get pending jobs")
	})

	t.Run("fails to claim job", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		handler := func(ctx context.Context, job *agent.Job[TestPayload, TestProperties, TestResource]) (*agent.JobResponse[TestResource], error) {
			return &agent.JobResponse[TestResource]{}, nil
		}
		stdAgent.OnJob("create", agent.JobHandlerWrapper(handler))

		// Create RawJob with marshaled params
		paramsBytes, _ := json.Marshal(&TestPayload{
			Name: "test",
			Port: 8080,
		})
		paramsRaw := json.RawMessage(paramsBytes)

		testJob := &agent.RawJob{
			ID:       "job-123",
			Action:   "create",
			Status:   agent.JobStatusPending,
			Priority: 1,
			Params:   &paramsRaw,
		}

		mockClient.EXPECT().GetPendingJobs().Return([]*agent.RawJob{testJob}, nil).Once()
		mockClient.EXPECT().ClaimJob("job-123").Return(assert.AnError).Once()

		ctx := context.Background()
		err = stdAgent.pollAndProcessJobs(ctx)
		assert.Error(t, err)
	})

	t.Run("handles job handler panic", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		// Register a job handler that panics
		handler := func(ctx context.Context, job *agent.Job[TestPayload, TestProperties, TestResource]) (*agent.JobResponse[TestResource], error) {
			panic("test panic in job handler")
		}
		stdAgent.OnJob("create", agent.JobHandlerWrapper(handler))

		// Create RawJob with marshaled params
		paramsBytes, _ := json.Marshal(&TestPayload{
			Name: "test",
			Port: 8080,
		})
		paramsRaw := json.RawMessage(paramsBytes)

		testJob := &agent.RawJob{
			ID:       "job-123",
			Action:   "create",
			Status:   agent.JobStatusPending,
			Priority: 1,
			Params:   &paramsRaw,
		}

		mockClient.EXPECT().GetPendingJobs().Return([]*agent.RawJob{testJob}, nil).Once()
		mockClient.EXPECT().ClaimJob("job-123").Return(nil).Once()
		mockClient.EXPECT().FailJob("job-123", mock.MatchedBy(func(errorMsg string) bool {
			return strings.Contains(errorMsg, "job handler panicked: test panic in job handler")
		})).Return(nil).Once()

		ctx := context.Background()
		err = stdAgent.pollAndProcessJobs(ctx)
		assert.NoError(t, err) // The method should not return an error, panic should be handled

		processed, succeeded, failed := stdAgent.GetJobStats()
		assert.Equal(t, 1, processed)
		assert.Equal(t, 0, succeeded)
		assert.Equal(t, 1, failed) // Panic should be converted to a failed job
	})
}

func TestAgent_CollectAndReportAllMetrics(t *testing.T) {
	t.Run("handles metrics reporter panic", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		// Register a metrics reporter that panics
		reporter := func(ctx context.Context, service *agent.RawService) ([]agent.MetricEntry, error) {
			panic("test panic in metrics reporter")
		}
		stdAgent.OnMetrics(reporter)

		// Mock service data
		testService := &agent.RawService{
			ID:     "service-123",
			Name:   "test-service",
			Status: agent.ServiceStatusStarted,
		}

		// Mock the expected calls
		mockClient.EXPECT().ListServices(mock.AnythingOfType("*agent.PaginationOptions")).Return(&agent.PageResponse[*agent.RawService]{
			Items:      []*agent.RawService{testService},
			TotalItems: 1,
			TotalPages: 1,
			PageSize:   50,
			Page:       1,
		}, nil).Once()

		ctx := context.Background()
		err = stdAgent.collectAndReportAllMetrics(ctx)
		assert.NoError(t, err) // The method should not return an error, panic should be handled and logged

		// Note: We can't easily verify the log output in this test, but the panic recovery
		// prevents the agent from crashing and allows metrics collection to continue
	})

	t.Run("processes metrics successfully", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		// Register a working typed metrics reporter using wrapper
		reporter := func(ctx context.Context, service *agent.Service[TestProperties, TestResource]) ([]agent.MetricEntry, error) {
			return []agent.MetricEntry{
				{AgentInstanceID: "test-ext", ResourceID: "test-res", Value: 100.0, TypeName: "cpu"},
			}, nil
		}
		stdAgent.OnMetrics(agent.MetricsReporterWrapper(reporter))

		// Mock service data
		testService := &agent.RawService{
			ID:     "service-123",
			Name:   "test-service",
			Status: agent.ServiceStatusStarted,
		}

		// Mock the expected calls
		mockClient.EXPECT().ListServices(mock.AnythingOfType("*agent.PaginationOptions")).Return(&agent.PageResponse[*agent.RawService]{
			Items:      []*agent.RawService{testService},
			TotalItems: 1,
			TotalPages: 1,
			PageSize:   50,
			Page:       1,
		}, nil).Once()

		mockClient.EXPECT().ReportMetric(mock.AnythingOfType("*agent.MetricEntry")).Return(nil).Once()

		ctx := context.Background()
		err = stdAgent.collectAndReportAllMetrics(ctx)
		assert.NoError(t, err)
	})

	t.Run("handles metrics reporter error", func(t *testing.T) {
		mockClient := NewMockFulcrumClient(t)
		stdAgent, err := New(mockClient)
		assert.NoError(t, err)

		// Register a typed metrics reporter that returns an error using wrapper
		reporter := func(ctx context.Context, service *agent.Service[TestProperties, TestResource]) ([]agent.MetricEntry, error) {
			return nil, assert.AnError
		}
		stdAgent.OnMetrics(agent.MetricsReporterWrapper(reporter))

		// Mock service data
		testService := &agent.RawService{
			ID:     "service-123",
			Name:   "test-service",
			Status: agent.ServiceStatusStarted,
		}

		// Mock the expected calls
		mockClient.EXPECT().ListServices(mock.AnythingOfType("*agent.PaginationOptions")).Return(&agent.PageResponse[*agent.RawService]{
			Items:      []*agent.RawService{testService},
			TotalItems: 1,
			TotalPages: 1,
			PageSize:   50,
			Page:       1,
		}, nil).Once()

		ctx := context.Background()
		err = stdAgent.collectAndReportAllMetrics(ctx)
		assert.NoError(t, err) // Should continue despite individual service errors
	})
}

func TestAgent_GetMethods(t *testing.T) {
	mockClient := NewMockFulcrumClient(t)
	stdAgent, err := New(mockClient)
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

func TestAgent_IntegrationWithHealth(t *testing.T) {
	mockClient := NewMockFulcrumClient(t)

	// Create agent with very short intervals for testing
	stdAgent, err := New(
		mockClient,
		WithHealthInterval(50*time.Millisecond),
	)
	assert.NoError(t, err)

	// Set up health handler
	healthCalled := false
	stdAgent.OnHealth(func(ctx context.Context) error {
		healthCalled = true
		return nil
	})

	// Mock startup sequence
	mockClient.EXPECT().GetAgentInfo().Return(&agent.AgentInfo[json.RawMessage]{
		ID: "test-agent-123",
	}, nil).Once()

	mockClient.EXPECT().UpdateAgentStatus(agent.AgentStatusConnected).Return(nil).Once()

	// Mock health calls (allow multiple calls)
	mockClient.EXPECT().UpdateAgentStatus(agent.AgentStatusConnected).Return(nil).Maybe()

	// Mock shutdown
	mockClient.EXPECT().UpdateAgentStatus(agent.AgentStatusDisconnected).Return(nil).Once()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Start the agent
	err = stdAgent.Run(ctx)
	assert.NoError(t, err)

	// Wait a bit for health to be called
	time.Sleep(100 * time.Millisecond)

	// Shutdown the agent
	shutdownCtx := context.Background()
	err = stdAgent.Shutdown(shutdownCtx)
	assert.NoError(t, err)

	assert.True(t, healthCalled, "Health handler should have been called")
}

// TestConnectHandlerWrapper demonstrates comprehensive typed configuration handling
func TestConnectHandlerWrapper(t *testing.T) {
	// Define a more complex config struct for testing
	type ComplexConfig struct {
		DatabaseURL string            `json:"database_url"`
		APIKey      string            `json:"api_key"`
		Settings    map[string]string `json:"settings"`
		Enabled     bool              `json:"enabled"`
		MaxRetries  int               `json:"max_retries"`
	}

	t.Run("handles complex typed configuration correctly", func(t *testing.T) {
		// Create test configuration
		testConfig := ComplexConfig{
			DatabaseURL: "postgres://localhost/test",
			APIKey:      "secret-key-123",
			Settings:    map[string]string{"timeout": "30s", "pool_size": "10"},
			Enabled:     true,
			MaxRetries:  3,
		}

		// Marshal to raw JSON
		configBytes, err := json.Marshal(testConfig)
		assert.NoError(t, err)
		configRaw := json.RawMessage(configBytes)

		// Create raw agent info
		rawInfo := &agent.AgentInfo[json.RawMessage]{
			ID:          "test-agent-456",
			Name:        "complex-agent",
			Status:      agent.AgentStatusConnected,
			AgentTypeID: "complex-type",
			Config:      &configRaw,
		}

		// Track handler execution
		handlerCalled := false
		var receivedConfig *ComplexConfig

		// Create typed handler
		typedHandler := func(ctx context.Context, info *agent.AgentInfo[ComplexConfig]) error {
			handlerCalled = true
			receivedConfig = info.Config

			// Verify all fields are correctly unmarshaled
			assert.Equal(t, "test-agent-456", info.ID)
			assert.Equal(t, "complex-agent", info.Name)
			assert.Equal(t, agent.AgentStatusConnected, info.Status)
			assert.Equal(t, "complex-type", info.AgentTypeID)

			assert.NotNil(t, info.Config)
			assert.Equal(t, "postgres://localhost/test", info.Config.DatabaseURL)
			assert.Equal(t, "secret-key-123", info.Config.APIKey)
			assert.Equal(t, map[string]string{"timeout": "30s", "pool_size": "10"}, info.Config.Settings)
			assert.True(t, info.Config.Enabled)
			assert.Equal(t, 3, info.Config.MaxRetries)

			return nil
		}

		// Wrap the typed handler
		wrappedHandler := agent.ConnectHandlerWrapper(typedHandler)

		// Execute the wrapped handler
		ctx := context.Background()
		err = wrappedHandler(ctx, rawInfo)

		assert.NoError(t, err)
		assert.True(t, handlerCalled)
		assert.NotNil(t, receivedConfig)
		assert.Equal(t, testConfig, *receivedConfig)
	})

	t.Run("handles JSON unmarshaling errors gracefully", func(t *testing.T) {
		// Create invalid JSON
		invalidJSON := json.RawMessage(`{"database_url": "test", "invalid_field":}`) // Invalid JSON syntax

		rawInfo := &agent.AgentInfo[json.RawMessage]{
			ID:     "test-agent-error",
			Config: &invalidJSON,
		}

		typedHandler := func(ctx context.Context, info *agent.AgentInfo[TestConfig]) error {
			t.Fatal("Handler should not be called with invalid JSON")
			return nil
		}

		wrappedHandler := agent.ConnectHandlerWrapper(typedHandler)

		ctx := context.Background()
		err := wrappedHandler(ctx, rawInfo)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid character")
	})

	t.Run("handles nil config correctly", func(t *testing.T) {
		rawInfo := &agent.AgentInfo[json.RawMessage]{
			ID:     "test-agent-nil",
			Config: nil,
		}

		handlerCalled := false
		typedHandler := func(ctx context.Context, info *agent.AgentInfo[TestConfig]) error {
			handlerCalled = true
			assert.NotNil(t, info.Config) // Should be pointer to zero value
			assert.Equal(t, "", info.Config.TestProperty)
			return nil
		}

		wrappedHandler := agent.ConnectHandlerWrapper(typedHandler)

		ctx := context.Background()
		err := wrappedHandler(ctx, rawInfo)

		assert.NoError(t, err)
		assert.True(t, handlerCalled)
	})
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
