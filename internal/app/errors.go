package app

import "errors"

var (
	ErrStartupSelfCheckFailed = errors.New("startup self-check failed")
	ErrRuntimeNotConfigured   = errors.New("runtime not configured")
)
