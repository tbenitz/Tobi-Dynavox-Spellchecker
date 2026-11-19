package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

var (
	user             = syscall.NewLazyDLL("user32.dll")
	kernel           = syscall.NewLazyDLL("kernel32.dll")
	gdi              = syscall.NewLazyDLL("gdi32.dll")
	regClass         = user.NewProc("RegisterClassExW")
	createWin        = user.NewProc("CreateWindowExW")
	defProc          = user.NewProc("DefWindowProcW")
	show             = user.NewProc("ShowWindow")
	update           = user.NewProc("UpdateWindow")
	activate         = user.NewProc("SetForegroundWindow")
	getMsg           = user.NewProc("GetMessageW")
	isDialog         = user.NewProc("IsDialogMessageW")
	translate        = user.NewProc("TranslateMessage")
	dispatch         = user.NewProc("DispatchMessageW")
	send             = user.NewProc("SendMessageW")
	post             = user.NewProc("PostMessageW")
	quit             = user.NewProc("PostQuitMessage")
	getTextLen       = user.NewProc("GetWindowTextLengthW")
	getText          = user.NewProc("GetWindowTextW")
	setText          = user.NewProc("SetWindowTextW")
	cursor           = user.NewProc("GetCursorPos")
	setFocus         = user.NewProc("SetFocus")
	loadCursor       = user.NewProc("LoadCursorW")
	loadIcon         = user.NewProc("LoadIconW")
	clientRect       = user.NewProc("GetClientRect")
	move             = user.NewProc("MoveWindow")
	enable           = user.NewProc("EnableWindow")
	registerHotkey   = user.NewProc("RegisterHotKey")
	unregisterHotkey = user.NewProc("UnregisterHotKey")
	setTimer         = user.NewProc("SetTimer")
	killTimer        = user.NewProc("KillTimer")
	getModule        = kernel.NewProc("GetModuleHandleW")
	createFont       = gdi.NewProc("CreateFontW")
	deleteGDI        = gdi.NewProc("DeleteObject")
	msgBox           = user.NewProc("MessageBoxW")
	openClip         = user.NewProc("OpenClipboard")
	closeClip        = user.NewProc("CloseClipboard")
	getClip          = user.NewProc("GetClipboardData")
	setClip          = user.NewProc("SetClipboardData")
	emptyClip        = user.NewProc("EmptyClipboard")
	globalAlloc      = kernel.NewProc("GlobalAlloc")
	globalSize       = kernel.NewProc("GlobalSize")
	globalLock       = kernel.NewProc("GlobalLock")
	globalUnlock     = kernel.NewProc("GlobalUnlock")
	globalFree       = kernel.NewProc("GlobalFree")
	moveMemory       = kernel.NewProc("RtlMoveMemory")
)

type point struct{ x, y int32 }
type rect struct{ l, t, r, b int32 }
type message struct {
	hwnd    uintptr
	message uint32
	w, l    uintptr
	time    uint32
	pt      point
	private uint32
}
type wndClass struct {
	size, style                        uint32
	proc                               uintptr
	clsExtra, winExtra                 int32
	instance, icon, cursor, background uintptr
	menu, class                        *uint16
	iconSmall                          uintptr
}

const (
	title           = "AAC Spellchecker"
	wmDone          = 0x8001
	wmModelProgress = 0x8002
	idCapture       = 101 + iota
	idPaste
	idCheck
	idApply
	idUndo
	idCopy
	idClear
	idInput
	idOutput
	idHeader
	idHelp
	idOriginal
	idPreview
	idStatus
	idEngineLabel
	idEngine
	idLink
	idAuto
	idAutoUndo
	idCloudKeyLabel
	idCloudKey
	idCloudSave
	idCloudRemove
)

type task struct {
	kind     int
	at       *point
	s        string
	engine   engine
	source   uintptr
	profile  gazeProfile
	approved bool
}
type result struct {
	kind                      int
	status, original, preview string
	checked, attached, undo   bool
	err                       error
	profile                   gazeProfile
	autoUndo                  bool
}

var jobs = make(chan task, 1)
var results = make(chan result, 1)
var statusUpdates = make(chan string, 16)
var appModelContext, stopModels = context.WithCancel(context.Background())

