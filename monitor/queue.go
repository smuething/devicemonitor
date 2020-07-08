package monitor

import (
	"sync"
	"time"
)

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
