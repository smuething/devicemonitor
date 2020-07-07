package monitor

import (
	"sync"
	"time"
)

type Job struct {
	m         sync.Mutex
	Time      time.Time
	Name      string
	queue     *Queue
	File      string
	Printer   string
	submitted bool
}
