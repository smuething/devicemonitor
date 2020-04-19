package main

import (
	"os"
	"runtime/pprof"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/smuething/devicemonitor/app"
	"github.com/smuething/devicemonitor/ui"
)

func main() {
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
