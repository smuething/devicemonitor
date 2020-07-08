package monitor

import (
	"os"
	"sync"

	"github.com/rjeczalik/notify"
)

type JobChannel <-chan *Job
type JobValidationFunc func(*os.File) bool

type Monitor struct {
	active   int64 // This has to be first to guarantee alignment for the atomic updates
	m        sync.Mutex
	path     string
	state    state
	spooling chan int
	fsEvents chan notify.EventInfo
	queues   map[string]*Queue
	jobs     chan *Job
	isValid  JobValidationFunc
}

func NewMonitor(path string, isValid JobValidationFunc) *Monitor {
	return &Monitor{
		path:     path,
		state:    valid,
		fsEvents: make(chan notify.EventInfo, 10),
		spooling: make(chan int, 1),
		queues:   make(map[string]*Queue),
		jobs:     make(chan *Job, 1),
		isValid:  isValid,
	}
}

func (m *Monitor) Path() string {
	return m.path
}

func (m *Monitor) Jobs() JobChannel {
	return m.jobs
}

func (m *Monitor) Spooling() <-chan int {
	return m.spooling
}
