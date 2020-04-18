// +build windows

package main

import (
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/smuething/systray"
)

var about *walk.Dialog

func CreateDialog() {
	Dialog{
		Title:    "About",
		Layout:   Grid{Columns: 3},
		MinSize:  Size{400, 300},
		AssignTo: &about,
	}.Run(systray.MainWindow())
}
