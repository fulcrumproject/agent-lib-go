# Fulcrum Agent Library for Go

A Go library for building Fulcrum agents that interact with the Fulcrum Core platform. This library provides the foundation for creating agents that can manage service lifecycles, report metrics, and process jobs from the Fulcrum Core job queue.

## Features

- **Job Processing**: Automatically poll and process jobs from the Fulcrum Core queue
- **Health Monitoring**: Periodic health checks with automatic status reporting
- **Metrics Collection**: Collect and report metrics for managed services
- **Type-Safe API**: Generic type support for strongly-typed service properties, resources, and configurations
- **Graceful Shutdown**: Proper cleanup and status updates on agent termination
- **Panic Recovery**: Built-in panic recovery for job handlers and metrics reporters
- **Flexible Configuration**: Customizable intervals for health checks, job polling, and metrics reporting

## Installation

```bash
go get github.com/fulcrumproject/agent-lib-go
```

## Quick Start

Here's a minimal example to get started:

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/fulcrumproject/agent-lib-go/pkg/agent"
    "github.com/fulcrumproject/agent-lib-go/pkg/fulcrumcli"
    "github.com/fulcrumproject/agent-lib-go/pkg/stdagent"
)

func main() {
    // Create HTTP client for Fulcrum Core API
    client := fulcrumcli.NewHTTPClient(
        "https://api.fulcrum.example.com",
        "your-agent-token-here",
    )

    // Create agent
    ag, err := stdagent.New(client)
    if err != nil {
        log.Fatalf("Failed to create agent: %v", err)
    }

    // Register job handler for Create action
    ag.OnJob("create", agent.JobHandlerWrapper(
        func(ctx context.Context, job *agent.Job[MyJobParams, MyServiceProps, MyResources]) (*agent.JobResponse[MyResources], error) {
            // Handle job
            return &agent.JobResponse[MyResources]{
                AgentInstanceID: stringPtr("instance-id-123"),
                AgentData:       &MyResources{/* ... */},
            }, nil
        },
    ))

    // Start agent
    ctx := context.Background()
    if err := ag.Run(ctx); err != nil {
        log.Fatalf("Failed to start agent: %v", err)
    }

    // Wait for interrupt signal
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    <-sigCh

    // Shutdown gracefully
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    if err := ag.Shutdown(shutdownCtx); err != nil {
        log.Fatalf("Failed to shutdown agent: %v", err)
    }
}
```

## Core Concepts

### Agent

The `Agent` interface represents a Fulcrum agent that connects to Fulcrum Core and processes jobs. The standard implementation (`stdagent.Agent`) provides:

- Automatic job polling and processing
- Health check reporting
- Metrics collection and reporting
- Status management

### Jobs

Jobs represent actions that need to be performed on services. Job actions are identified by string values. Common job actions include:

- `"create"`: Create a new service
- `"start"`: Start an existing service
- `"stop"`: Stop a running service
- `"update"`: Update service configuration
- `"delete"`: Delete a service

### Services

Services are the resources managed by the agent. Each service has:

- **Properties**: Configuration parameters for the service
- **AgentData**: Agent-managed operational data (IDs, endpoints, etc.)
- **Status**: Current state (New, Started, Stopped, Deleted)

### Type Safety

The library uses Go generics to provide type-safe handling of:

- **Job Parameters (JP)**: Parameters specific to a job
- **Service Properties (SP)**: Configuration for a service
- **AgentData (R)**: Agent operational data returned after operations
- **Configuration (C)**: Agent configuration from Fulcrum Core

## Detailed Usage

### 1. Creating an Agent

```go
import (
    "github.com/fulcrumproject/agent-lib-go/pkg/fulcrumcli"
    "github.com/fulcrumproject/agent-lib-go/pkg/stdagent"
)

// Create HTTP client
client := fulcrumcli.NewHTTPClient(
    "https://api.fulcrum.example.com",
    os.Getenv("FULCRUM_AGENT_TOKEN"),
)

// Create agent with custom intervals
agent, err := stdagent.New(
    client,
    stdagent.WithHealthInterval(60 * time.Second),
    stdagent.WithJobPollInterval(5 * time.Second),
    stdagent.WithMetricsReportInterval(30 * time.Second),
)
```

### 2. Registering Job Handlers

Define your types:

```go
// Job parameters for creating a VM
type CreateVMParams struct {
    ImageID      string `json:"imageId"`
    InstanceType string `json:"instanceType"`
}

// Service properties
type VMProperties struct {
    Region string `json:"region"`
    Zone   string `json:"zone"`
}

