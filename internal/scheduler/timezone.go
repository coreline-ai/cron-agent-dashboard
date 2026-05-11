package scheduler

import (
	"os"
	"time"
)

const (
	DefaultTimezone = "Asia/Seoul"
	TimezoneEnv     = "CORN_AGENT_DASHBOARD_TIMEZONE"
)

func LoadLocation(name string) (*time.Location, error) {
	if name == "" {
		name = os.Getenv(TimezoneEnv)
	}
	if name == "" {
		name = DefaultTimezone
	}
	return time.LoadLocation(name)
}
