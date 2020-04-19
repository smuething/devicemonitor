package monitor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rjeczalik/notify"
	log "github.com/sirupsen/logrus"
)

var jobCh chan Job = make(chan Job, 1)

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
	Name    string
	Queue   *Queue
	File    string
	Printer string
}

type Queue struct {
	Device       string
	File         string
	Name         string
	Settings     Settings
	Job          *Job
	LastActivity time.Time
}

func (q Queue) Active() bool {
	return q.Job != nil
}

func (q *Queue) StartJob() (*Job, error) {
	if q.Active() {
		return nil, fmt.Errorf("Queue %s already has a job", q.Name)
	}

	q.Job = &Job{
		Name:    "foo",
		Queue:   q,
		File:    filepath.Join(filepath.Dir(q.File), "foo.txt"),
		Printer: q.Settings.Get("printer"),
	}
	return q.Job, nil
}

func (q *Queue) SubmitJob() {
	jobCh <- *q.Job
	q.Job = nil
}

type Monitor struct {
	Path   string
	PathCh chan notify.EventInfo
	StopCh chan struct{}
	Queues map[string]Queue
}

func NewMonitor(path string, stopCh chan struct{}) *Monitor {
	return &Monitor{
		Path:   path,
		PathCh: make(chan notify.EventInfo, 10),
		StopCh: stopCh,
		Queues: make(map[string]Queue),
	}
}

func (m *Monitor) AddLPTPort(port int, name string) (queue Queue, err error) {

	device := fmt.Sprintf("LPT%d", port)

	if port < 1 || port > 9 {
		err = fmt.Errorf("Invalid device %s, only support LPT1 - LPT9", device)
		return
	}

	file := filepath.Join(m.Path, fmt.Sprintf("lptport%d.txt", port))
	if _, found := m.Queues[file]; found {
		err = fmt.Errorf("Cannot add %s, already monitoring", device)
		return
	}

	m.Queues[file] = Queue{
		Device:   device,
		File:     file,
		Name:     name,
		Settings: &dummySettings{},
	}

	queue = m.Queues[file]
	return
}

// Start TODO
func (m *Monitor) Start(ctx context.Context) {

	for _, queue := range m.Queues {
		targetPath := `\??\` + queue.File
		err := DefineDosDevice(queue.Device, targetPath, false, false, true)
		if err != nil {
			panic(err)
		}
		defer DefineDosDevice(queue.Device, targetPath, false, true, true)
	}

	if err := notify.Watch(m.Path, m.PathCh, notify.Write); err != nil {
		panic(err)
	}
	defer notify.Stop(m.PathCh)

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	timeout := 300 * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			return
		case ei := <-m.PathCh:
			file := filepath.Base(ei.Path())
			if queue, found := m.Queues[file]; found {
				log.Infof("Event %s for queue %s", ei, queue.Name)
				if ei.Event() == notify.Write {
					if !queue.Active() {
						if fi, err := os.Stat(queue.File); err != nil {
							log.Error(err)
							break
						} else {
							if fi.Size() == 0 {
								break
							}
						}
						log.Infof("Started new job for queue %s", queue.Name)
						queue.StartJob()
						queue.LastActivity = time.Now()
					} else {
						queue.LastActivity = time.Now()
					}
				}
			}
		case <-ticker.C:
			for file, queue := range m.Queues {
				if time.Since(queue.LastActivity) > timeout {
					log.Infof("Job complete for queue %s", file)
					os.Rename(queue.File, queue.File+"done")
					f, err := os.Create(queue.File)
					if err != nil {
						panic(err)
					}
					f.Close()
					queue.SubmitJob()
				}
			}
		}
	}
}