// AgentData returned after creation
type VMResources struct {
    InstanceID string `json:"instanceId"`
    PublicIP   string `json:"publicIp"`
    PrivateIP  string `json:"privateIp"`
}
```

Register handlers:

```go
// Handler for creating VMs
agent.OnJob("create", agent.JobHandlerWrapper(
    func(ctx context.Context, job *agent.Job[CreateVMParams, VMProperties, VMResources]) (*agent.JobResponse[VMResources], error) {
        // Access typed parameters
        imageID := job.Params.ImageID
        region := job.Service.Properties.Region
        
        // Create VM using your cloud provider SDK
        vm, err := cloudProvider.CreateVM(ctx, imageID, region)
        if err != nil {
            return nil, err
        }
        
        // Return typed response
        return &agent.JobResponse[VMResources]{
            AgentInstanceID: &vm.ID,
            AgentData: &VMResources{
                InstanceID: vm.ID,
                PublicIP:   vm.PublicIP,
                PrivateIP:  vm.PrivateIP,
            },
        }, nil
    },
))

// Handler for starting VMs
agent.OnJob("start", agent.JobHandlerWrapper(
    func(ctx context.Context, job *agent.Job[interface{}, VMProperties, VMResources]) (*agent.JobResponse[VMResources], error) {
        instanceID := job.Service.AgentData.InstanceID
        
        if err := cloudProvider.StartVM(ctx, instanceID); err != nil {
            return nil, err
        }
        
        return &agent.JobResponse[VMResources]{
            AgentData: job.Service.AgentData,
        }, nil
    },
))

// Handler for stopping VMs
agent.OnJob("stop", agent.JobHandlerWrapper(
    func(ctx context.Context, job *agent.Job[interface{}, VMProperties, VMResources]) (*agent.JobResponse[VMResources], error) {
        instanceID := job.Service.AgentData.InstanceID
        
        if err := cloudProvider.StopVM(ctx, instanceID); err != nil {
            return nil, err
        }
        
        return &agent.JobResponse[VMResources]{
            AgentData: job.Service.AgentData,
        }, nil
    },
))

// Handler for deleting VMs
agent.OnJob("delete", agent.JobHandlerWrapper(
    func(ctx context.Context, job *agent.Job[interface{}, VMProperties, VMResources]) (*agent.JobResponse[VMResources], error) {
        instanceID := job.Service.AgentData.InstanceID
        
        if err := cloudProvider.DeleteVM(ctx, instanceID); err != nil {
            return nil, err
        }
        
        return &agent.JobResponse[VMResources]{
            AgentData: job.Service.AgentData,
        }, nil
    },
))
```

### 3. Implementing Health Checks

```go
agent.OnHealth(func(ctx context.Context) error {
    // Check if your cloud provider is accessible
    if err := cloudProvider.Ping(ctx); err != nil {
        return fmt.Errorf("cloud provider unreachable: %w", err)
    }
    
    // Check if required credentials are valid
    if err := cloudProvider.ValidateCredentials(ctx); err != nil {
        return fmt.Errorf("invalid credentials: %w", err)
    }
    
    return nil
})
```

### 4. Reporting Metrics

```go
agent.OnMetrics(agent.MetricsReporterWrapper(
    func(ctx context.Context, service *agent.Service[VMProperties, VMResources]) ([]agent.MetricEntry, error) {
        instanceID := service.AgentData.InstanceID
        
        // Get metrics from cloud provider
        cpuUsage, err := cloudProvider.GetCPUUsage(ctx, instanceID)
        if err != nil {
            return nil, err
        }
        
        memoryUsage, err := cloudProvider.GetMemoryUsage(ctx, instanceID)
        if err != nil {
            return nil, err
        }
        
        return []agent.MetricEntry{
            {
                AgentInstanceID: instanceID,
                ResourceID:      service.ID,
                TypeName:        "cpu_usage",
                Value:           cpuUsage,
            },
            {
                AgentInstanceID: instanceID,
                ResourceID:      service.ID,
                TypeName:        "memory_usage",
                Value:           memoryUsage,
            },
        }, nil
    },
))
```

### 5. Handling Agent Configuration

Use the `OnConnect` handler to receive and process agent configuration from Fulcrum Core:

```go
type AgentConfig struct {
    CloudProvider string `json:"cloudProvider"`
    Region        string `json:"region"`
    APIKey        string `json:"apiKey"`
}

agent.OnConnect(agent.ConnectHandlerWrapper(
    func(ctx context.Context, info *agent.AgentInfo[AgentConfig]) error {
        log.Printf("Agent connected: %s", info.Name)
        log.Printf("Agent ID: %s", info.ID)
        
        if info.Config != nil {
            // Use configuration from Fulcrum Core
            log.Printf("Cloud Provider: %s", info.Config.CloudProvider)
            log.Printf("Region: %s", info.Config.Region)
            
            // Initialize cloud provider with config
            cloudProvider.Init(info.Config.APIKey, info.Config.Region)
        }
        
        return nil
    },
))
```

### 6. Running the Agent

```go
ctx := context.Background()

// Start the agent
if err := agent.Run(ctx); err != nil {
    log.Fatalf("Failed to start agent: %v", err)
}

// Wait for interrupt signal
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
<-sigCh

