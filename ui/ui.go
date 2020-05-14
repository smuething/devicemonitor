package ui

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"syscall"

	"github.com/lxn/walk"
	log "github.com/sirupsen/logrus"
	"github.com/smuething/devicemonitor/app"
	"github.com/smuething/devicemonitor/monitor"
	"github.com/smuething/devicemonitor/printing"
)

func ShowError(owner walk.Form, title, message string) {
	walk.MsgBox(owner, title, message, walk.MsgBoxIconError|walk.MsgBoxSystemModal)
}

type displayErrorHook struct{}

func (displayErrorHook) Levels() []log.Level {
	return []log.Level{log.PanicLevel, log.FatalLevel, log.ErrorLevel}
}

func (displayErrorHook) Fire(entry *log.Entry) error {
	app.Go(func() {
		ShowError(nil, "Fehler", entry.Message)
	})
	return nil
}

func RunUI() {
	// make sure we stay on the main thread, otherwise we will crash sooner or later
	runtime.LockOSThread()

	config := app.Config()

	logFile, err := os.OpenFile(config.Logging.File, os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("Could not open log file %s", config.Logging.File)
	}
	log.SetOutput(logFile)

	defer func() {
		if err := recover(); err != nil {
			ShowError(nil, "Panic", fmt.Sprint(err, "\n\n", string(debug.Stack())))
		}
	}()

	mainWindow, err := walk.NewMainWindow()
	if err != nil {
		panic(err)
	}
	defer mainWindow.Dispose()

	tray, err := NewTray(mainWindow)
	defer tray.Dispose()

	var m *monitor.Monitor

	func() {
		config.Lock()
		defer config.Unlock()
		m = monitor.NewMonitor(config.Paths.SpoolDir, nil)

		deviceConfigs := make([]app.DeviceConfig, 0)
		for _, dc := range config.Devices {
			deviceConfigs = append(deviceConfigs, dc)
		}
		sort.Slice(deviceConfigs, func(i, j int) bool {
			return deviceConfigs[i].Pos < deviceConfigs[j].Pos
		})

		for i := range deviceConfigs {
			// make sure we get separate variables for the closure down below
			dc := deviceConfigs[i]
			_, err = m.AddDevice(dc.Device, dc.File, dc.Name, dc.Timeout)
			if err != nil {
				log.Fatalf("Could not add device: %s", dc.Device)
			}
			err = tray.addDeviceMenu(&dc)
			if err != nil {
				log.Fatalf("Could not create menu for device: %s", dc.Device)
			}
			app.Go(func() {
				device := tray.devices[dc.Device]
				for target := range device.Selected() {
					app.SetConfigByPath(target, "Devices", dc.Device, "target")
					func() {
						config := app.Config()
						config.Lock()
						defer config.Unlock()
						device.ResetJobTypes(config.Printer(target), config.Devices[strings.ToLower(dc.Device)].JobConfigs[strings.ToLower(target)])
					}()
				}
			})
			app.Go(func() {
				device := tray.devices[dc.Device]
				for value := range device.ExtendTimeout() {
					app.SetConfigByPath(value, "devices", dc.Device, "extend_timeout")
				}
			})
			app.Go(func() {
				device := tray.devices[dc.Device]
				for value := range device.PrintViaPDF() {
					app.SetConfigByPath(value, "devices", dc.Device, "print_via_pdf")
				}
			})
			app.Go(func() {
				device := tray.devices[dc.Device]
				for value := range device.JobConfig() {
					app.SetConfigByPath(value.JobConfig, "devices", dc.Device, "job_configs", strings.ToLower(value.Printer))
				}
			})

		}

	}()

	tray.finalize()

	app.Go(func() {
		printing.Foo(m)
		// for job := range monitor.Jobs() {
		// 	log.Infof("Processing job %s", job.Name)
		// }
	})

	app.Go(func() {
		previous := 0
		for active := range m.Spooling() {
			if active != previous {
				id := ""
				if active > 0 && previous == 0 {
					id = "8"
				}
				if previous > 0 && active == 0 {
					id = "2"
				}
				if id != "" {
					icon, err := walk.Resources.Icon(id)
					if err != nil {
						log.Fatal(err)
					}
					err = tray.SetIcon(icon)
					if err != nil {
						log.Fatal(err)
					}
				}
				previous = active
			}
		}
	})

	app.GoWithError(func() error {
		return m.Start(app.Context())
	})

	ch := make(chan os.Signal)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	app.Go(func() {
		ctx := app.Context()
		select {
		case <-ch:
			log.Infof("Received SIGTERM, shutting down")
			mainWindow.Synchronize(func() {
				walk.App().Exit(0)
			})
		case <-ctx.Done():
		}
	})

	func() {
		config.Lock()
		defer config.Unlock()
		if config.PProf.Enable {
			go func() {
				log.Println(http.ListenAndServe(config.PProf.Address, nil))
			}()
		}
	}()

	log.AddHook(displayErrorHook{})

	mainWindow.Run()

}
