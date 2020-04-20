package monitor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rjeczalik/notify"
	log "github.com/sirupsen/logrus"
	"github.com/smuething/devicemonitor/app"
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

type dummySettings struct{}

func (d *dummySettings) Get(string) string {
	return ""
}

func (d *dummySettings) Set(string, string) {}

type Job struct {
	m         sync.Mutex
	Name      string
	queue     *Queue
	File      string
	Printer   string
	submitted bool
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

func (q *Queue) IsSpooling() bool {
	return q.job != nil
}

func (q *Queue) devicePath() string {
	return `\??\` + q.File
}

func (q *Queue) start() error {
	q.m.Lock()
	defer q.m.Unlock()

	if q.state != valid {
		return fmt.Errorf("Cannot start queue %s in state %s", q.Name, q.state)
	}

	err := q.resetLocked(false)
	if err != nil {
		return err
	}
	defer func() {
		if q.state != running {
			os.Remove(q.File)
		}
	}()

	err = DefineDosDevice(q.Device, q.devicePath(), false, false, false)
	if err != nil {
		return err
	}

	q.state = running
	log.Infof("Started queue for %s", q.File)

	return nil
}

func (q *Queue) stop() {
	q.m.Lock()
	defer q.m.Unlock()

	if q.state == running {
		err := DefineDosDevice(q.Device, q.devicePath(), false, true, true)
		if err != nil {
			log.Error(err)
		}
		err = os.Remove(q.File)
		if err != nil && !os.IsNotExist(err) {
			log.Error(err)
		}
		q.state = stopped

		log.Infof("Stopped queue for %s", q.File)
	}
}

func (q *Queue) startJob() (*Job, error) {

	q.m.Lock()
	defer q.m.Unlock()

	if q.IsSpooling() {
		return nil, fmt.Errorf("Queue %s already has a job", q.Name)
	}

	q.job = &Job{
		Name:    "foo",
		queue:   q,
		File:    filepath.Join(filepath.Dir(q.File), "foo.txt"),
		Printer: q.Settings.Get("printer"),
	}
	q.monitor.updateSpooling(1)
	return q.job, nil
}

func (q *Queue) reset(submitJob bool) error {
	q.m.Lock()
	defer q.m.Unlock()
	return q.resetLocked(submitJob)
}

func (q *Queue) resetLocked(submitJob bool) error {

	// We have to actually remove the file instead of relying on os.Create()
	// to truncate it because there could be an active job whose file is
	// hardlinked to the spool file, and we need to break that connection
	err := os.Remove(q.File)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if submitJob && q.job != nil && q.job.submitted {
		select {
		case q.monitor.jobs <- q.job:
			log.Debugf("Submitted job %s to work queue", q.job.Name)
		default:
			log.Errorf("Dropped job %s", q.job.Name)
		}
		q.job = nil
	}

	// Create empty spool file
	f, err := os.Create(q.File)
	if err != nil {
		return err
	}
	f.Close()
	return nil
}

func (q *Queue) submitJob() {
	q.m.Lock()
	defer q.m.Unlock()

	if q.job != nil {
		app.Go(q.job.submit)
	}
}

func (j *Job) submit() {

	j.m.Lock()
	defer j.m.Unlock()

	if j.submitted {
		return
	}

	err := os.Link(j.queue.File, j.File)
	if err != nil && !os.IsExist(err) {
		panic(err)
	}

	if j.queue.monitor.isValid != nil {

		f, err := os.Open(j.File)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		fi, err := f.Stat()
		if err != nil {
			panic(err)
		}

		if j.queue.monitor.isValid(f) {
			fi2, err := f.Stat()
			if err != nil {
				panic(err)
			}

			fmt.Println(fi, fi2)

			if !fi2.ModTime().After(fi.ModTime()) {
				j.submitted = true
			}

		}

	} else {
		j.submitted = true
	}

	if j.submitted {
		j.queue.resetLocked(true)
		j.queue.monitor.updateSpooling(-1)
	}

}

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

func (m *Monitor) updateSpooling(delta int) {
	new := int(atomic.AddInt64(&(m.active), int64(delta)))
	select {
	case m.spooling <- new:
	default:
	}
}

func (m *Monitor) AddLPTPort(port int, name string) (queue *Queue, err error) {

	device := fmt.Sprintf("LPT%d", port)

	if port < 1 || port > 9 {
		err = fmt.Errorf("Invalid device %s, only support LPT1 - LPT9", device)
		return
	}

	file := fmt.Sprintf("lpt-%d.txt", port)
	if _, found := m.queues[file]; found {
		err = fmt.Errorf("Cannot add %s, already monitoring", device)
		return
	}

	m.queues[file] = &Queue{
		Device:   device,
		File:     filepath.Join(m.path, file),
		Name:     name,
		Settings: &dummySettings{},
		state:    valid,
		monitor:  m,
		timeout:  1000 * time.Millisecond,
	}

	queue = m.queues[file]
	return
}

// Start TODO
func (m *Monitor) Start(ctx context.Context) error {

	defer func() {
		if m.state != stopped {
			m.state = invalid
		}
	}()

	defer close(m.jobs)
	defer close(m.spooling)

	if m.state != valid {
		return fmt.Errorf("Cannot start monitor with state %s", m.state)
	}

	log.Infof("Starting monitor in directory %s", m.path)

	err := os.MkdirAll(m.path, 0755)
	if err != nil {
		return err
	}

	for file, queue := range m.queues {
		log.Infof("Starting queue %s", file)
		err = queue.start()
		if err != nil {
			return err
		}
		defer queue.stop()
	}

	if err = notify.Watch(m.path, m.fsEvents, notify.Write); err != nil {
		return err
	}
	defer notify.Stop(m.fsEvents)

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	m.state = running

	for {
		select {
		case <-ctx.Done():
			m.state = stopped
			return nil
		case ei := <-m.fsEvents:
			file := filepath.Base(ei.Path())
			if queue, found := m.queues[file]; found {
				log.Infof("Event %s for queue %s", ei, queue.Name)
				if ei.Event() == notify.Write {
					if !queue.IsSpooling() {
						if fi, err := os.Stat(queue.File); err != nil {
							log.Error(err)
							break
						} else {
							if fi.Size() == 0 {
								// spurious write event from creation of spool file
								break
							}
						}
						log.Infof("Started new job for queue %s", queue.Name)
						queue.startJob()
						queue.lastActivity = time.Now()
					} else {
						queue.lastActivity = time.Now()
					}
				}
			}
		case <-ticker.C:
			for file, queue := range m.queues {
				if queue.IsSpooling() && time.Since(queue.lastActivity) > queue.timeout {
					log.Infof("Job complete for queue %s", file)
					log.Infof("Now: %s lastActivity: %s", time.Now(), queue.lastActivity)
					queue.submitJob()
				}
			}
		}
	}
}