log.Println("Shutting down agent...")

// Shutdown with timeout
shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

if err := agent.Shutdown(shutdownCtx); err != nil {
    log.Fatalf("Failed to shutdown agent: %v", err)
}

log.Println("Agent shutdown complete")
```

## Configuration Options

The `stdagent.New()` function accepts the following options:

- `WithHealthInterval(duration)`: Set the interval for health checks (default: 60s)
- `WithJobPollInterval(duration)`: Set the interval for polling jobs (default: 5s)
- `WithMetricsReportInterval(duration)`: Set the interval for reporting metrics (default: 30s)

Example:

```go
agent, err := stdagent.New(
    client,
    stdagent.WithHealthInterval(2 * time.Minute),
    stdagent.WithJobPollInterval(10 * time.Second),
    stdagent.WithMetricsReportInterval(1 * time.Minute),
)
```

## Agent States

The agent can be in one of the following states:

- `AgentStatusNew`: Initial state before connection
- `AgentStatusConnected`: Successfully connected and processing jobs
- `AgentStatusDisconnected`: Disconnected (after shutdown)
- `AgentStatusError`: In error state (health check failed)
- `AgentStatusDisabled`: Disabled by Fulcrum Core

## Job States

Jobs progress through the following states:

- `JobStatusPending`: Waiting to be processed
- `JobStatusProcessing`: Currently being processed
- `JobStatusCompleted`: Successfully completed
- `JobStatusFailed`: Failed with an error

## Service States

Services can be in the following states:

- `ServiceStatusNew`: Newly created, not yet started
- `ServiceStatusStarted`: Running
- `ServiceStatusStopped`: Stopped
- `ServiceStatusDeleted`: Deleted

## Error Handling

When a job handler encounters an error, simply return a Go error. The agent will automatically mark the job as failed and report the error message to Fulcrum Core:

```go
return nil, fmt.Errorf("failed to create VM: %w", err)
```

If your agent doesn't support a particular job action, you can either:
- Not register a handler for that action (the agent will mark such jobs as failed automatically)
- Register a handler that returns an error indicating the operation is unsupported:

```go
agent.OnJob("update", agent.JobHandlerWrapper(
    func(ctx context.Context, job *agent.Job[interface{}, VMProperties, VMResources]) (*agent.JobResponse[VMResources], error) {
        return nil, fmt.Errorf("VM updates are not supported by this agent")
    },
))
```

## Advanced Features

### Pagination

When listing services in the metrics reporter, the agent automatically handles pagination:

```go
// The agent will automatically iterate through all pages
// You only need to implement metrics collection for individual services
```

### Panic Recovery

The agent includes built-in panic recovery for:

- Job handlers
- Metrics reporters

If a handler panics, the panic is recovered and converted to an error, preventing the agent from crashing.

### Automatic Status Management

The agent automatically manages its status:

- Updates to `Connected` after successful initialization
- Updates to `Error` if health checks fail
- Updates to `Disconnected` on shutdown
- Stops job polling when in error state
- Resumes job polling when health is recovered

## Example Implementation

For a complete working example, check out the [agent-test repository](https://github.com/fulcrumproject/agent-test).

## API Reference

### Agent Interface

```go
type Agent interface {
    Run(ctx context.Context) error
    Shutdown(ctx context.Context) error
    OnConnect(handler RawConnectHandler) error
    OnHealth(handler HealthHandler) error
    OnJob(action string, handler RawJobHandler) error
    OnMetrics(handler RawMetricsReporter) error
}
```

### FulcrumClient Interface

```go
type FulcrumClient interface {
    UpdateAgentStatus(status AgentStatus) error
    GetAgentInfo() (*AgentInfo[json.RawMessage], error)
    GetPendingJobs() ([]*RawJob, error)
    ClaimJob(jobID string) error
    CompleteJob(jobID string, resources any) error
    FailJob(jobID string, errorMessage string) error
    ReportMetric(metrics *MetricEntry) error
    ListServices(pagination *PaginationOptions) (*PageResponse[*RawService], error)
}
```

### Type Wrappers

The library provides wrapper functions to convert typed handlers to raw handlers:

- `ConnectHandlerWrapper[C any](handler ConnectHandler[C]) RawConnectHandler`
- `JobHandlerWrapper[JP, SP, R any](handler JobHandler[JP, SP, R]) RawJobHandler`
- `MetricsReporterWrapper[P, R any](reporter MetricsReporter[P, R]) RawMetricsReporter`

These wrappers handle JSON marshaling/unmarshaling automatically.

## Requirements

- Go 1.24.5 or higher
- Access to a Fulcrum Core instance
- Valid agent authentication token

## Dependencies

- `resty.dev/v3` - HTTP client for API communication
- `github.com/stretchr/testify` - Testing utilities (dev dependency)

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Support

For issues, questions, or contributions, please visit the [GitHub repository](https://github.com/fulcrumproject/agent-lib-go).
