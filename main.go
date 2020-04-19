package main

import (
	"time"

	"github.com/smuething/devicemonitor/app"
	"github.com/smuething/devicemonitor/ui"
)

func main() {
	ui.RunUI()
	shutdownCtx, cancelShutdown := app.ContextWithTimeout(time.Second, false)
	defer cancelShutdown()
	if !app.Shutdown(shutdownCtx) {
		ui.ShowError(nil, "Error", "Timeout during shutdown")
	}
}
