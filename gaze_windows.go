package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var (
	enumWindows      = user.NewProc("EnumWindows")
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
	WindowImage string
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
	if !ok(el.invoke(20, uintptr(unsafe.Pointer(&pid)))) {
		return ""
	}
	return imageForPID(pid)
}

func imageForWindow(w uintptr) string {
	var pid uint32
	windowPID.Call(w, uintptr(unsafe.Pointer(&pid)))
	return imageForPID(pid)
}

func imageForPID(pid uint32) string {
	if pid == 0 || pid == uint32(os.Getpid()) {
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
	f, text, err := getField(ui, &at)
	if err != nil {
		return p, errors.New("That field does not expose writable plain text to Windows. No gaze shortcut was linked.")
	}
	defer f.el.release()
	next := gazeProfile{WindowClass: windowClass(w), WindowImage: imageForWindow(w), Engine: p.Engine,
		Field: fieldKey{Executable: elementImage(f.el), ID: elementString(f.el, 29), Class: elementString(f.el, 30)}}
	if next.Field.ID == "" {
		if strings.TrimSpace(text) != "" {
			return p, errors.New("This field has no accessibility ID. Clear the draft and link the empty Message Window once.")
		}
		name := strings.TrimSpace(elementString(f.el, 23))
		// A provider may expose its value as its name. Only accept an
		// accessible label when the field is empty.
		if name != "" && len(name) <= 100 && !strings.ContainsAny(name, "\r\n") {
			next.Field.Name = name
		}
	}
	if next.WindowClass == "" || !next.Field.valid() {
		return p, errors.New("This field has no stable Windows accessibility ID or name. It cannot be linked safely.")
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

var (
	errLinkedWindowAmbiguous = errors.New("More than one AAC field matches the link. No draft was read or changed.")
	errLinkedFieldMissing    = errors.New("The linked AAC field is not visible or has changed. Relink it if the AAC app was updated.")
)

func linkedWindowCandidate(w uintptr, p gazeProfile) bool {
	if w == 0 || p.WindowClass == "" || windowClass(w) != p.WindowClass {
		return false
	}
	iconic, _, _ := isIconic.Call(w)
	visible, _, _ := isWindowVisible.Call(w)
	if iconic != 0 || visible == 0 {
		return false
	}
	return p.WindowImage == "" || strings.EqualFold(imageForWindow(w), p.WindowImage)
}

// Match the saved field identity without reading anyone's draft yet.
func matchGazeField(ui *com, w uintptr, p gazeProfile) (*captured, error) {
	if !p.Field.valid() {
		return nil, errors.New("One-time setup is needed. Open the EXE normally and choose Link AAC field.")
	}
	if !linkedWindowCandidate(w, p) {
		return nil, errors.New("The linked AAC window is not available. Open its message page.")
	}
	var root, cond, items *com
	if !ok(ui.invoke(6, w, uintptr(unsafe.Pointer(&root)))) || root == nil {
		return nil, errors.New("Windows could not inspect the AAC window.")
	}
	defer root.release()
	if !ok(ui.invoke(21, uintptr(unsafe.Pointer(&cond)))) || cond == nil {
		return nil, errors.New("Windows could not prepare the AAC field search.")
	}
	defer cond.release()
	if !ok(root.invoke(6, 7, uintptr(unsafe.Pointer(cond)), uintptr(unsafe.Pointer(&items)))) || items == nil {
		return nil, errors.New("Windows could not search the AAC window.")
	}
	defer items.release()
	var count int32
	if !ok(items.invoke(3, uintptr(unsafe.Pointer(&count)))) || count < 1 || count > 4000 {
		return nil, errors.New("The linked AAC field could not be identified in this window.")
	}
	var found *com
	for i := int32(0); i < count; i++ {
		var el *com
		if !ok(items.invoke(4, uintptr(i), uintptr(unsafe.Pointer(&el)))) || el == nil {
			found.release()
			return nil, errors.New("The AAC window changed during the field search. Try again.")
		}
		candidate := fieldKey{Class: elementString(el, 30)}
		if candidate.Class == p.Field.Class {
			if p.Field.ID != "" {
				candidate.ID = elementString(el, 29)
			} else {
				candidate.Name = strings.TrimSpace(elementString(el, 23))
			}
			if p.Field.ID != "" && candidate.ID == p.Field.ID || p.Field.ID == "" && candidate.Name == p.Field.Name {
				candidate.Executable = elementImage(el)
			}
		}
		if !p.Field.matches(candidate) || !eligibleGazeElement(el) {
			el.release()
			continue
		}
		if found != nil {
			found.release()
			el.release()
			return nil, errLinkedWindowAmbiguous
		}
		found = el
	}
	if found == nil {
		return nil, errLinkedFieldMissing
	}
	return &captured{found}, nil
}

func findGazeField(ui *com, w uintptr, p gazeProfile) (*captured, string, error) {
	f, err := matchGazeField(ui, w, p)
	if err != nil {
		return nil, "", err
	}
	s, err := f.read()
	if err != nil {
		f.el.release()
		return nil, "", errors.New("The linked AAC field is not writable right now. No text was changed.")
	}
	return f, s, nil
}

var (
	windowListLock   sync.Mutex
	listedWindows    []uintptr
	collectAACWindow = syscall.NewCallback(func(w, _ uintptr) uintptr {
		listedWindows = append(listedWindows, w)
		return 1
	})
)

func visibleDesktopWindows() ([]uintptr, bool) {
	windowListLock.Lock()
	defer windowListLock.Unlock()
	listedWindows = nil
	ok, _, _ := enumWindows.Call(collectAACWindow, 0)
	windows := listedWindows
	listedWindows = nil
	return windows, ok != 0
}

func resolveGazeTarget(ui *com, p gazeProfile) (uintptr, *captured, string, error) {
	if !p.Field.valid() || p.WindowClass == "" {
		return 0, nil, "", errors.New("No AAC field is linked. Open Settings and use Link AAC field once before using the TD Snap button.")
	}
	windows, enumerated := visibleDesktopWindows()
	var candidates []uintptr
	for _, w := range windows {
		if linkedWindowCandidate(w, p) {
			candidates = append(candidates, w)
		}
	}
	if !enumerated {
		return 0, nil, "", errors.New("Windows could not list open AAC windows.")
	}
	var matches []uintptr
	for _, w := range candidates {
		f, err := matchGazeField(ui, w, p)
		if errors.Is(err, errLinkedWindowAmbiguous) {
			return 0, nil, "", err
		}
		if p.WindowImage != "" && err != nil && !errors.Is(err, errLinkedFieldMissing) {
			return 0, nil, "", err
		}
		if err == nil {
			f.el.release()
			matches = append(matches, w)
		}
	}
	w, err := chooseLinkedWindow(matches)
	if err != nil {
		return 0, nil, "", err
	}
	f, s, err := findGazeField(ui, w, p)
	return w, f, s, err
}

// Keep an in-memory undo of only the last successful automatic edit. The
// persisted link never contains a draft or correction.
type autoRecord struct {
	source              uintptr
	profile             gazeProfile
	field               *captured
	original, corrected string
}

func (r *autoRecord) release() {
	if r != nil && r.field != nil {
		r.field.el.release()
	}
}
func (r *autoRecord) verify(ui *com) error {
	current, _, err := findGazeField(ui, r.source, r.profile)
	if err != nil {
		return err
	}
	defer current.el.release()
	var same uint32
	hr := ui.invoke(3, uintptr(unsafe.Pointer(r.field.el)), uintptr(unsafe.Pointer(current.el)), uintptr(unsafe.Pointer(&same)))
	if !ok(hr) || same == 0 {
		return errors.New("The linked AAC field changed. Nothing was written.")
	}
	return nil
}
