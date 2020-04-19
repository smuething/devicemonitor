package ui

import (
	"fmt"

	"github.com/lxn/walk"
)

type Tray struct {
	*walk.NotifyIcon

	mw      *walk.MainWindow
	devices map[string]*walk.Action
}

func NewTray(mainWindow *walk.MainWindow) (*Tray, error) {
	tray := &Tray{
		mw:      mainWindow,
		devices: make(map[string]*walk.Action),
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

	action := walk.NewAction()
	action.SetText("Über Druckverwaltung")
	tray.ContextMenu().Actions().Add(action)
	action = walk.NewSeparatorAction()
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
