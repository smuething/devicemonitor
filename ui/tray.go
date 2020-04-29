package ui

import (
	"fmt"

	"github.com/smuething/devicemonitor/app"

	"github.com/alexbrainman/printer"
	"github.com/lxn/walk"
	log "github.com/sirupsen/logrus"
)

type deviceTarget struct {
	name      string
	active    bool
	separator bool
}

type DeviceMenu struct {
	*walk.Menu
	action              *walk.Action
	selected            chan string
	extendTimeout       chan bool
	printViaPDF         chan bool
	active              *walk.Action
	entries             map[string]*walk.Action
	extendTimeoutAction *walk.Action
	printViaPDFAction   *walk.Action
}

func NewDeviceMenu() (*DeviceMenu, error) {

	menu := &DeviceMenu{
		selected:      make(chan string),
		extendTimeout: make(chan bool),
		printViaPDF:   make(chan bool),
		entries:       make(map[string]*walk.Action),
	}

	var err error
	menu.Menu, err = walk.NewMenu()
	if err != nil {
		return nil, err
	}

	return menu, nil
}

func (dm *DeviceMenu) Selected() <-chan string {
	return dm.selected
}

func (dm *DeviceMenu) ExtendTimeout() <-chan bool {
	return dm.extendTimeout
}

func (dm *DeviceMenu) PrintViaPDF() <-chan bool {
	return dm.printViaPDF
}

type Tray struct {
	*walk.NotifyIcon

	mw      *walk.MainWindow
	devices map[string]*DeviceMenu
}

func NewTray(mainWindow *walk.MainWindow) (*Tray, error) {
	tray := &Tray{
		mw:      mainWindow,
		devices: make(map[string]*DeviceMenu),
	}

	var err error
	tray.NotifyIcon, err = walk.NewNotifyIcon(mainWindow)
	if err != nil {
		return nil, err
	}

	return tray, tray.setup()
}

func (tray *Tray) setup() error {

	tray.SetToolTip("Druckverwaltung")
	icon, err := walk.Resources.Icon("2")
	if err != nil {
		return fmt.Errorf("Icon konnte nicht geladen werden")
	}
	tray.SetIcon(icon)
	tray.SetVisible(true)

	return nil
}

func (tray *Tray) finalize() error {

	var action *walk.Action

	if len(tray.devices) == 0 {
		action = walk.NewAction()
		action.SetText("Keine Geräte")
		action.SetCheckable(false)
		action.SetDefault(true)
		tray.ContextMenu().Actions().Add(action)
	}

	action = walk.NewSeparatorAction()
	tray.ContextMenu().Actions().Add(action)
	action = walk.NewAction()
	action.SetText("Über Druckverwaltung")
	tray.ContextMenu().Actions().Add(action)
	action = walk.NewAction()
	action.SetText("Beenden")
	action.Triggered().Attach(func() {
		tray.mw.Synchronize(func() {
			walk.App().Exit(0)
		})
	})
	tray.ContextMenu().Actions().Add(action)

	return nil
}

func containsString(stack []string, needle string) bool {
	for _, hay := range stack {
		if hay == needle {
			return true
		}
	}
	return false
}

func (tray *Tray) addDeviceMenu(config *app.DeviceConfig) (err error) {

	menu, err := NewDeviceMenu()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			menu.Dispose()
		}
	}()

	target := config.Target
	if target == "" {
		target, _ = printer.Default()
	}

	options := []deviceTarget{
		{name: "PDF", active: "PDF" == target},
		{name: "Drucker wählen", active: "Drucker wählen" == target},
		{separator: true},
	}

	blacklist := []string{
		"PDF",
		"Drucker wählen",
	}

	printers, _ := printer.ReadNames()
	for _, printer := range printers {
		if !containsString(blacklist, printer) {
			options = append(options, deviceTarget{name: printer, active: printer == target})
		} else {
			log.Infof("Skipped blacklisted printer %s", printer)
		}
	}

	for _, option := range options {
		if option.separator {
			action := walk.NewSeparatorAction()
			menu.Actions().Add(action)
			continue
		}
		target := option.name
		if _, found := menu.entries[target]; found {
			return fmt.Errorf("Duplicate menu entry: %s", target)
		}
		action := walk.NewAction()
		action.SetText(target)
		action.SetCheckable(true)
		if option.active {
			if menu.action != nil {
				return fmt.Errorf("Cannot have more than one active device target")
			}
			menu.active = action
		}
		action.SetChecked(option.active)
		action.Triggered().Attach(func() {
			if menu.active != nil && menu.active != action {
				menu.active.SetChecked(false)
			}
			action.SetChecked(true)
			menu.active = action
			select {
			case menu.selected <- target:
			default:
				// Ignore if no receiver
			}
		})
		menu.Actions().Add(action)
		menu.entries[option.name] = action
	}

	if menu.active == nil {
		return fmt.Errorf("Must have an active device target")
	}

	action := walk.NewSeparatorAction()
	menu.Actions().Add(action)

	action = walk.NewAction()
	action.SetText("langsame Druckjobs")
	action.SetCheckable(true)
	action.SetChecked(config.ExtendTimeout)
	menu.extendTimeoutAction = action
	action.Triggered().Attach(func() {
		action := menu.extendTimeoutAction
		select {
		case menu.extendTimeout <- action.Checked():
		default:
			// ignore if no receiver
		}
	})
	menu.Actions().Add(action)

	action = walk.NewAction()
	action.SetText("Drucken als PDF")
	action.SetCheckable(true)
	action.SetChecked(config.PrintViaPDF)
	menu.printViaPDFAction = action
	action.Triggered().Attach(func() {
		action := menu.printViaPDFAction
		select {
		case menu.printViaPDF <- action.Checked():
		default:
			// ignore if no receiver
		}
	})
	menu.Actions().Add(action)

	tray.mw.Disposing().Attach(func() {
		// avoid leaking channels and stalling listening goroutines
		close(menu.selected)
		close(menu.extendTimeout)
		close(menu.printViaPDF)
	})

	menu.action, err = tray.ContextMenu().Actions().InsertMenu(len(tray.devices), menu.Menu)
	if err != nil {
		return err
	}
	menu.action.SetText(fmt.Sprintf("%s (%s)", config.Device, config.Name))
	tray.devices[config.Device] = menu

	return err

}
