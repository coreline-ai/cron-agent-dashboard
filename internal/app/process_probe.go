package app

import (
	"context"

	"github.com/shirou/gopsutil/v4/process"
)

type ProcessProbeResult struct {
	Checked bool
	Alive   bool
	Exe     string
}

type ProcessProbe interface {
	ProbeProcess(ctx context.Context, pid int) ProcessProbeResult
}

type ProcessProbeFunc func(ctx context.Context, pid int) ProcessProbeResult

func (f ProcessProbeFunc) ProbeProcess(ctx context.Context, pid int) ProcessProbeResult {
	return f(ctx, pid)
}

type GopsutilProcessProbe struct{}

func (GopsutilProcessProbe) ProbeProcess(ctx context.Context, pid int) ProcessProbeResult {
	if pid <= 0 {
		return ProcessProbeResult{}
	}
	exists, err := process.PidExistsWithContext(ctx, int32(pid))
	if err != nil {
		return ProcessProbeResult{}
	}
	if !exists {
		return ProcessProbeResult{Checked: true, Alive: false}
	}
	p, err := process.NewProcessWithContext(ctx, int32(pid))
	if err != nil {
		return ProcessProbeResult{Checked: true, Alive: true}
	}
	exe, _ := p.ExeWithContext(ctx)
	return ProcessProbeResult{Checked: true, Alive: true, Exe: exe}
}
