package ui

import (
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"sync/atomic"

	"runtime"
	"sync"

	"github.com/lxn/walk"
	log "github.com/sirupsen/logrus"
	"github.com/smuething/devicemonitor/app"
)

func ShowError(owner walk.Form, title, message string) {
	walk.MsgBox(owner, title, message, walk.MsgBoxIconError|walk.MsgBoxSystemModal)
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

	app.Go(func() {
		device, ok := tray.devices["LPT1"]
		if !ok {
			return
		}
		for target := range device.Selected {
			log.Infof("Selected target: %s", target)
		}
	})

	mainWindow.Run()

}

var (
	hasStarted = int64(0)
	hasQuit    = int64(0)
)

// MenuItem is used to keep track each menu item of systray
// Don't create it directly, use the one systray.AddMenuItem() returned
type MenuItem struct {
	// Clicked is the channel which will be notified when the menu item is clicked
	Clicked chan struct{}

	// id uniquely identify a menu item, not supposed to be modified
	id int32
	// title is the text shown on menu item
	title string
	// tooltip is the text shown when pointing to menu item
	tooltip string
	// disabled menu item is grayed out and has no effect when clicked
	disabled bool
	// checked menu item has a tick before the title
	checked bool
	// parent item, for sub menus
	parent *MenuItem
}

func (item *MenuItem) String() string {
	if item.parent == nil {
		return fmt.Sprintf("MenuItem[%d, %q]", item.id, item.title)
	}
	return fmt.Sprintf("MenuItem[%d, parent %d, %q]", item.id, item.parent.id, item.title)
}

// newMenuItem returns a populated MenuItem object
func newMenuItem(title string, tooltip string, parent *MenuItem) *MenuItem {
	return &MenuItem{
		Clicked:  make(chan struct{}),
		id:       atomic.AddInt32(&currentID, 1),
		title:    title,
		tooltip:  tooltip,
		disabled: false,
		checked:  false,
		parent:   parent,
	}
}

var (
	systrayReady  func()
	systrayExit   func()
	menuItems     = make(map[int32]*MenuItem)
	menuItemsLock sync.RWMutex

	currentID = int32(-1)
)

// Run initializes GUI and starts the event loop, then invokes the onReady
// callback. It blocks until systray.Quit() is called.
// Should be called at the very beginning of main() to lock at main thread.
func Run(onReady func(), onExit func()) {
	RunWithAppWindow("", 0, 0, onReady, onExit)
}

// RunWithAppWindow is like Run but also enables an application window with the given title.
func RunWithAppWindow(title string, width int, height int, onReady func(), onExit func()) {
	runtime.LockOSThread()
	atomic.StoreInt64(&hasStarted, 1)

	if onReady == nil {
		systrayReady = func() {}
	} else {
		// Run onReady on separate goroutine to avoid blocking event loop
		readyCh := make(chan interface{})
		go func() {
			<-readyCh
			onReady()
		}()
		systrayReady = func() {
			close(readyCh)
		}
	}

	// unlike onReady, onExit runs in the event loop to make sure it has time to
	// finish before the process terminates
	if onExit == nil {
		onExit = func() {}
	}
	systrayExit = onExit

	nativeLoop(title, width, height)
}

// Quit the systray
func Quit() {
	if atomic.LoadInt64(&hasStarted) == 1 && atomic.CompareAndSwapInt64(&hasQuit, 0, 1) {
		quit()
	}
}

// AddMenuItem adds a menu item with the designated title and tooltip.
//
// It can be safely invoked from different goroutines.
func AddMenuItem(title string, tooltip string) *MenuItem {
	item := newMenuItem(title, tooltip, nil)
	item.update()
	return item
}

// AddSeparator adds a separator bar to the menu
func AddSeparator() {
	addSeparator(atomic.AddInt32(&currentID, 1))
}

// AddSubMenuItem adds a nested sub-menu item with the designated title and tooltip.
//
// It can be safely invoked from different goroutines.
func (item *MenuItem) AddSubMenuItem(title string, tooltip string) *MenuItem {
	child := newMenuItem(title, tooltip, item)
	child.update()
	return child
}

// SetTitle set the text to display on a menu item
func (item *MenuItem) SetTitle(title string) {
	item.title = title
	item.update()
}

