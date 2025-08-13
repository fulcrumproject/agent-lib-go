package stdagent

import (
	"fmt"
	"time"
)

type AgentOption func(*Agent) error

func WithHealthInterval(interval time.Duration) AgentOption {
	return func(a *Agent) error {
		if interval <= 0 {
			return fmt.Errorf("health interval must be greater than 0")
		}
		a.healthInterval = interval
		return nil
	}
}

func WithJobPollInterval(interval time.Duration) AgentOption {
	return func(a *Agent) error {
		if interval <= 0 {
			return fmt.Errorf("job poll interval must be greater than 0")
		}
		a.jobPollInterval = interval
		return nil
	}
}

func WithMetricsReportInterval(interval time.Duration) AgentOption {
	return func(a *Agent) error {
		if interval <= 0 {
			return fmt.Errorf("metrics report interval must be greater than 0")
		}
		a.metricsReportInterval = interval
		return nil
	}
}
