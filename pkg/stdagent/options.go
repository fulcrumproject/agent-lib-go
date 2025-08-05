package stdagent

import (
	"fmt"
	"time"
)

type AgentOption[P any, R any] func(*Agent[P, R]) error

func WithHeartbeatInterval[P any, R any](interval time.Duration) AgentOption[P, R] {
	return func(a *Agent[P, R]) error {
		if interval <= 0 {
			return fmt.Errorf("heartbeat interval must be greater than 0")
		}
		a.heartbeatInterval = interval
		return nil
	}
}

func WithJobPollInterval[P any, R any](interval time.Duration) AgentOption[P, R] {
	return func(a *Agent[P, R]) error {
		if interval <= 0 {
			return fmt.Errorf("job poll interval must be greater than 0")
		}
		a.jobPollInterval = interval
		return nil
	}
}

func WithMetricsReportInterval[P any, R any](interval time.Duration) AgentOption[P, R] {
	return func(a *Agent[P, R]) error {
		if interval <= 0 {
			return fmt.Errorf("metrics report interval must be greater than 0")
		}
		a.metricsReportInterval = interval
		return nil
	}
}
