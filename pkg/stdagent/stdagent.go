package stdagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fulcrumproject/agent-lib-go/pkg/agent"
)

// Default intervals
const (
	DefaultHealthInterval        = 60 * time.Second
	DefaultJobPollInterval       = 5 * time.Second
	DefaultMetricsReportInterval = 30 * time.Second
)

type Agent struct {
	client                FulcrumClient
	healthInterval        time.Duration
	healthbeatHandler     agent.HealthHandler
	connectHandler        agent.RawConnectHandler
	jobPollInterval       time.Duration
	jobHandlers           map[string]agent.RawJobHandler
	metricsReportInterval time.Duration
	metricsReporter       agent.RawMetricsReporter

	// Job statistics
	jobStats struct {
		processed int
		succeeded int
		failed    int
	}

	// Runtime state
	stopCh        chan struct{}
	jobPollStopCh chan struct{}
	wg            sync.WaitGroup
	startTime     time.Time
	status        agent.AgentStatus
	statusMu      sync.RWMutex
	agentID       string
}

func New(client FulcrumClient, options ...AgentOption) (*Agent, error) {
	agent := &Agent{
		client:                client,
		jobHandlers:           make(map[string]agent.RawJobHandler),
		healthInterval:        DefaultHealthInterval,
		jobPollInterval:       DefaultJobPollInterval,
		metricsReportInterval: DefaultMetricsReportInterval,
		stopCh:                make(chan struct{}),
		jobPollStopCh:         make(chan struct{}),
		status:                agent.AgentStatusNew,
	}

	// Apply options
	for _, option := range options {
		if err := option(agent); err != nil {
			return nil, err
		}
	}

	return agent, nil
}

func (a *Agent) OnJob(action string, handler agent.RawJobHandler) error {
	a.jobHandlers[action] = handler
	return nil
}

func (a *Agent) OnMetrics(handler agent.RawMetricsReporter) error {
	a.metricsReporter = handler
	return nil
}

func (a *Agent) OnHealth(handler agent.HealthHandler) error {
	a.healthbeatHandler = handler
	return nil
}

func (a *Agent) OnConnect(handler agent.RawConnectHandler) error {
	a.connectHandler = handler
	return nil
}

func (a *Agent) Run(ctx context.Context) error {
	a.startTime = time.Now()

	// Get agent information to verify the token is valid
	agentInfo, err := a.client.GetAgentInfo()
	if err != nil {
		return fmt.Errorf("failed to get agent information: %w", err)
	}

	// Extract agent ID from the response
	if agentInfo.ID == "" {
		return fmt.Errorf("invalid agent information received: missing ID")
	}
	a.agentID = agentInfo.ID

	slog.Info("Agent authenticated", "id", agentInfo.ID)

	// Resolve secrets in agent configuration transparently
	// Ephemeral secrets not found are set to nil
	if agentInfo.Config != nil {
		if err := resolveSecretsInJSON(ctx, a.client, agentInfo.Config, true); err != nil {
			return fmt.Errorf("failed to resolve secrets in agent configuration: %w", err)
		}
	}

	// If a connect handler is provided, use it to handle the agent info
	if a.connectHandler != nil {
		if agentInfo.Config != nil {
			stringConfig, err := json.Marshal(agentInfo.Config)
			if err != nil {
				return fmt.Errorf("failed to marshal config: %w", err)
			}
			slog.Info("Running agent with connect handler", "config", stringConfig)
		} else {
			slog.Info("Running agent with connect handler", "config", "nil")
		}
		if err := a.connectHandler(ctx, agentInfo); err != nil {
			return fmt.Errorf("failed to handle connect: %w", err)
		}
	}

	// Update agent status to Connected
	if err := a.client.UpdateAgentStatus(agent.AgentStatusConnected); err != nil {
		return fmt.Errorf("failed to update agent status: %w", err)
	}
	a.setStatus(agent.AgentStatusConnected)

	slog.Info("Agent status updated to Connected")

	// Start background goroutines
	if a.healthbeatHandler != nil {
		a.wg.Add(1)
		go a.health(ctx)
	}

	if a.metricsReporter != nil {
		a.wg.Add(1)
		go a.reportMetrics(ctx)
	}

	if len(a.jobHandlers) > 0 {
		a.wg.Add(1)
		go a.pollJobs(ctx)
	}

	return nil
}

func (a *Agent) Shutdown(ctx context.Context) error {
	// Close the stop channel to signal all goroutines to stop
	close(a.stopCh)

	// Wait for all goroutines to complete with a timeout
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All goroutines exited successfully
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for goroutines to exit")
	}

	// Update agent status to Disconnected
	if a.GetStatus() != agent.AgentStatusNew {
		if err := a.client.UpdateAgentStatus(agent.AgentStatusDisconnected); err != nil {
			return fmt.Errorf("failed to update agent status on shutdown: %w", err)
		}
		a.setStatus(agent.AgentStatusDisconnected)
		slog.Info("Agent status updated to Disconnected")
	}

	slog.Info("Agent shut down successfully")
	return nil
}

