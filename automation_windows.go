package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	ole32     = syscall.NewLazyDLL("ole32.dll")
	oleaut32  = syscall.NewLazyDLL("oleaut32.dll")
	coInit    = ole32.NewProc("CoInitializeEx")
	coUninit  = ole32.NewProc("CoUninitialize")
	coCreate  = ole32.NewProc("CoCreateInstance")
	coFree    = ole32.NewProc("CoTaskMemFree")
	allocBSTR = oleaut32.NewProc("SysAllocStringLen")
	freeBSTR  = oleaut32.NewProc("SysFreeString")
	lenBSTR   = oleaut32.NewProc("SysStringLen")
)

type com struct{ vtable *[128]uintptr }

type guid struct {
	a    uint32
	b, c uint16
	d    [8]byte
}

var (
	uiaClass   = guid{0xff48dba4, 0x60ef, 0x4201, [8]byte{0xaa, 0x87, 0x54, 0x10, 0x3e, 0xef, 0x59, 0x4e}}
	uiaIID     = guid{0x30cbe57d, 0xd9d0, 0x452a, [8]byte{0xab, 0x13, 0x7a, 0xc5, 0xac, 0x48, 0x25, 0xee}}
	valueIID   = guid{0xa94cd8b1, 0x0844, 0x4cd6, [8]byte{0x9d, 0x2d, 0x64, 0x05, 0x37, 0xab, 0x39, 0xe9}}
	spellClass = guid{0x7ab36653, 0x1796, 0x484b, [8]byte{0xbd, 0xfa, 0xe7, 0x4f, 0x1d, 0xb7, 0xc1, 0xdc}}
	spellIID   = guid{0x8e018a9d, 0x2415, 0x4677, [8]byte{0xbf, 0x08, 0x79, 0x4e, 0xa6, 0x1f, 0x94, 0xbb}}
)

//go:uintptrescapes
func (c *com) invoke(slot int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(c.vtable[slot], append([]uintptr{uintptr(unsafe.Pointer(c))}, args...)...)
	return r
}
func (c *com) release() {
	if c != nil {
		c.invoke(2)
	}
}
func ok(hr uintptr) bool { return int32(hr) >= 0 }

func makeCOM(clsid, iid *guid) (*com, error) {
	var c *com
	r, _, _ := coCreate.Call(uintptr(unsafe.Pointer(clsid)), 0, 1, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&c)))
	if !ok(r) || c == nil {
		return nil, fmt.Errorf("Windows service unavailable (0x%08X).", uint32(r))
	}
	return c, nil
}
func wide(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }
func utf16z(p *uint16, limit int) string {
	if p == nil || limit <= 0 {
		return ""
	}
	// Callers pass an inspected allocation size or the provider's NUL-terminated string.
	n := 0
	for n < limit && *(*uint16)(unsafe.Add(unsafe.Pointer(p), 2*n)) != 0 {
		n++
	}
	return syscall.UTF16ToString(unsafe.Slice(p, n))
}

type captured struct{ el *com }

func (f *captured) pattern() (*com, error) {
	var control, pid, password, enabled uint32
	e := f.el
	if !ok(e.invoke(20, uintptr(unsafe.Pointer(&pid)))) || pid == 0 || pid == uint32(os.Getpid()) ||
		!ok(e.invoke(21, uintptr(unsafe.Pointer(&control)))) || (control != 50004 && control != 50030) ||
		!ok(e.invoke(35, uintptr(unsafe.Pointer(&password)))) || password != 0 ||
		!ok(e.invoke(28, uintptr(unsafe.Pointer(&enabled)))) || enabled == 0 {
		return nil, errors.New("This is not an available editable text field. Try the AAC message field or use clipboard mode.")
	}
	var v *com
	if !ok(e.invoke(14, 10002, uintptr(unsafe.Pointer(&valueIID)), uintptr(unsafe.Pointer(&v)))) || v == nil {
		return nil, errors.New("This field does not expose text editing to Windows. Copy the AAC draft and use Paste draft.")
	}
	var ro uint32
	if !ok(v.invoke(5, uintptr(unsafe.Pointer(&ro)))) || ro != 0 {
		v.release()
		return nil, errors.New("The captured field is read-only. Use Copy / Paste in the AAC app.")
	}
	return v, nil
}
func (f *captured) read() (string, error) {
	v, err := f.pattern()
	if err != nil {
		return "", err
	}
	defer v.release()
	var b *uint16
	if !ok(v.invoke(4, uintptr(unsafe.Pointer(&b)))) {
		return "", errors.New("Windows could not read the field.")
	}
	if b == nil {
		return "", nil
	}
	defer freeBSTR.Call(uintptr(unsafe.Pointer(b)))
	n, _, _ := lenBSTR.Call(uintptr(unsafe.Pointer(b)))
	if n > maxDraft {
		return "", errors.New("The draft is too long. Check up to 10,000 characters at a time.")
	}
	s := string(utf16.Decode(unsafe.Slice(b, int(n))))
	if strings.ContainsRune(s, 0) {
		return "", errors.New("This field contains unsupported text.")
	}
	return s, nil
}
func (f *captured) set(s string) error {
	v, err := f.pattern()
	if err != nil {
		return err
	}
	defer v.release()
	units := utf16.Encode([]rune(s))
	b, _, _ := allocBSTR.Call(uintptr(unsafe.Pointer(unsafe.SliceData(units))), uintptr(len(units)))
	if b == 0 {
		return errors.New("Windows could not allocate the replacement.")
	}
	defer freeBSTR.Call(b)
	if hr := v.invoke(3, b); !ok(hr) {
		return fmt.Errorf("Windows returned 0x%08X.", uint32(hr))
	}
	return nil
}

