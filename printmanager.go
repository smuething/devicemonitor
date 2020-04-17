package main

//go:generate goversioninfo

import (
	"fmt"
	"sort"
	"time"

	// _ "runtime/cgo"
	"net/http"
	_ "net/http"
	_ "net/http/pprof"

	"github.com/alexbrainman/printer"
	_ "github.com/lxn/walk"
	_ "github.com/lxn/win"
	_ "github.com/rjeczalik/notify"
	"github.com/smuething/systray"
)

func main() {
	onExit := func() {
		fmt.Println("exiting")
	}

	go func() {
		fmt.Println(http.ListenAndServe("0.0.0.0:6060", nil))
	}()

	systray.Run(onReady, onExit)
	fmt.Println("Got Here")
	//pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
}

func onReady() {
	icon := 0
	systray.SetIcon(2 + icon)
	systray.SetTooltip("AM Druckerverwaltung")
	mQuitOrig := systray.AddMenuItem("Beenden", "Beendet die Druckverwaltung")
	stop := make(chan struct{})
	go func() {
		<-mQuitOrig.ClickedCh
		close(stop)
		fmt.Println("Requesting Quit")
		systray.Quit()
		fmt.Println("Finished Quitting")
	}()

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				icon = (icon + 1) % 3
				systray.SetIcon(2 + 6*icon)
			}
		}
	}()

	go func() {
		systray.AddMenuItem("Über...", "Informationen über das Programm")
		systray.AddSeparator()
		lpt1 := systray.AddMenuItem("LPT1", "LPT1")
		items := []*systray.MenuItem{}
		printers, _ := printer.ReadNames()
		defaultPrinter, _ := printer.Default()
		sort.Strings(printers)
		var activeItem *systray.MenuItem = nil
		for _, name := range printers {
			item := lpt1.AddSubMenuItem(name, name)
			if name == defaultPrinter {
				item.Check()
				activeItem = item
			}
			go func() {
				for {
					select {
					case <-stop:
						return
					case <-item.ClickedCh:
						if item != activeItem {
							activeItem.Uncheck()
							activeItem = item
							activeItem.Check()
						}
					}
				}
			}()
			items = append(items, item)
		}
	}()

}
