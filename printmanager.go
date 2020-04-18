package main

//go:generate goversioninfo

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	// _ "runtime/cgo"
	"net/http"
	_ "net/http"
	_ "net/http/pprof"

	"github.com/alexbrainman/printer"
	_ "github.com/lxn/win"
	_ "github.com/rjeczalik/notify"
	"github.com/smuething/systray"
	"golang.org/x/sys/windows"
)

var stop chan struct{} = make(chan struct{})

func main() {
	onExit := func() {
		fmt.Println("exiting")
	}

	go func() {
		fmt.Println(http.ListenAndServe("0.0.0.0:6060", nil))
	}()

	path := os.Args[1]
	longpath, _ := fixLongPath(path)
	fmt.Printf("Input: %s Output: %s\n", path, longpath)

	targets, err := QueryDosDevice("LPT1")
	if err != nil {
		if err == windows.ERROR_FILE_NOT_FOUND {
			fmt.Println("LPT1 nicht gefunden")
		} else {
			panic(err)
		}
	} else {
		fmt.Printf("LPT1 -> %v\n", targets)
	}

	err = DefineDosDevice("LPT1", path, false, false, true)
	if err != nil {
		panic(err)
	}
	defer func() {
		DefineDosDevice("LPT1", path, false, true, true)
		targets, err = QueryDosDevice("LPT1")
		if err != nil {
			if err == windows.ERROR_FILE_NOT_FOUND {
				fmt.Println("LPT1 nicht gefunden")
			} else {
				panic(err)
			}
		} else {
			fmt.Printf("LPT1 -> %v\n", targets)
		}
	}()

	targets, err = QueryDosDevice("LPT1")
	if err != nil {
		if err == windows.ERROR_FILE_NOT_FOUND {
			fmt.Println("LPT1 nicht gefunden")
		} else {
			panic(err)
		}
	} else {
		fmt.Printf("LPT1 -> %v\n", targets)
	}

	m := NewMonitor(`w:\`, stop)
	go m.Start(context.Background())

	systray.Run(onReady, onExit)
	fmt.Println("Got Here")
}

func onReady() {
	icon := 0
	systray.SetIcon(2 + icon)
	systray.SetTooltip("AM Druckerverwaltung")
	mQuitOrig := systray.AddMenuItem("Beenden", "Beendet die Druckverwaltung")
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
		aItem := systray.AddMenuItem("Über...", "Informationen über das Programm")
		go func() {
			for {
				select {
				case <-stop:
					return
				case <-aItem.ClickedCh:
					if about == nil {
						CreateDialog()
					} else {
						about.Show()
					}
				}
			}
		}()
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

	go func() {

		ticker := time.NewTicker(time.Second)
		shown := false
		time := 0

		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if shown {
					time++
					if time >= 2 {
						systray.NotifyIcon().ShowInfo("", "")
						shown = false
						time = 0
					}
				} else {
					time++
					if time >= 4 {
						systray.NotifyIcon().ShowInfo("Drucken", "Es wird gedruckt\nIst das nicht toll?")
						shown = true
						time = 0
					}
				}
			}
		}

	}()

}