func getField(a *com, at *point) (*captured, string, error) {
	var el *com
	var hr uintptr
	if at == nil {
		hr = a.invoke(8, uintptr(unsafe.Pointer(&el)))
	} else {
		packed := uintptr(uint64(uint32(at.x)) | uint64(uint32(at.y))<<32)
		hr = a.invoke(7, packed, uintptr(unsafe.Pointer(&el)))
	}
	if !ok(hr) || el == nil {
		return nil, "", errors.New("Could not identify the AAC message field. Try Paste draft.")
	}
	// A hit inside a rich text field can return a text leaf. Try up to four
	// parents, keeping the check restricted to editable Edit/Document controls.
	var walker *com
	if at != nil && ok(a.invoke(16, uintptr(unsafe.Pointer(&walker)))) && walker != nil {
		defer walker.release()
	}
	var last error
	for i := 0; i < 5 && el != nil; i++ {
		f := &captured{el}
		var s string
		s, last = f.read()
		if last == nil {
			return f, s, nil
		}
		var parent *com
		if walker != nil && i < 4 {
			walker.invoke(3, uintptr(unsafe.Pointer(el)), uintptr(unsafe.Pointer(&parent)))
		}
		el.release()
		el = parent
	}
	return nil, "", last
}

func spellChecker(language string) (*com, error) {
	f, err := makeCOM(&spellClass, &spellIID)
	if err != nil {
		return nil, err
	}
	defer f.release()
	var c *com
	if !ok(f.invoke(5, uintptr(unsafe.Pointer(wide(language))), uintptr(unsafe.Pointer(&c)))) || c == nil {
		return nil, fmt.Errorf("The %s Windows spelling dictionary is unavailable. Check Windows Language settings.", language)
	}
	return c, nil
}

func propose(c *com, s string) (string, int, error) {
	units := utf16.Encode([]rune(s))
	if strings.TrimSpace(s) == "" || len(units) > maxDraft || strings.ContainsRune(s, 0) {
		return "", 0, errors.New("Enter a draft of up to 10,000 characters.")
	}
	var list *com
	if !ok(c.invoke(4, uintptr(unsafe.Pointer(wide(s))), uintptr(unsafe.Pointer(&list)))) || list == nil {
		return "", 0, errors.New("Windows could not spell-check the draft.")
	}
	defer list.release()
	var edits []edit
	for i := 0; i < len(units); i++ {
		var item *com
		hr := list.invoke(3, uintptr(unsafe.Pointer(&item)))
		if !ok(hr) {
			return "", 0, errors.New("Windows could not finish checking the draft.")
		}
		if hr != 0 || item == nil {
			break
		}
		if e, yes := suggestion(c, item, units); yes {
			edits = append(edits, e)
		}
		item.release()
	}
	result, n := applyEdits(s, edits)
	return result, n, nil
}

func suggestion(c, item *com, units []uint16) (edit, bool) {
	var start, length, action uint32
	if !ok(item.invoke(3, uintptr(unsafe.Pointer(&start)))) ||
		!ok(item.invoke(4, uintptr(unsafe.Pointer(&length)))) ||
		!ok(item.invoke(5, uintptr(unsafe.Pointer(&action)))) ||
		length == 0 || uint64(start)+uint64(length) > uint64(len(units)) {
		return edit{}, false
	}
	word := string(utf16.Decode(units[int(start):int(start+length)]))
	// No deletion suggestions. Preserve all-caps abbreviations.
	if action != 1 && action != 2 || (strings.ToUpper(word) == word && strings.ToLower(word) != word && len([]rune(word)) > 1) {
		return edit{}, false
	}
	var p *uint16
	if action == 2 {
		if !ok(item.invoke(6, uintptr(unsafe.Pointer(&p)))) || p == nil {
			return edit{}, false
		}
	} else {
		var opts *com
		if !ok(c.invoke(5, uintptr(unsafe.Pointer(wide(word))), uintptr(unsafe.Pointer(&opts)))) || opts == nil {
			return edit{}, false
		}
		defer opts.release()
		var fetched uint32
		if !ok(opts.invoke(3, 1, uintptr(unsafe.Pointer(&p)), uintptr(unsafe.Pointer(&fetched)))) || fetched != 1 || p == nil {
			return edit{}, false
		}
	}
	defer coFree.Call(uintptr(unsafe.Pointer(p)))
	fix := utf16z(p, maxDraft+1)
	if strings.TrimSpace(fix) == "" || len(fix) > maxDraft || strings.ContainsAny(fix, "\r\n") {
		return edit{}, false
	}
	return edit{int(start), int(length), fix}, true
}