func worker(hwnd uintptr) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	r, _, _ := coInit.Call(0, 0)
	ready := ok(r)
	if ready {
		defer coUninit.Call()
	}
	var ui, sp *com
	defer func() { ui.release(); sp.release() }()
	var field *captured
	var capturedText, undoText string
	var automaticUndo *autoRecord
	defer func() { automaticUndo.release() }()
	var canUndo bool
	detach := func() {
		if field != nil {
			field.el.release()
		}
		field = nil
		capturedText = ""
		canUndo = false
		undoText = ""
	}
	defer detach()
	check := func(e engine, s string, approved bool) (string, int, error) {
		if p, cloud := providerFor(e.id); cloud {
			if !approved {
				return "", 0, fmt.Errorf("Start a spelling check explicitly to use %s.", p.name)
			}
			key, err := readCloudKey(p)
			if err != nil {
				return "", 0, fmt.Errorf("Save a %s API key in the main spellchecker window, or use Windows spelling.", p.name)
			}
			return spellCloud(appModelContext, cloudHTTP, p, key, s, approved)
		}
		if e.id != "windows" {
			return spellModel(appModelContext, e, s, func(message string) {
				select {
				case statusUpdates <- message:
					post.Call(hwnd, wmModelProgress, 0, 0)
				default:
				}
			})
		}
		if !ready {
			return "", 0, fmt.Errorf("Windows text services did not start.")
		}
		if sp == nil {
			var err error
			sp, err = spellChecker("en-US")
			if err != nil {
				return "", 0, err
			}
		}
		return propose(sp, s)
	}
	ensureUI := func() error {
		if !ready {
			return fmt.Errorf("Windows accessibility services did not start.")
		}
		if ui == nil {
			var err error
			ui, err = makeCOM(&uiaClass, &uiaIID)
			return err
		}
		return nil
	}
	for j := range jobs {
		out := result{kind: j.kind}
		switch j.kind {
		case idAuto:
			if out.err = ensureUI(); out.err != nil {
				break
			}
			if j.source == 0 || activeRoot() != j.source {
				out.err = fmt.Errorf("The linked AAC window is no longer active. No text was changed.")
				break
			}
			f, original, err := findGazeField(ui, j.source, j.profile)
			if err != nil {
				out.err = err
				break
			}
			record := &autoRecord{source: j.source, profile: j.profile, field: f, original: original}
			var changed bool
			record.corrected, changed, out.err = autoCorrect(f, original, func(draft string) (string, error) {
				if activeRoot() != j.source || appModelContext.Err() != nil {
					return "", fmt.Errorf("The linked AAC window is no longer active. No draft was sent or changed.")
				}
				fresh, err := f.read()
				if err != nil || fresh != original || activeRoot() != j.source {
					return "", fmt.Errorf("The draft changed before checking. No draft was sent or changed.")
				}
				// Pressing the TD Snap action is the request to check this one
				// draft using the selected engine and saved credentials.
				preview, _, err := check(j.engine, draft, true)
				return preview, err
			}, func() error {
				if appModelContext.Err() != nil || activeRoot() != j.source {
					return fmt.Errorf("The AAC window changed. The correction was not applied.")
				}
				if err := record.verify(ui); err != nil {
					return err
				}
				if activeRoot() != j.source {
					return fmt.Errorf("The AAC window changed. The correction was not applied.")
				}
				return nil
			})
			if changed {
				automaticUndo.release()
				automaticUndo = record
				out.status = "Corrected the AAC draft."
			} else if mayHaveWritten(out.err) {
				// Windows may have written the proposal even when read-back
				// failed. Undo still verifies identity and exact text.
				automaticUndo.release()
				automaticUndo = record
			} else {
				record.release()
				out.status = "No spelling changes."
			}
		case idAutoUndo:
			if automaticUndo == nil {
				out.err = fmt.Errorf("There is no automatic correction to undo.")
				break
			}
			if j.source != 0 && (activeRoot() != j.source || j.source != automaticUndo.source) {
				out.err = fmt.Errorf("The window does not match the last corrected AAC draft.")
				break
			}
			if out.err = automaticUndo.verify(ui); out.err == nil {
				out.err = writeReviewed(automaticUndo.field, automaticUndo.corrected, automaticUndo.original)
			}
			if out.err != nil {
				break
			}
			automaticUndo.release()
			automaticUndo = nil
			out.status = "Restored the original AAC draft."
		case idCapture, idLink:
			detach()
			if out.err = ensureUI(); out.err != nil {
				break
			}
			if j.kind == idLink {
				if j.at == nil {
					out.err = fmt.Errorf("Point at the AAC Message Window to link it.")
					break
				}
				out.profile, out.err = bindGazeField(ui, *j.at, j.profile)
				out.status = "AAC field linked. Add the Spell Check button to your AAC page."
				break
			}
			field, capturedText, out.err = getField(ui, j.at)
			if out.err != nil {
				break
			}
			if strings.TrimSpace(capturedText) == "" {
				detach()
				out.err = fmt.Errorf("The captured field was empty. Write a draft in the AAC app first.")
				break
			}
			out.original = capturedText
			if _, cloud := providerFor(j.engine.id); cloud {
				out.status = "Draft captured. Choose Check spelling to check it with the selected cloud engine."
				break
			}
			out.preview = capturedText
			var n int
			proposed, n, err := check(j.engine, capturedText, false)
			if err != nil {
				out.err = err
				break
			}
			out.preview = proposed
			out.checked = true
			out.status = checkStatus(j.engine, n, true)
		case idCheck:
			var n int
			if j.approved && field != nil {
				current, err := field.read()
				if err != nil || current != capturedText {
					out.err = fmt.Errorf("The AAC draft changed. Nothing was sent. Return to AAC and check the latest draft.")
					break
				}
				j.s = capturedText
			}
			out.preview, n, out.err = check(j.engine, j.s, j.approved)
			out.checked = out.err == nil
			out.status = checkStatus(j.engine, n, field != nil)
		case idApply:
			if field == nil {
				out.err = fmt.Errorf("No AAC field is attached. Use Copy correction and paste in AAC.")
				break
			}
			j.s = matchLineEndings(j.s, capturedText)
			out.err = writeReviewed(field, capturedText, j.s)
			if out.err == nil {
				undoText, capturedText = capturedText, j.s
				canUndo = true
				out.original = j.s
				out.preview = j.s
				out.checked = true
				out.status = "Updated the captured AAC field. You can undo this replacement."
			}
		case idUndo:
			if field == nil || !canUndo {
				out.err = fmt.Errorf("There is no replacement to undo.")
				break
			}
			out.err = writeReviewed(field, capturedText, undoText)
			if out.err == nil {
				capturedText = undoText
				canUndo = false
				undoText = ""
				out.original = capturedText
				out.preview = capturedText
				out.checked = false
				out.status = "Restored the original draft in AAC."
			}
		case idClear, idPaste:
			detach()
			out.original = j.s
			if j.kind == idPaste {
				out.status = "Draft pasted. Choose Check spelling to review suggestions."
			} else {
				out.status = "Cleared this window. The AAC draft was not changed."
			}
		}
		out.attached = field != nil
		out.undo = canUndo
		out.autoUndo = automaticUndo != nil
		results <- out
		post.Call(hwnd, wmDone, 0, 0)
	}
}

