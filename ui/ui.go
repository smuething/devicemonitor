package ui

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"

	"github.com/lxn/walk"
	log "github.com/sirupsen/logrus"
	"github.com/smuething/devicemonitor/app"
	"github.com/smuething/devicemonitor/monitor"
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

	defer func() {
		if err := recover(); err != nil {
			ShowError(nil, "Panic", fmt.Sprint(err, "\n\n", string(debug.Stack())))
		}
	}()

	var err error

	mainWindow, err := walk.NewMainWindow()
	if err != nil {
		panic(err)
	}
	defer mainWindow.Dispose()

	tray, err := NewTray(mainWindow)
	defer tray.Dispose()

	monitor := monitor.NewMonitor(`w:\spool`, nil)

	_, err = monitor.AddLPTPort(1, "Formulare")
	if err != nil {
		panic(err)
	}

	app.Go(func() {
		previous := 0
		for active := range monitor.Spooling() {
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
		return monitor.Start(app.Context())
	})

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	app.Go(func() {
		ctx := app.Context()
		select {
		case <-c:
			log.Infof("Received SIGTERM, shutting down")
			mainWindow.Synchronize(func() {
				walk.App().Exit(0)
			})
		case <-ctx.Done():
		}
	})

	go func() {
		log.Println(http.ListenAndServe(":6060", nil))
	}()

	log.AddHook(displayErrorHook{})

	mainWindow.Run()

}