// health periodically calls the health handler and updates the agent status
func (a *Agent) health(ctx context.Context) {
	defer a.wg.Done()

	ticker := time.NewTicker(a.healthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			currentStatus := a.GetStatus()
			healthError := false

			// Call custom health handler if provided
			if a.healthbeatHandler != nil {
				if err := a.healthbeatHandler(ctx); err != nil {
					slog.Error("Health handler error", "error", err)
					healthError = true
				}
			}

			// Handle health failure
			if healthError {
				if currentStatus != agent.AgentStatusError {
					slog.Info("Setting agent status to Error due to health failure")
					// Update status to Error
					if err := a.client.UpdateAgentStatus(agent.AgentStatusError); err != nil {
						slog.Error("Failed to update agent status to Error", "error", err)
					}
					a.setStatus(agent.AgentStatusError)

					// Stop job polling
					a.stopJobPolling()
				}
			} else {
				// Health succeeded
				if currentStatus == agent.AgentStatusError {
					slog.Info("Health recovered, setting agent status back to Connected")
					// Restart job polling
					a.restartJobPolling(ctx)
				}

				// Update agent status to maintain connection
				if err := a.client.UpdateAgentStatus(agent.AgentStatusConnected); err != nil {
					slog.Error("Failed to update agent status", "error", err)
				} else {
					a.setStatus(agent.AgentStatusConnected)
					slog.Info("Health: Agent status updated to Connected")
				}
			}
		case <-a.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// reportMetrics periodically calls the metrics reporter for each service
func (a *Agent) reportMetrics(ctx context.Context) {
	defer a.wg.Done()

	ticker := time.NewTicker(a.metricsReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := a.collectAndReportAllMetrics(ctx); err != nil {
				slog.Error("Error during metrics collection and reporting", "error", err)
			}
		case <-a.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// collectAndReportAllMetrics iterates through all services with pagination and collects metrics for each
func (a *Agent) collectAndReportAllMetrics(ctx context.Context) error {
	totalMetricsReported := 0
	page := 1
	pageSize := 50 // Default page size for service listing

	for {
		// Get services with pagination
		pagination := &agent.PaginationOptions{
			Page:     page,
			PageSize: pageSize,
		}

		servicesResponse, err := a.client.ListServices(pagination)
		if err != nil {
			return fmt.Errorf("failed to list services for metrics collection: %w", err)
		}

		// If no services on this page, we're done
		if len(servicesResponse.Items) == 0 {
			break
		}

		// Process each service
		for _, service := range servicesResponse.Items {
			if service == nil {
				slog.Warn("Received nil service in response, skipping")
				continue
			}

			// Skip if service is new or deleted
			if service.Status == agent.ServiceStatusNew || service.Status == agent.ServiceStatusDeleted {
				slog.Debug("Skipping new or deleted service", "service_id", service.ID, "service_name", service.Name)
				continue
			}

			// Call the metrics reporter for this specific service with panic recovery
			metrics, err := a.executeMetricsReporterWithPanicRecovery(ctx, service)
			if err != nil {
				slog.Error("Error collecting metrics for service",
					"service_id", service.ID,
					"service_name", service.Name,
					"error", err)
				continue
			}

			// Report each metric to the client
			for _, metric := range metrics {
				if err := a.client.ReportMetric(&metric); err != nil {
					slog.Error("Error reporting metric",
						"service_id", service.ID,
						"metric_type", metric.TypeName,
						"error", err)
				}
			}

			totalMetricsReported += len(metrics)

			if len(metrics) > 0 {
				slog.Info("Collected metrics for service",
					"service_id", service.ID,
					"service_name", service.Name,
					"metric_count", len(metrics))
			}
		}

		// Check if we've processed all pages
		if page >= servicesResponse.TotalPages {
			break
		}

		// Move to next page
		page++
	}

	if totalMetricsReported > 0 {
		slog.Info("Completed metrics reporting cycle", "total_metrics", totalMetricsReported)
	}

	return nil
}

// pollJobs periodically polls for pending jobs and processes them
func (a *Agent) pollJobs(ctx context.Context) {
	defer a.wg.Done()

	ticker := time.NewTicker(a.jobPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Only poll jobs if status is Connected
			if a.GetStatus() == agent.AgentStatusConnected {
				if err := a.pollAndProcessJobs(ctx); err != nil {
					slog.Error("Error polling jobs", "error", err)
				}
			}
		case <-a.jobPollStopCh:
			slog.Info("Job polling stopped due to status change")
			return
		case <-a.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// pollAndProcessJobs polls for pending jobs and processes them
func (a *Agent) pollAndProcessJobs(ctx context.Context) error {
	// Get pending jobs
	jobs, err := a.client.GetPendingJobs()
	if err != nil {
		return fmt.Errorf("failed to get pending jobs: %w", err)
	}

	if len(jobs) == 0 {
		slog.Info("Pending jobs not found")
		return nil
	}

	// Process the first job
	job := jobs[0]
	a.jobStats.processed++

	// Check if we have a handler for this job action
	handler, ok := a.jobHandlers[job.Action]
	if !ok {
		unsupportedErr := fmt.Errorf("unsupported job action '%s'", job.Action)
		slog.Warn("No handler registered for job action", "action", job.Action, "job_id", job.ID)

		// Mark job as unsupported
		if err := a.client.FailJob(job.ID, unsupportedErr.Error()); err != nil {
			slog.Error("Failed to mark job as unsupported", "job_id", job.ID, "error", err)
			return err
		}

		a.jobStats.failed++
		slog.Info("Job failed because unsupported", "job_id", job.ID, "action", job.Action)
		return nil
	}

	// Claim the job
	if err := a.client.ClaimJob(job.ID); err != nil {
		slog.Error("Failed to claim job", "job_id", job.ID, "error", err)
		return err
	}

	slog.Info("Processing job", "job_id", job.ID, "action", job.Action)

	// Process the job using the registered handler with panic recovery
	resp, err := a.executeJobHandlerWithPanicRecovery(ctx, job, handler)

	if err != nil {
		// Mark job as failed
		slog.Error("Job failed", "job_id", job.ID, "error", err)
		a.jobStats.failed++
		if failErr := a.client.FailJob(job.ID, err.Error()); failErr != nil {
			slog.Error("Failed to mark job as failed", "job_id", job.ID, "error", failErr)
			return failErr
		}
		return nil
	}

	// Job succeeded
	a.jobStats.succeeded++
	if complErr := a.client.CompleteJob(job.ID, resp); complErr != nil {
		slog.Error("Failed to mark job as completed", "job_id", job.ID, "error", complErr)
		return complErr
	}
	slog.Info("Job completed successfully", "job_id", job.ID)
	return nil
}

// executeJobHandlerWithPanicRecovery executes a job handler with panic recovery
// If the handler panics, the panic is recovered and returned as an error
func (a *Agent) executeJobHandlerWithPanicRecovery(ctx context.Context, job *agent.RawJob, handler agent.RawJobHandler) (resp *agent.RawJobResponse, err error) {
	defer func() {
		if r := recover(); r != nil {
			// Convert panic to error
			err = fmt.Errorf("job handler panicked: %v", r)
			resp = nil
			slog.Error("Job handler panic recovered", "job_id", job.ID, "panic", r)
		}
	}()

	// Resolve secrets in job parameters transparently
	// Ephemeral secrets not found are set to nil
	if job.Params != nil {
		if err := resolveSecretsInJSON(ctx, a.client, job.Params, true); err != nil {
			return nil, fmt.Errorf("failed to resolve secrets in job parameters: %w", err)
		}
	}

	// Execute the handler
	return handler(ctx, job)
}

// executeMetricsReporterWithPanicRecovery executes a metrics reporter with panic recovery
// If the reporter panics, the panic is recovered and returned as an error
func (a *Agent) executeMetricsReporterWithPanicRecovery(ctx context.Context, service *agent.RawService) (metrics []agent.MetricEntry, err error) {
	defer func() {
		if r := recover(); r != nil {
			// Convert panic to error
			err = fmt.Errorf("metrics reporter panicked: %v", r)
			metrics = nil
			slog.Error("Metrics reporter panic recovered", "service_id", service.ID, "service_name", service.Name, "panic", r)
		}
	}()

	// Execute the metrics reporter
	return a.metricsReporter(ctx, service)
}

// GetAgentID returns the agent's ID
func (a *Agent) GetAgentID() string {
	return a.agentID
}

// GetUptime returns the agent's uptime
func (a *Agent) GetUptime() time.Duration {
	return time.Since(a.startTime)
}

// GetJobStats returns the job processing statistics
func (a *Agent) GetJobStats() (processed, succeeded, failed int) {
	return a.jobStats.processed, a.jobStats.succeeded, a.jobStats.failed
}

// GetStatus returns the current agent status
func (a *Agent) GetStatus() agent.AgentStatus {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.status
}

// setStatus sets the agent status (thread-safe)
func (a *Agent) setStatus(status agent.AgentStatus) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.status = status
}

// stopJobPolling stops the job polling goroutine
func (a *Agent) stopJobPolling() {
	select {
	case a.jobPollStopCh <- struct{}{}:
	default:
		// Channel might be full or closed, ignore
	}
}

// restartJobPolling restarts the job polling goroutine
func (a *Agent) restartJobPolling(ctx context.Context) {
	// Create a new stop channel for the new polling goroutine
	a.jobPollStopCh = make(chan struct{})

	// Start job polling if we have handlers
	if len(a.jobHandlers) > 0 {
		a.wg.Add(1)
		go a.pollJobs(ctx)
	}
}