func checkStatus(e engine, n int, attached bool) string {
	if e.id == "windows" {
		return spellStatus(n, attached)
	}
	if n == 0 {
		return "The selected model left this draft unchanged. You can still edit the preview."
	}
	if attached {
		return "Model preview ready. Review the changes before choosing Apply to AAC."
	}
	return "Model preview ready. Review the changes before choosing Copy correction."
}

func spellStatus(n int, attached bool) string {
	if n == 0 {
		return "No spelling replacements suggested. You can still edit the preview."
	}
	if attached {
		return fmt.Sprintf("%d spelling suggestion(s). Review, then choose Apply to AAC.", n)
	}
	return fmt.Sprintf("%d spelling suggestion(s). Review, then choose Copy correction.", n)
}

var controls = map[int]uintptr{}
var hwnd, instance, font, larger uintptr
var busy, holding, attached, undo, previewValid, updating, hotkeyOK bool
var pendingCapture = idCapture
var profile gazeProfile
var autoUndoReady, exiting bool

func main() {
	runtime.LockOSThread()
	setDPIAware.Call()
	action := uintptr(wmOpenSettings)
	if len(os.Args) > 1 {
		if len(os.Args) != 2 {
			return
		}
		switch os.Args[1] {
		case "--check":
			action = wmStartAuto
		case "--undo":
			action = wmStartUndo
		default:
			return
		}
	}
	source := activeRoot()
	var lock uintptr
	for attempt := 0; attempt < 2; attempt++ {
		var lockErr error
		lock, _, lockErr = createMutex.Call(0, 0, uintptr(unsafe.Pointer(wide(`Local\AACSpellcheckerTrayHostV1`))))
		if lock == 0 {
			return
		}
		if lockErr != syscall.Errno(183) {
			break
		}
		closeHandle.Call(lock)
		lock = 0
		if handoffExisting(action, source) {
			return
		}
		// The first instance may have exited during handoff. If so, we
		// can become the tray host without losing the original request.
	}
	if lock == 0 {
		return
	}
	defer closeHandle.Call(lock)
	profile = loadProfile()
	instance, _, _ = getModule.Call(0)
	c, _, _ := loadCursor.Call(0, 32512)
	trayHIcon, _, _ = loadIcon.Call(0, 32512)
	mid, _, _ := registerWindowMessage.Call(uintptr(unsafe.Pointer(wide("TaskbarCreated"))))
	taskbarCreated = uint32(mid)
	wc := wndClass{proc: syscall.NewCallback(wndProc), instance: instance, cursor: c, icon: trayHIcon, iconSmall: trayHIcon, background: 16, class: wide(hostWindowClass)}
	wc.size = uint32(unsafe.Sizeof(wc))
	atom, _, _ := regClass.Call(uintptr(unsafe.Pointer(&wc)))
	if atom != 0 {
		hwnd = create(0, hostWindowClass, title, 0x00cf0000, 0x80000000, 0x80000000, 730, 600, 0, 0)
	}
	if hwnd == 0 {
		if action == wmOpenSettings {
			msgBox.Call(0, uintptr(unsafe.Pointer(wide("The AAC Spellchecker window could not start."))), uintptr(unsafe.Pointer(wide(title))), 0x10)
		}
		return
	}
	go worker(hwnd)
	addTray()
	hot, _, _ := registerHotkey.Call(hwnd, 1, 0x4003, 0x77)
	hotkeyOK = hot != 0
	switch action {
	case wmStartAuto:
		startAutomatic(idAuto, source)
	case wmStartUndo:
		startAutomatic(idAutoUndo, source)
	default:
		show.Call(hwnd, 5)
		update.Call(hwnd)
	}
	var m message
	for {
		r, _, _ := getMsg.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if m.message >= 0x100 && m.message <= 0x109 {
			handled, _, _ := isDialog.Call(hwnd, uintptr(unsafe.Pointer(&m)))
			if handled != 0 {
				continue
			}
		}
		translate.Call(uintptr(unsafe.Pointer(&m)))
		dispatch.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func create(ex uintptr, class, caption string, style, x, y, w, h, owner, id uintptr) uintptr {
	r, _, _ := createWin.Call(ex, uintptr(unsafe.Pointer(wide(class))), uintptr(unsafe.Pointer(wide(caption))), style, x, y, w, h, owner, id, instance, 0)
	return r
}
func widget(parent uintptr, id int, class, caption string, style, ex, font uintptr) {
	h := create(ex, class, caption, style|0x50000000, 0, 0, 1, 1, parent, uintptr(id))
	controls[id] = h
	send.Call(h, 0x30, font, 1)
}
func makeFont(h, weight int) uintptr {
	r, _, _ := createFont.Call(uintptr(-h), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(wide("Segoe UI"))))
	return r
}
func initControls(h uintptr) {
	font, larger = makeFont(18, 400), makeFont(25, 650)
	widget(h, idHeader, "STATIC", title, 0, 0, larger)
	widget(h, idHelp, "STATIC", "The TD Snap Spell Check button edits your linked draft in one step. Close this window to leave the app in the tray.", 0, 0, font)
	widget(h, idOriginal, "STATIC", "Original draft", 0, 0, font)
	widget(h, idEngineLabel, "STATIC", "&Spelling engine", 0, 0, font)
	widget(h, idEngine, "COMBOBOX", "", 0x210003, 0, font) // tab stop, scroll, dropdown list
	for _, e := range engines {
		send.Call(controls[idEngine], 0x143, 0, uintptr(unsafe.Pointer(wide(e.label))))
	}
	send.Call(controls[idEngine], 0x14e, uintptr(engineIndex(profile.Engine)), 0)
	widget(h, idPreview, "STATIC", "Review and edit the correction", 0, 0, font)
	widget(h, idStatus, "STATIC", "Offline English (US) spelling. This app does not press Speak.", 0, 0, font)
	widget(h, idCloudKeyLabel, "STATIC", "API &key", 0, 0, font)
	widget(h, idCloudKey, "EDIT", "", 0x100a0, 0x200, font) // ES_PASSWORD and tab stop
	send.Call(controls[idCloudKey], 0xC5, 512, 0)
	widget(h, idCloudSave, "BUTTON", "Sa&ve key", 0x10000, 0, font)
	widget(h, idCloudRemove, "BUTTON", "&Remove key", 0x10000, 0, font)
	for _, b := range []struct {
		id    int
		label string
	}{
		{idLink, "&Link AAC field"}, {idCapture, "&Capture field"}, {idPaste, "&Paste draft"}, {idCheck, "Check &spelling"}, {idApply, "&Apply to AAC"},
		{idUndo, "&Undo AAC fix"}, {idCopy, "C&opy correction"}, {idClear, "C&lear"},
	} {
		widget(h, b.id, "BUTTON", b.label, 0x10000, 0, font)
	}
	widget(h, idInput, "EDIT", "", 0x211044, 0x200, font)
	widget(h, idOutput, "EDIT", "", 0x211044, 0x200, font)
	for _, id := range []int{idInput, idOutput} {
		send.Call(controls[id], 0xC5, maxDraft, 0)
	}
	if p, cloud := providerFor(profile.Engine); cloud {
		status(p.name + " is selected. Using Spell Check sends the current draft to this provider.")
	}
	syncControls()
}
func layout(h uintptr) {
	var rc rect
	clientRect.Call(h, uintptr(unsafe.Pointer(&rc)))
	w, v := int(rc.r), int(rc.b)
	if w < 250 || v < 250 {
		return
	}
	box := func(id, x, y, wi, he int) {
		move.Call(controls[id], uintptr(x), uintptr(y), uintptr(wi), uintptr(he), 1)
	}
	row := func(y int, ids ...int) {
		width := (w - 32 - 8*(len(ids)-1)) / len(ids)
		for i, id := range ids {
			box(id, 16+i*(width+8), y, width, 42)
		}
	}
	box(idHeader, 16, 12, w-32, 31)
	box(idHelp, 16, 48, w-32, 43)
	box(idEngineLabel, 16, 97, 130, 23)
	box(idEngine, 152, 94, w-168, 270)
	_, cloud := providerFor(selectedEngine().id)
	extra := 0
	if cloud {
		extra = 49
		box(idCloudKeyLabel, 16, 142, 90, 24)
		box(idCloudKey, 110, 132, w-376, 38)
		box(idCloudSave, w-256, 132, 112, 38)
		box(idCloudRemove, w-136, 132, 120, 38)
	}
	row(135+extra, idLink, idCapture, idPaste, idCheck)
	panel := max(42, (v-356-extra)/2)
	box(idOriginal, 16, 186+extra, w-32, 23)
	box(idInput, 16, 211+extra, w-32, panel)
	py := 217 + extra + panel
	box(idPreview, 16, py, w-32, 23)
	box(idOutput, 16, py+25, w-32, panel)
	row(v-100, idApply, idUndo, idCopy, idClear)
	box(idStatus, 16, v-50, w-32, 48)
}
func wndProc(h uintptr, m uint32, w, l uintptr) uintptr {
	if taskbarCreated != 0 && m == taskbarCreated {
		addTray()
		return 0
	}
	switch m {
	case wmOpenSettings:
		activateApp()
		return 0
	case wmStartAuto, wmStartUndo:
		if m == wmStartAuto {
			startAutomatic(idAuto, w)
		} else if w != 0 && activeRoot() == w {
			startAutomatic(idAutoUndo, w)
		}
		return 0
	case wmTrayIcon:
		onTrayEvent(l)
		return 0
	case 1:
		initControls(h)
		return 0
	case 0x0010: // WM_CLOSE
		if trayAdded || addTray() {
			show.Call(h, 0)
		} else {
			requestExit()
		}
		return 0
	case 2:
		removeTray()
		stopModels()
		if hotkeyOK {
			unregisterHotkey.Call(h, 1)
		}
		if font != 0 {
			deleteGDI.Call(font)
		}
		if larger != 0 {
			deleteGDI.Call(larger)
		}
		quit.Call(0)
		return 0
	case 5:
		if controls[idHeader] != 0 {
			layout(h)
		}
		return 0
	case 0x24:
		min := point{650, 530}
		// WM_GETMINMAXINFO supplies a native struct of five POINTs.
		moveMemory.Call(l+3*unsafe.Sizeof(min), uintptr(unsafe.Pointer(&min)), unsafe.Sizeof(min))
		return 0
	case 0x0312:
		if w == 1 && !busy && !holding {
			start(task{kind: idCapture})
			return 0
		}
	case 0x0113:
		if w == 1 && holding {
			killTimer.Call(h, 1)
			holding = false
			var p point
			ok, _, _ := cursor.Call(uintptr(unsafe.Pointer(&p)))
			if ok != 0 {
				start(task{kind: pendingCapture, at: &p, profile: profile})
			} else {
				status("Could not get the pointer location.")
				syncControls()
			}
			return 0
		}
	case wmModelProgress:
		select {
		case message := <-statusUpdates:
			if busy {
				status(message)
				if autoInProgress {
					trayUpdate("AAC Spellchecker — " + message)
				}
			}
		default:
		}
		return 0
	case wmDone:
		select {
		case r := <-results:
			finish(r)
		default:
		}
		return 0
	case 0x0111:
		id, n := int(w&0xffff), uint32(w>>16)
		if id == idEngine && n == 1 && !busy && !holding { // CBN_SELCHANGE
			next := profile
			next.Engine = selectedEngine().id
			if err := saveProfile(next); err != nil {
				// Preserve the old provider and its input together. A key typed
				// for one service must never be saved against another service.
				send.Call(controls[idEngine], 0x14e, uintptr(engineIndex(profile.Engine)), 0)
				status("The engine choice could not be changed because Windows could not save it.")
				return 0
			}
			profile = next
			setBox(idOutput, "")
			setBox(idCloudKey, "")
			previewValid = false
			if p, cloud := providerFor(next.Engine); cloud {
				status(p.name + " needs a key and internet. Spell Check sends the current draft to this provider.")
			} else if next.Engine == "windows" {
				status("Windows Spell Checking selected. No download needed.")
			} else {
				status("The selected model runs locally. On the first check, the app downloads missing files.")
			}
			layout(h)
			syncControls()
			return 0
		}
		if n == 0x0300 && (id == idInput || id == idOutput) && !updating && !busy && !holding {
			if id == idInput && !attached {
				setBox(idOutput, "")
				previewValid = false
				status("Draft changed. Check spelling to refresh the preview.")
			}
			syncControls()
		}
		if l != 0 && n == 0 && !busy && !holding && (id >= idCapture && id <= idClear || id == idLink || id == idCloudSave || id == idCloudRemove) {
			action(h, id)
			return 0
		}
	}
	r, _, _ := defProc.Call(h, uintptr(m), w, l)
	return r
}

func setBox(id int, s string) {
	if id == idInput || id == idOutput {
		s = matchLineEndings(s, "\r\n")
	}
	was := updating
	updating = true
	setText.Call(controls[id], uintptr(unsafe.Pointer(wide(s))))
	updating = was
}
func status(s string) { setBox(idStatus, s) }
func textBox(id int) string {
	h := controls[id]
	n, _, _ := getTextLen.Call(h)
	if n == 0 || n > maxDraft+10000 {
		return ""
	}
	buf := make([]uint16, n+1)
	getText.Call(h, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}
func activateApp() { show.Call(hwnd, 9); activate.Call(hwnd) }
func selectedEngine() engine {
	value, _, _ := send.Call(controls[idEngine], 0x147, 0, 0) // CB_GETCURSEL
	return engineAt(int(value))
}

var autoInProgress bool

func startAutomatic(kind int, source uintptr) {
	if busy || holding || exiting {
		trayUpdate("AAC Spellchecker — already working")
		return
	}
	if kind == idAuto && (source == 0 || source == hwnd || source != activeRoot()) {
		message := "The AAC window is no longer active. No draft was sent or changed."
		status(message)
		trayUpdate("AAC Spellchecker — " + message)
		return
	}
	if kind == idAutoUndo && !autoUndoReady {
		message := "There is no automatic correction to undo."
		status(message)
		trayUpdate("AAC Spellchecker — " + message)
		return
	}
	start(task{kind: kind, source: source, profile: profile})
}

func start(t task) {
	t.engine = selectedEngine()
	autoInProgress = t.kind == idAuto || t.kind == idAutoUndo
	if autoInProgress {
		t.engine = getEngine(t.profile.Engine)
	}
	busy = true
	if t.approved {
		setBox(idOutput, "")
		previewValid = false
		if p, cloud := providerFor(t.engine.id); cloud {
			status("Waiting for " + p.name + "…")
		}
	} else {
		status("Working…")
	}
	if autoInProgress {
		trayUpdate("AAC Spellchecker — checking draft")
	}
	syncControls()
	jobs <- t
}
func finish(r result) {
	busy = false
	if exiting {
		destroyWindow.Call(hwnd)
		return
	}
	autoInProgress = false
	autoUndoReady = r.autoUndo
	if r.kind == idAuto || r.kind == idAutoUndo {
		message := r.status
		if r.err != nil {
			message = r.err.Error()
		}
		status(message)
		trayUpdate("AAC Spellchecker — " + message)
		syncControls()
		return
	}
	if r.kind == idLink {
		attached, undo = r.attached, r.undo
		setBox(idOutput, "")
		previewValid = false
		if r.err == nil {
			r.err = saveProfile(r.profile)
			if r.err == nil {
				profile = r.profile
			}
		}
		if r.err != nil {
			status(r.err.Error())
		} else {
			status(r.status)
		}
		activateApp()
		syncControls()
		return
	}
	attached, undo = r.attached, r.undo
	if r.kind == idCapture || r.kind == idPaste || r.kind == idClear {
		setBox(idInput, r.original)
		setBox(idOutput, r.preview)
		previewValid = r.checked
		if r.kind == idCapture {
			activateApp()
		}
	}
	if r.err == nil && (r.kind == idCheck || r.kind == idApply || r.kind == idUndo) {
		if r.kind != idCheck {
			setBox(idInput, r.original)
		}
		setBox(idOutput, r.preview)
		previewValid = r.checked
	}
	if r.err != nil {
		status(r.err.Error())
	} else {
		status(r.status)
	}
	syncControls()
	if r.kind == idClear || r.kind == idPaste {
		setFocus.Call(controls[idInput])
	}
	if r.kind == idCapture || r.kind == idCheck {
		setFocus.Call(controls[idOutput])
	}
}
func syncControls() {
	idle := !busy && !holding
	draft, corrected := textBox(idInput), textBox(idOutput)
	have := strings.TrimSpace(corrected) != ""
	on := func(id int, yes bool) {
		var v uintptr
		if yes {
			v = 1
		}
		enable.Call(controls[id], v)
	}
	for _, id := range []int{idLink, idCapture, idPaste, idClear} {
		on(id, idle)
	}
	on(idEngine, idle)
	_, cloud := providerFor(selectedEngine().id)
	for _, id := range []int{idCloudKeyLabel, idCloudKey, idCloudSave, idCloudRemove} {
		show.Call(controls[id], windowVisibility(cloud))
		on(id, cloud && idle)
	}
	on(idCheck, idle && strings.TrimSpace(draft) != "")
	on(idApply, idle && attached && have && corrected != draft)
	on(idUndo, idle && ((attached && undo) || autoUndoReady))
	on(idCopy, idle && have)
	readonly := func(id int, b bool) {
		var v uintptr
		if b {
			v = 1
		}
		send.Call(controls[id], 0xCF, v, 0)
	}
	readonly(idInput, attached || !idle)
	readonly(idOutput, !idle || (!previewValid && strings.TrimSpace(draft) == ""))
}

func windowVisibility(visible bool) uintptr {
	if visible {
		return 8
	} // SW_SHOWNA
	return 0
}

func action(h uintptr, id int) {
	switch id {
	case idCapture, idLink:
		pendingCapture = id
		holding = true
		delay := uintptr(5000)
		if id == idLink {
			delay = 12000
		}
		if id == idLink {
			msgBox.Call(h, uintptr(unsafe.Pointer(wide("After choosing OK, point at the AAC Message Window and leave the pointer there. This one-time setup takes 12 seconds."))), uintptr(unsafe.Pointer(wide(title))), 0x40)
		}
		timer, _, _ := setTimer.Call(h, 1, delay, 0)
		if timer == 0 {
			holding = false
			status("The field setup timer could not start.")
			syncControls()
			return
		}
		status("Point at the AAC Message Window. The app will minimize.")
		syncControls()
		show.Call(h, 6)
	case idPaste:
		s, err := loadClipboard(h)
		if err != nil {
			status(err.Error())
			return
		}
		start(task{kind: idPaste, s: s})
	case idCheck:
		_, cloud := providerFor(selectedEngine().id)
		start(task{kind: idCheck, s: textBox(idInput), approved: cloud})
	case idCloudSave, idCloudRemove:
		p, cloud := providerFor(selectedEngine().id)
		if !cloud {
			return
		}
		if id == idCloudSave {
			if err := saveCloudKey(p, strings.TrimSpace(textBox(idCloudKey))); err != nil {
				status("The API key was not saved. Check the key and your Windows account.")
				return
			}
			status(p.name + " API key saved. No draft was sent.")
		} else {
			if err := removeCloudKey(p); err != nil {
				status("The saved API key could not be removed.")
				return
			}
			status(p.name + " API key removed from this device.")
		}
		setBox(idCloudKey, "")
	case idApply:
		start(task{kind: idApply, s: textBox(idOutput)})
	case idUndo:
		if attached && undo {
			start(task{kind: idUndo})
		} else {
			startAutomatic(idAutoUndo, 0)
		}
	case idCopy:
		if err := saveClipboard(h, textBox(idOutput)); err != nil {
			status(err.Error())
		} else {
			status("Copied. In AAC, select the old draft and paste the correction.")
		}
	case idClear:
		start(task{kind: idClear})
	}
}

func requestExit() {
	if busy {
		exiting = true
		stopModels()
		show.Call(hwnd, 0)
		return
	}
	destroyWindow.Call(hwnd)
}

func loadClipboard(h uintptr) (string, error) {
	good, _, _ := openClip.Call(h)
	if good == 0 {
		return "", fmt.Errorf("The clipboard is busy. Try again.")
	}
	defer closeClip.Call()
	handle, _, _ := getClip.Call(13)
	if handle == 0 {
		return "", fmt.Errorf("No text on the clipboard. Copy the AAC draft first.")
	}
	size, _, _ := globalSize.Call(handle)
	if size < 2 {
		return "", fmt.Errorf("No readable text on the clipboard.")
	}
	ptr, _, _ := globalLock.Call(handle)
	if ptr == 0 {
		return "", fmt.Errorf("The clipboard text could not be read.")
	}
	defer globalUnlock.Call(handle)
	units := make([]uint16, min(size/2, maxDraft+1))
	moveMemory.Call(uintptr(unsafe.Pointer(&units[0])), ptr, uintptr(len(units)*2))
	end := -1
	for i, u := range units {
		if u == 0 {
			end = i
			break
		}
	}
	if end == -1 {
		return "", fmt.Errorf("Clipboard draft exceeds 10,000 characters or is invalid.")
	}
	s := syscall.UTF16ToString(units[:end])
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("Clipboard has no draft text.")
	}
	return s, nil
}

func saveClipboard(h uintptr, s string) error {
	buf, err := syscall.UTF16FromString(s)
	if err != nil || strings.TrimSpace(s) == "" {
		return fmt.Errorf("There is no correction to copy.")
	}
	bytes := uintptr(len(buf) * 2)
	handle, _, _ := globalAlloc.Call(2, bytes)
	if handle == 0 {
		return fmt.Errorf("Windows could not allocate clipboard text.")
	}
	owned := true
	defer func() {
		if owned {
			globalFree.Call(handle)
		}
	}()
	ptr, _, _ := globalLock.Call(handle)
	if ptr == 0 {
		return fmt.Errorf("Windows could not prepare clipboard text.")
	}
	moveMemory.Call(ptr, uintptr(unsafe.Pointer(&buf[0])), bytes)
	globalUnlock.Call(handle)
	good, _, _ := openClip.Call(h)
	if good == 0 {
		return fmt.Errorf("The clipboard is busy. Try again.")
	}
	defer closeClip.Call()
	good, _, _ = emptyClip.Call()
	if good == 0 {
		return fmt.Errorf("Windows could not open the clipboard for copying.")
	}
	copied, _, _ := setClip.Call(13, handle)
	if copied == 0 {
		return fmt.Errorf("Windows could not copy the correction.")
	}
	owned = false
	return nil
}
