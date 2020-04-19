package ui

import (
	"fmt"

	"github.com/alexbrainman/printer"
	"github.com/lxn/walk"
)

type deviceTarget struct {
	name      string
	active    bool
	separator bool
}

type DeviceMenu struct {
	*walk.Menu
	action   *walk.Action
	Selected chan string
	active   *walk.Action
	entries  map[string]*walk.Action
}

func NewDeviceMenu() (*DeviceMenu, error) {

	menu := &DeviceMenu{
		Selected: make(chan string),
		entries:  make(map[string]*walk.Action),
	}

	var err error
	menu.Menu, err = walk.NewMenu()
	if err != nil {
		return nil, err
	}

	return menu, nil
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

	tray.addDeviceMenu()

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

func (tray *Tray) addDeviceMenu() (err error) {

	menu, err := NewDeviceMenu()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			menu.Dispose()
		}
	}()

	options := []deviceTarget{
		{"PDF", false, false},
		{"Drucker wählen", false, false},
		{separator: true},
	}

	printers, _ := printer.ReadNames()
	defaultPrinter, _ := printer.Default()
	for _, printer := range printers {
		options = append(options, deviceTarget{name: printer, active: printer == defaultPrinter})
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
			case menu.Selected <- target:
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

	tray.mw.Disposing().Attach(func() {
		// avoid leaking the channel and stalling listening goroutines
		close(menu.Selected)
	})

	menu.action, err = tray.ContextMenu().Actions().InsertMenu(0, menu.Menu)
	if err != nil {
		return err
	}
	menu.action.SetText("LPT1")
	tray.devices["LPT1"] = menu

	return err

}
