package main

import (
	"fmt"
	"os"
	"runtime/pprof"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/smuething/devicemonitor/app"
	"github.com/smuething/devicemonitor/ui"
)

func main() {
	if err := app.LoadConfig(os.Args[1], os.Args[2:]...); err != nil {
		ui.ShowError(nil, "Fehler beim Laden der Konfiguration", fmt.Sprintf("%s", err))
		log.Fatal(err)
	}
	ui.RunUI()
	shutdownCtx, cancelShutdown := app.ContextWithTimeout(time.Second, false)
	defer cancelShutdown()
	if !app.Shutdown(shutdownCtx) {
		ui.ShowError(nil, "Error", "Timeout during shutdown")
		pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
	} else {
		log.Infof("Successful shutdown")
	}
}
