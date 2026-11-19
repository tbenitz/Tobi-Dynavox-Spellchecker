package main

import (
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	hostWindowClass = "AACSpellcheckerTrayHostV1"
	wmTrayIcon      = 0x8003
	wmStartAuto     = 0x8004
	wmStartUndo     = 0x8005
	wmOpenSettings  = 0x8006
	trayIconID      = 1
	trayOpen        = 1
	trayUndo        = 2
	trayExit        = 3
)

var (
	sendTimeout           = user.NewProc("SendMessageTimeoutW")
	shell32               = syscall.NewLazyDLL("shell32.dll")
	notifyIcon            = shell32.NewProc("Shell_NotifyIconW")
	findWindow            = user.NewProc("FindWindowW")
	registerWindowMessage = user.NewProc("RegisterWindowMessageW")
	newPopupMenu          = user.NewProc("CreatePopupMenu")
	addMenuItem           = user.NewProc("AppendMenuW")
	trackMenu             = user.NewProc("TrackPopupMenu")
	destroyMenu           = user.NewProc("DestroyMenu")
	windowPID             = user.NewProc("GetWindowThreadProcessId")
	allowActivate         = user.NewProc("AllowSetForegroundWindow")
	taskbarCreated        uint32
	trayHIcon             uintptr
	trayVersion4          bool
	trayAdded             bool
	trayTooltip           = title
)

type notifyIconData struct {
	size                uint32
	window              uintptr
	id, flags, callback uint32
	icon                uintptr
	tip                 [128]uint16
	state, stateMask    uint32
	info                [256]uint16
	version             uint32
	infoTitle           [64]uint16
	infoFlags           uint32
	item                guid
	balloonIcon         uintptr
}

func wideCopy(dst []uint16, value string) {
	// Truncate in UTF-16, without splitting a surrogate pair.
	units := utf16.Encode([]rune(value))
	n := min(len(dst)-1, len(units))
	if n > 0 && n < len(units) && units[n-1] >= 0xd800 && units[n-1] <= 0xdbff {
		n--
	}
	copy(dst, units[:n])
	dst[n] = 0
}

func iconData() notifyIconData {
	return notifyIconData{size: uint32(unsafe.Sizeof(notifyIconData{})), window: hwnd, id: trayIconID}
}

func addTray() bool {
	n := iconData()
	n.flags, n.callback, n.icon = 1|2|4|0x80, wmTrayIcon, trayHIcon // NIF_MESSAGE|ICON|TIP|SHOWTIP
	wideCopy(n.tip[:], trayTooltip)
	r, _, _ := notifyIcon.Call(0, uintptr(unsafe.Pointer(&n))) // NIM_ADD
	trayAdded = r != 0
	trayVersion4 = false
	if trayAdded {
		n.version = 4
		r, _, _ = notifyIcon.Call(4, uintptr(unsafe.Pointer(&n))) // NIM_SETVERSION
		trayVersion4 = r != 0
	}
	return trayAdded
}

func removeTray() {
	if !trayAdded {
		return
	}
	n := iconData()
	notifyIcon.Call(2, uintptr(unsafe.Pointer(&n))) // NIM_DELETE
	trayAdded = false
}

func trayUpdate(tip string) {
	trayTooltip = tip
	if !trayAdded {
		return
	}
	n := iconData()
	n.flags = 4 | 0x80
	wideCopy(n.tip[:], tip)
	notifyIcon.Call(1, uintptr(unsafe.Pointer(&n))) // NIM_MODIFY
}

func menuItem(menu uintptr, flags uintptr, id uintptr, label string) {
	addMenuItem.Call(menu, flags, id, uintptr(unsafe.Pointer(wide(label))))
}

func trayMenu(x, y int32) {
	m, _, _ := newPopupMenu.Call()
	if m == 0 {
		return
	}
	defer destroyMenu.Call(m)
	menuItem(m, 0, trayOpen, "Settings")
	flags := uintptr(0)
	if !autoUndoReady || busy || holding {
		flags = 1
	} // MF_GRAYED
	menuItem(m, flags, trayUndo, "Undo last automatic correction")
	menuItem(m, 0x800, 0, "") // MF_SEPARATOR
	menuItem(m, 0, trayExit, "Exit")
	activate.Call(hwnd)
	cmd, _, _ := trackMenu.Call(m, 0x100|0x80|0x02, uintptr(x), uintptr(y), 0, hwnd, 0)
	post.Call(hwnd, 0, 0, 0) // Dismiss the menu cleanly on the next click.
	switch cmd {
	case trayOpen:
		activateApp()
	case trayUndo:
		startAutomatic(idAutoUndo, 0)
	case trayExit:
		requestExit()
	}
}

func onTrayEvent(l uintptr) {
	event := uint32(l)
	if trayVersion4 {
		event &= 0xffff
	}
	switch event {
	case 0x400, 0x401, 0x202: // NIN_SELECT, NIN_KEYSELECT, WM_LBUTTONUP
		activateApp()
	case 0x007b, 0x205: // WM_CONTEXTMENU, WM_RBUTTONUP
		var pt point
		cursor.Call(uintptr(unsafe.Pointer(&pt)))
		trayMenu(pt.x, pt.y)
	}
}

func activeRoot() uintptr {
	w, _, _ := foregroundWindow.Call()
	if w == 0 {
		return 0
	}
	root, _, _ := windowAncestor.Call(w, 2)
	return root
}

// A launch command carries only the active window handle, never a draft or key.
func handoffExisting(action uintptr, source uintptr) bool {
	for i := 0; i < 35; i++ {
		h, _, _ := findWindow.Call(uintptr(unsafe.Pointer(wide(hostWindowClass))), 0)
		if h != 0 {
			if action == wmOpenSettings {
				var pid uint32
				windowPID.Call(h, uintptr(unsafe.Pointer(&pid)))
				if pid != 0 {
					allowActivate.Call(uintptr(pid))
				}
			}
			var result uintptr
			sent, _, _ := sendTimeout.Call(h, action, source, 0, 0x23, 200, uintptr(unsafe.Pointer(&result)))
			if sent != 0 {
				return true
			}
		}
		time.Sleep(40 * time.Millisecond)
	}
	return false
}
