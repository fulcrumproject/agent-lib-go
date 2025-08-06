package stdagent

import (
	"fmt"
	"time"
)

type AgentOption[P, R, C any] func(*Agent[P, R, C]) error

func WithHeartbeatInterval[P, R, C any](interval time.Duration) AgentOption[P, R, C] {
	return func(a *Agent[P, R, C]) error {
		if interval <= 0 {
			return fmt.Errorf("heartbeat interval must be greater than 0")
		}
		a.heartbeatInterval = interval
		return nil
	}
}

func WithJobPollInterval[P, R, C any](interval time.Duration) AgentOption[P, R, C] {
	return func(a *Agent[P, R, C]) error {
		if interval <= 0 {
			return fmt.Errorf("job poll interval must be greater than 0")
		}
		a.jobPollInterval = interval
		return nil
	}
}

func WithMetricsReportInterval[P, R, C any](interval time.Duration) AgentOption[P, R, C] {
	return func(a *Agent[P, R, C]) error {
		if interval <= 0 {
			return fmt.Errorf("metrics report interval must be greater than 0")
		}
		a.metricsReportInterval = interval
		return nil
	}
}
