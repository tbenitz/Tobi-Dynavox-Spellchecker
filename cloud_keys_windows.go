package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	crypt32        = syscall.NewLazyDLL("crypt32.dll")
	cryptProtect   = crypt32.NewProc("CryptProtectData")
	cryptUnprotect = crypt32.NewProc("CryptUnprotectData")
	localFree      = kernel.NewProc("LocalFree")
)

type dataBlob struct {
	size uint32
	data *byte
}

func cryptKey(input []byte, protect bool) ([]byte, error) {
	if len(input) == 0 || len(input) > 8192 {
		return nil, errors.New("Invalid API key data.")
	}
	src, dst := dataBlob{uint32(len(input)), unsafe.SliceData(input)}, dataBlob{}
	proc := cryptUnprotect
	if protect {
		proc = cryptProtect
	}
	// CRYPTPROTECT_UI_FORBIDDEN. The default scope is the current Windows user.
	ok, _, _ := proc.Call(uintptr(unsafe.Pointer(&src)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&dst)))
	runtime.KeepAlive(input)
	if dst.data != nil {
		defer localFree.Call(uintptr(unsafe.Pointer(dst.data)))
	}
	if ok == 0 || dst.size == 0 || dst.size > 8192 || dst.data == nil {
		return nil, errors.New("Windows could not protect or read the saved API key.")
	}
	raw := unsafe.Slice(dst.data, int(dst.size))
	out := append([]byte(nil), raw...)
	clear(raw)
	return out, nil
}

func cloudKeyPath(p cloudProvider) (string, error) {
	if _, ok := providerFor(p.id); !ok {
		return "", errors.New("Unknown spelling provider.")
	}
	profileFile, err := profilePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(profileFile), "credentials", p.id+".dpapi"), nil
}

func readCloudKey(p cloudProvider) (string, error) {
	path, err := cloudKeyPath(p)
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil {
		return "", err
	}
	raw, err := cryptKey(data, false)
	if err != nil {
		return "", err
	}
	defer clear(raw)
	key := string(raw)
	if !validCloudKey(key) {
		return "", errors.New("The saved API key is invalid.")
	}
	return key, nil
}

func saveCloudKey(p cloudProvider, key string) error {
	if !validCloudKey(key) {
		return errors.New("Enter a valid API key without internal spaces.")
	}
	path, err := cloudKeyPath(p)
	if err != nil {
		return err
	}
	raw := []byte(key)
	defer clear(raw)
	data, err := cryptKey(raw, true)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "key-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

func removeCloudKey(p cloudProvider) error {
	path, err := cloudKeyPath(p)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
