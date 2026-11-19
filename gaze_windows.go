package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	foregroundWindow = user.NewProc("GetForegroundWindow")
	windowFromPoint  = user.NewProc("WindowFromPoint")
	windowAncestor   = user.NewProc("GetAncestor")
	getClassName     = user.NewProc("GetClassNameW")
	isIconic         = user.NewProc("IsIconic")
	isWindowVisible  = user.NewProc("IsWindowVisible")
	destroyWindow    = user.NewProc("DestroyWindow")
	setDPIAware      = user.NewProc("SetProcessDPIAware")
	openProcess      = kernel.NewProc("OpenProcess")
	processImage     = kernel.NewProc("QueryFullProcessImageNameW")
	closeHandle      = kernel.NewProc("CloseHandle")
	createMutex      = kernel.NewProc("CreateMutexW")
)

type gazeProfile struct {
	WindowClass string
	Field       fieldKey
	Engine      string
}

func profilePath() (string, error) {
	root, err := os.UserConfigDir()
	return filepath.Join(root, "AAC Spellchecker", "gaze-v1.json"), err
}

func loadProfile() gazeProfile {
	var p gazeProfile
	path, err := profilePath()
	if err == nil {
		if data, err := os.ReadFile(path); err == nil && len(data) < 65536 {
			if json.Unmarshal(data, &p) == nil {
				return p
			}
		}
	}
	return gazeProfile{}
}

func saveProfile(p gazeProfile) error {
	path, err := profilePath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "gaze-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	writeErr := json.NewEncoder(f).Encode(p)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

func pointBits(p point) uintptr {
	return uintptr(uint64(uint32(p.x)) | uint64(uint32(p.y))<<32)
}

func rootWindowAt(p point) uintptr {
	w, _, _ := windowFromPoint.Call(pointBits(p))
	r, _, _ := windowAncestor.Call(w, 2) // GA_ROOT
	return r
}

func windowClass(w uintptr) string {
	var buf [256]uint16
	n, _, _ := getClassName.Call(w, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

func elementString(el *com, slot int) string {
	var s *uint16
	if !ok(el.invoke(slot, uintptr(unsafe.Pointer(&s)))) || s == nil {
		return ""
	}
	defer freeBSTR.Call(uintptr(unsafe.Pointer(s)))
	n, _, _ := lenBSTR.Call(uintptr(unsafe.Pointer(s)))
	if n > 4096 {
		return ""
	}
	return syscall.UTF16ToString(unsafe.Slice(s, int(n)))
}

func elementImage(el *com) string {
	var pid uint32
	if !ok(el.invoke(20, uintptr(unsafe.Pointer(&pid)))) || pid == 0 || pid == uint32(os.Getpid()) {
		return ""
	}
	h, _, _ := openProcess.Call(0x1000, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer closeHandle.Call(h)
	buf := make([]uint16, 32768)
	n := uint32(len(buf))
	r, _, _ := processImage.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r == 0 || n == 0 || int(n) > len(buf) {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

func eligibleGazeElement(el *com) bool {
	var offscreen uint32
	return ok(el.invoke(38, uintptr(unsafe.Pointer(&offscreen)))) && offscreen == 0
}

func bindGazeField(ui *com, at point, p gazeProfile) (gazeProfile, error) {
	w := rootWindowAt(at)
	if w == 0 {
		return p, errors.New("No AAC window was found under the pointer.")
	}
	f, _, err := getField(ui, &at)
	if err != nil {
		return p, errors.New("That field does not expose writable plain text to Windows. No gaze shortcut was linked.")
	}
	defer f.el.release()
	next := gazeProfile{WindowClass: windowClass(w), Engine: p.Engine,
		Field: fieldKey{elementImage(f.el), elementString(f.el, 29), elementString(f.el, 30)}}
	if next.WindowClass == "" || !next.Field.valid() {
		return p, errors.New("This field has no stable Windows accessibility ID. It cannot be linked safely.")
	}
	found, _, err := findGazeField(ui, w, next)
	if err != nil {
		return p, err
	}
	defer found.el.release()
	var same uint32
	if !ok(ui.invoke(3, uintptr(unsafe.Pointer(f.el)), uintptr(unsafe.Pointer(found.el)), uintptr(unsafe.Pointer(&same)))) || same == 0 {
		return p, errors.New("The selected AAC field could not be verified.")
	}
	return next, nil
}

// Search only the window that was active when the AAC button launched the EXE.
// No synthetic clicks, clipboard reads, or focus-based writes are used.
func findGazeField(ui *com, w uintptr, p gazeProfile) (*captured, string, error) {
	if !p.Field.valid() {
		return nil, "", errors.New("One-time setup is needed. Open the EXE normally and choose Link AAC field.")
	}
	iconic, _, _ := isIconic.Call(w)
	visible, _, _ := isWindowVisible.Call(w)
	if w == 0 || p.WindowClass == "" || windowClass(w) != p.WindowClass || iconic != 0 || visible == 0 {
		return nil, "", errors.New("The linked AAC window is not active. Open AAC and try its Spell Check button again.")
	}
	var root, cond, items *com
	if !ok(ui.invoke(6, w, uintptr(unsafe.Pointer(&root)))) || root == nil {
		return nil, "", errors.New("Windows could not inspect the AAC window.")
	}
	defer root.release()
	if !ok(ui.invoke(21, uintptr(unsafe.Pointer(&cond)))) || cond == nil {
		return nil, "", errors.New("Windows could not prepare the AAC field search.")
	}
	defer cond.release()
	if !ok(root.invoke(6, 7, uintptr(unsafe.Pointer(cond)), uintptr(unsafe.Pointer(&items)))) || items == nil {
		return nil, "", errors.New("Windows could not search the AAC window.")
	}
	defer items.release()
	var count int32
	if !ok(items.invoke(3, uintptr(unsafe.Pointer(&count)))) || count < 1 || count > 4000 {
		return nil, "", errors.New("The linked AAC field could not be identified in this window.")
	}
	var found *com
	for i := int32(0); i < count; i++ {
		var el *com
		if !ok(items.invoke(4, uintptr(i), uintptr(unsafe.Pointer(&el)))) || el == nil {
			found.release()
			return nil, "", errors.New("The AAC window changed during the field search. Try again.")
		}
		id, class := elementString(el, 29), elementString(el, 30)
		if id != p.Field.ID || class != p.Field.Class || !eligibleGazeElement(el) ||
			!p.Field.matches(fieldKey{elementImage(el), id, class}) {
			el.release()
			continue
		}
		if found != nil {
			found.release()
			el.release()
			return nil, "", errors.New("More than one AAC field matches the link. No draft was read or changed.")
		}
		found = el
	}
	if found == nil {
		return nil, "", errors.New("The linked AAC field is not visible or has changed. Relink it if the AAC app was updated.")
	}
	f := &captured{found}
	s, err := f.read()
	if err != nil {
		found.release()
		return nil, "", errors.New("The linked AAC field is not writable right now. No text was changed.")
	}
	return f, s, nil
}