// SetTooltip set the tooltip to show when mouse hover
func (item *MenuItem) SetTooltip(tooltip string) {
	item.tooltip = tooltip
	item.update()
}

// Disabled checks if the menu item is disabled
func (item *MenuItem) Disabled() bool {
	return item.disabled
}

// Enable a menu item regardless if it's previously enabled or not
func (item *MenuItem) Enable() {
	item.disabled = false
	item.update()
}

// Disable a menu item regardless if it's previously disabled or not
func (item *MenuItem) Disable() {
	item.disabled = true
	item.update()
}

// Hide hides a menu item
func (item *MenuItem) Hide() {
	hideMenuItem(item)
}

// Show shows a previously hidden menu item
func (item *MenuItem) Show() {
	showMenuItem(item)
}

// Checked returns if the menu item has a check mark
func (item *MenuItem) Checked() bool {
	return item.checked
}

// Check a menu item regardless if it's previously checked or not
func (item *MenuItem) Check() {
	item.checked = true
	item.update()
}

// Uncheck a menu item regardless if it's previously unchecked or not
func (item *MenuItem) Uncheck() {
	item.checked = false
	item.update()
}

// update propagates changes on a menu item to systray
func (item *MenuItem) update() {
	menuItemsLock.Lock()
	defer menuItemsLock.Unlock()
	menuItems[item.id] = item
	addOrUpdateMenuItem(item)
}

func systrayMenuItemSelected(id int32) {
	menuItemsLock.RLock()
	item := menuItems[id]
	menuItemsLock.RUnlock()
	select {
	case item.Clicked <- struct{}{}:
	// in case no one waiting for the channel
	default:
	}
}

var (
	tmpDir     string
	mainWindow *walk.MainWindow
	webView    *walk.WebView
	notifyIcon *walk.NotifyIcon

	actions = make(map[int32]*walk.Action)
	menus   = make(map[int32]*walk.Menu)

	okayToClose int32
)

func NotifyIcon() *walk.NotifyIcon {
	return notifyIcon
}

func MainWindow() *walk.MainWindow {
	return mainWindow
}

func nativeLoop(title string, width int, height int) {

	var err error
	mainWindow, err = walk.NewMainWindow()
	if err != nil {
		fail("Unable to create main window", err)
	}
	mainWindow.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		// don't close app unless we're actually finished
		actuallyClose := atomic.LoadInt32(&okayToClose) == 1
		*canceled = !actuallyClose
		if !actuallyClose {
			mainWindow.SetVisible(false)
		}
	})
	layout := walk.NewVBoxLayout()
	if err := mainWindow.SetLayout(layout); err != nil {
		fail("Unable to set main layout", err)
	}
	notifyIcon, err = walk.NewNotifyIcon(mainWindow)
	if err != nil {
		fail("Unable to create notify icon", err)
	}
	if title != "" {
		webView, err = walk.NewWebView(mainWindow)
		if err != nil {
			fail("Unable to create web view", err)
		}
		if err := mainWindow.SetTitle(title); err != nil {
			fail("Unable to set main title", err)
		}
		if err := mainWindow.SetWidth(width); err != nil {
			fail("Unable to set width", err)
		}
		if err := mainWindow.SetHeight(height); err != nil {
			fail("Unable to set height", err)
		}
	}
	systrayReady()
	mainWindow.Run()
}

func quit() {
	atomic.StoreInt32(&okayToClose, 1)
	mainWindow.Synchronize(func() {
		notifyIcon.Dispose()
		mainWindow.Close()
	})
	systrayExit()
}

// SetIcon sets the systray icon.
// iconBytes should be the content of .ico for windows and .ico/.jpg/.png
// for other platforms.
func SetIcon(idx int) {
	icon, err := walk.Resources.Icon(strconv.Itoa(idx))
	err = notifyIcon.SetIcon(icon)
	if err != nil {
		fail("Unable to set systray icon", err)
	}
	err = notifyIcon.SetVisible(true)
	if err != nil {
		fail("Unable to make systray icon visible", err)
	}
}

// SetTemplateIcon sets the systray icon as a template icon (on macOS), falling back
// to a regular icon on other platforms.
// templateIconBytes and iconBytes should be the content of .ico for windows and
// .ico/.jpg/.png for other platforms.
func SetTemplateIcon(templateIconBytes []byte, regularIconBytes int) {
	SetIcon(regularIconBytes)
}

