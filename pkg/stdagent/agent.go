package stdagent

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/fulcrumproject/agent-lib-go/pkg/agent"
)

// Default intervals
const (
	DefaultHeartbeatInterval     = 60 * time.Second
	DefaultJobPollInterval       = 5 * time.Second
	DefaultMetricsReportInterval = 30 * time.Second
)

type Agent[P any, R any] struct {
	client                FulcrumClient[P]
	heartbeatInterval     time.Duration
	heartbeatHandler      agent.HeartbeatHandler
	jobPollInterval       time.Duration
	jobHandlers           map[agent.JobAction]agent.JobHandler[P, R]
	metricsReportInterval time.Duration
	metricsReporter       agent.MetricsReporter[P]

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

func New[P any, R any](client FulcrumClient[P], options ...AgentOption[P, R]) (*Agent[P, R], error) {
	agent := &Agent[P, R]{
		client:                client,
		jobHandlers:           make(map[agent.JobAction]agent.JobHandler[P, R]),
		heartbeatInterval:     DefaultHeartbeatInterval,
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

func (a *Agent[P, R]) OnJob(action agent.JobAction, handler agent.JobHandler[P, R]) error {
	a.jobHandlers[action] = handler
	return nil
}

func (a *Agent[P, R]) OnMetricsReport(handler agent.MetricsReporter[P]) error {
	a.metricsReporter = handler
	return nil
}

func (a *Agent[P, R]) OnHeartbeat(handler agent.HeartbeatHandler) error {
	a.heartbeatHandler = handler
	return nil
}

func (a *Agent[P, R]) Run(ctx context.Context) error {
	a.startTime = time.Now()

	// Get agent information to verify the token is valid
	agentInfo, err := a.client.GetAgentInfo()
	if err != nil {
		return fmt.Errorf("failed to get agent information: %w", err)
	}

	// Extract agent ID from the response
	id, ok := agentInfo["id"].(string)
	if !ok {
		return fmt.Errorf("invalid agent information received")
	}
	a.agentID = id

	log.Printf("Agent authenticated with ID: %s", id)

	// Update agent status to Connected
	if err := a.client.UpdateAgentStatus(agent.AgentStatusConnected); err != nil {
		return fmt.Errorf("failed to update agent status: %w", err)
	}
	a.setStatus(agent.AgentStatusConnected)

	log.Printf("Agent status updated to Connected")

	// Start background goroutines
	if a.heartbeatHandler != nil {
		a.wg.Add(1)
		go a.heartbeat(ctx)
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

func (a *Agent[P, R]) Shutdown(ctx context.Context) error {
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
		log.Println("Agent status updated to Disconnected")
	}

	log.Println("Agent shut down successfully")
	return nil
}

// heartbeat periodically calls the heartbeat handler and updates the agent status
func (a *Agent[P, R]) heartbeat(ctx context.Context) {
	defer a.wg.Done()

	ticker := time.NewTicker(a.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			currentStatus := a.GetStatus()
			heartbeatError := false

			// Call custom heartbeat handler if provided
			if a.heartbeatHandler != nil {
				if err := a.heartbeatHandler(ctx); err != nil {
					log.Printf("Heartbeat handler error: %v", err)
					heartbeatError = true
				}
			}

			// Handle heartbeat failure
			if heartbeatError {
				if currentStatus != agent.AgentStatusError {
					log.Printf("Setting agent status to Error due to heartbeat failure")
					// Update status to Error
					if err := a.client.UpdateAgentStatus(agent.AgentStatusError); err != nil {
						log.Printf("Failed to update agent status to Error: %v", err)
					}
					a.setStatus(agent.AgentStatusError)

					// Stop job polling
					a.stopJobPolling()
				}
			} else {
				// Heartbeat succeeded
				if currentStatus == agent.AgentStatusError {
					log.Printf("Heartbeat recovered, setting agent status back to Connected")
					// Restart job polling
					a.restartJobPolling(ctx)
				}

				// Update agent status to maintain connection
				if err := a.client.UpdateAgentStatus(agent.AgentStatusConnected); err != nil {
					log.Printf("Failed to update agent status: %v", err)
				} else {
					a.setStatus(agent.AgentStatusConnected)
					log.Printf("Heartbeat: Agent status updated to Connected")
				}
			}
		case <-a.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// reportMetrics periodically calls the metrics reporter
func (a *Agent[P, R]) reportMetrics(ctx context.Context) {
	defer a.wg.Done()

	ticker := time.NewTicker(a.metricsReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metrics, err := a.metricsReporter(ctx)
			if err != nil {
				log.Printf("Error collecting metrics: %v", err)
				continue
			}

			// Report each metric to the client
			for _, metric := range metrics {
				if err := a.client.ReportMetric(&metric); err != nil {
					log.Printf("Error reporting metric: %v", err)
				}
			}

			if len(metrics) > 0 {
				log.Printf("Reported %d metrics", len(metrics))
			}
		case <-a.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// pollJobs periodically polls for pending jobs and processes them
func (a *Agent[P, R]) pollJobs(ctx context.Context) {
	defer a.wg.Done()

	ticker := time.NewTicker(a.jobPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Only poll jobs if status is Connected
			if a.GetStatus() == agent.AgentStatusConnected {
				if err := a.pollAndProcessJobs(ctx); err != nil {
					log.Printf("Error polling jobs: %v", err)
				}
			}
		case <-a.jobPollStopCh:
			log.Printf("Job polling stopped due to status change")
			return
		case <-a.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// pollAndProcessJobs polls for pending jobs and processes them
func (a *Agent[P, R]) pollAndProcessJobs(ctx context.Context) error {
	// Get pending jobs
	jobs, err := a.client.GetPendingJobs()
	if err != nil {
		return fmt.Errorf("failed to get pending jobs: %w", err)
	}

	if len(jobs) == 0 {
		log.Printf("Pending jobs not found")
		return nil
	}

	// Process the first job
	job := jobs[0]
	a.jobStats.processed++

	// Check if we have a handler for this job action
	handler, ok := a.jobHandlers[job.Action]
	if !ok {
		log.Printf("No handler registered for job action: %s", job.Action)
		return nil
	}

	// Claim the job
	if err := a.client.ClaimJob(job.ID); err != nil {
		log.Printf("Failed to claim job %s: %v", job.ID, err)
		return err
	}

	log.Printf("Processing job %s of type %s", job.ID, job.Action)

	// Process the job using the registered handler
	resp, err := handler(ctx, job)
	if err != nil {
		// Mark job as failed
		log.Printf("Job %s failed: %v", job.ID, err)
		a.jobStats.failed++
		if failErr := a.client.FailJob(job.ID, err.Error()); failErr != nil {
			log.Printf("Failed to mark job %s as failed: %v", job.ID, failErr)
			return failErr
		}
	} else {
		// Job succeeded
		a.jobStats.succeeded++
		if complErr := a.client.CompleteJob(job.ID, resp); complErr != nil {
			log.Printf("Failed to mark job %s as completed: %v", job.ID, complErr)
			return complErr
		}
		log.Printf("Job %s completed successfully", job.ID)
	}

	return nil
}

// GetAgentID returns the agent's ID
func (a *Agent[P, R]) GetAgentID() string {
	return a.agentID
}

// GetUptime returns the agent's uptime
func (a *Agent[P, R]) GetUptime() time.Duration {
	return time.Since(a.startTime)
}

// GetJobStats returns the job processing statistics
func (a *Agent[P, R]) GetJobStats() (processed, succeeded, failed int) {
	return a.jobStats.processed, a.jobStats.succeeded, a.jobStats.failed
}

// GetStatus returns the current agent status
func (a *Agent[P, R]) GetStatus() agent.AgentStatus {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.status
}

// setStatus sets the agent status (thread-safe)
func (a *Agent[P, R]) setStatus(status agent.AgentStatus) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.status = status
}

// stopJobPolling stops the job polling goroutine
func (a *Agent[P, R]) stopJobPolling() {
	select {
	case a.jobPollStopCh <- struct{}{}:
	default:
		// Channel might be full or closed, ignore
	}
}

// restartJobPolling restarts the job polling goroutine
func (a *Agent[P, R]) restartJobPolling(ctx context.Context) {
	// Create a new stop channel for the new polling goroutine
	a.jobPollStopCh = make(chan struct{})

	// Start job polling if we have handlers
	if len(a.jobHandlers) > 0 {
		a.wg.Add(1)
		go a.pollJobs(ctx)
	}
}
