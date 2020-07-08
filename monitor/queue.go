package monitor

import (
	"fmt"
	"sync"
	"time"
)

type state int

const (
	invalid = iota
	valid
	running
	stopped
)

func (s state) String() string {
	switch s {
	case invalid:
		return "invalid"
	case valid:
		return "valid"
	case running:
		return "running"
	case stopped:
		return "stopped"
	default:
		return fmt.Sprintf("UNKNOWN STATE: %d", s)
	}
}

type Settings interface {
	Get(name string) string
	Set(name string, value string)
}

type Queue struct {
	m            sync.Mutex
	Device       string
	File         string
	Name         string
	Settings     Settings
	state        state
	job          *Job
	lastActivity time.Time
	timeout      time.Duration
	monitor      *Monitor
}