// SetTitle sets the systray title, only available on Mac.
func SetTitle(title string) {
	// not supported on Windows
}

// SetTooltip sets the systray tooltip to display on mouse hover of the tray icon,
// only available on Mac and Windows.
func SetTooltip(tooltip string) {
	if err := notifyIcon.SetToolTip(tooltip); err != nil {
		fail("Unable to set tooltip", err)
	}
}

// ShowAppWindow shows the given URL in the application window. Only works if
// configureAppWindow has been called first.
func ShowAppWindow(url string) {
	if webView == nil {
		return
	}
	webView.SetURL(url)
	mainWindow.SetVisible(true)
}

func getOrCreateMenu(item *MenuItem) *walk.Menu {
	if item == nil {
		return notifyIcon.ContextMenu()
	}
	menu := menus[item.id]
	if menu != nil {
		return menu
	}
	menu, err := walk.NewMenu()
	if err != nil {
		fail("Unable to create new menu", err)
	}
	menus[item.id] = menu
	action := actions[item.id]
	// If we already have an action in array, it means an action is already created (as a simple action)
	// Get parent menu to remove it and create a menu entry instead
	if action != nil {
		parent := getOrCreateMenu(item.parent)
		parent.Actions().Remove(action)
		actions[item.id] = nil
		updateAction(item, getOrCreateAction(item, menu))
	}
	return menu
}

func getOrCreateAction(item *MenuItem, menu *walk.Menu) *walk.Action {
	action := actions[item.id]
	if action == nil {
		if menu != nil {
			action = walk.NewMenuAction(menu)
		} else {
			action = walk.NewAction()
		}
		action.Triggered().Attach(func() {
			select {
			case item.Clicked <- struct{}{}:
				// okay
			default:
				// no listener, ignore
			}
		})
		if err := getOrCreateMenu(item.parent).Actions().Add(action); err != nil {
			fail("Unable to add menu item to systray", err)
		}
		actions[item.id] = action
	}
	return action
}

func updateAction(item *MenuItem, action *walk.Action) {
	err := action.SetText(item.title)
	if err != nil {
		fail("Unable to set menu item text", err)
	}
	err = action.SetChecked(item.checked)
	if err != nil {
		fail("Unable to set menu item checked", err)
	}
	err = action.SetEnabled(!item.Disabled())
	if err != nil {
		fail("Unable to set menu item enabled", err)
	}
}

func addOrUpdateMenuItem(item *MenuItem) {
	updateAction(item, getOrCreateAction(item, nil))
}

// SetIcon sets the icon of a menu item. Only works on macOS and Windows.
// iconBytes should be the content of .ico/.jpg/.png
func (item *MenuItem) SetIcon(iconBytes []byte) {
	md5 := md5.Sum(iconBytes)
	filename := fmt.Sprintf("%x.ico", md5)
	iconpath := filepath.Join(walk.Resources.RootDirPath(), filename)
	// First, try to find a previously loaded icon in walk cache
	icon, err := walk.Resources.Image(filename)
	if err != nil {
		// Cache miss, load the icon
		err := ioutil.WriteFile(iconpath, iconBytes, 0644)
		if err != nil {
			fail("Unable to save icon to disk", err)
		}
		defer os.Remove(iconpath)
		icon, err = walk.Resources.Image(filename)
		if err != nil {
			fail("Unable to load icon", err)
		}
	}
	actions[item.id].SetImage(icon)
}

// SetTemplateIcon sets the icon of a menu item as a template icon (on macOS). On Windows, it
// falls back to the regular icon bytes and on Linux it does nothing.
// templateIconBytes and regularIconBytes should be the content of .ico for windows and
// .ico/.jpg/.png for other platforms.
func (item *MenuItem) SetTemplateIcon(templateIconBytes []byte, regularIconBytes []byte) {
	item.SetIcon(regularIconBytes)
}

func addSeparator(id int32) {
	action := walk.NewSeparatorAction()
	if err := notifyIcon.ContextMenu().Actions().Add(action); err != nil {
		fail("Unable to add separator", err)
	}
}

func hideMenuItem(item *MenuItem) {
	actions[item.id].SetVisible(false)
}

func showMenuItem(item *MenuItem) {
	actions[item.id].SetVisible(true)
}

func fail(msg string, err error) {
	panic(fmt.Errorf("%v: %v", msg, err))
}
