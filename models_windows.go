package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

//go:embed model_support/*.py model_support/*LICENSE.txt
var modelSupport embed.FS

const uvURL = "https://github.com/astral-sh/uv/releases/download/0.8.22/uv-x86_64-pc-windows-msvc.zip"
const uvDigest = "5049375aa2a5162f132b2c1cb992e25d42d47d934cab8c174dbe6f60973dcc12"

func spellModel(ctx context.Context, e engine, draft string, progress func(string)) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	defer cancel()
	root, err := os.UserCacheDir()
	if err != nil {
		return "", 0, errors.New("Windows did not provide a local model cache directory.")
	}
	root = filepath.Join(root, "AAC Spellchecker")
	runtimeDir := filepath.Join(root, "runtime-v1")
	if err = os.MkdirAll(runtimeDir, 0700); err != nil {
		return "", 0, fmt.Errorf("Cannot create the local model cache: %w", err)
	}
	python, err := prepareRuntime(ctx, runtimeDir, progress)
	if err != nil {
		return "", 0, err
	}
	script, err := installModelSupport(root)
	if err != nil {
		return "", 0, fmt.Errorf("Cannot prepare the embedded model helper: %w", err)
	}

	request, _ := json.Marshal(map[string]string{"text": draft})
	// -I removes the script directory from sys.path. Run it intentionally via a
	// fixed local launcher so the helper can import its embedded tiny-model code.
	cmd := hiddenCommand(ctx, python, "-I", "-u", "-c", "import runpy,sys; from pathlib import Path; p=sys.argv.pop(1); sys.path.insert(0,str(Path(p).parent)); runpy.run_path(p,run_name='__main__')", script, e.id, filepath.Join(root, "models"))
	cmd.Stdin = bytes.NewReader(request)
	cmd.Env = localRuntimeEnv(map[string]string{
		"HF_HOME":                       filepath.Join(root, "hub"),
		"HF_HUB_DISABLE_IMPLICIT_TOKEN": "1",
		"HF_HUB_DISABLE_TELEMETRY":      "1",
		"HF_HUB_DISABLE_PROGRESS_BARS":  "1",
		"HF_HUB_ETAG_TIMEOUT":           "25",
		"HF_HUB_DOWNLOAD_TIMEOUT":       "120",
	})
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", 0, errors.New("Could not open the model worker's output.")
	}
	if err = cmd.Start(); err != nil {
		return "", 0, fmt.Errorf("Could not start the local model worker: %w", err)
	}
	var answer, failure string
	lines := bufio.NewScanner(stdout)
	lines.Buffer(make([]byte, 8192), 1024*1024)
	for lines.Scan() {
		var m struct{ Progress, Text, Error string }
		if json.Unmarshal(lines.Bytes(), &m) != nil {
			continue
		}
		switch {
		case m.Progress != "":
			progress(m.Progress)
		case m.Error != "":
			failure = m.Error
		default:
			answer = m.Text
		}
	}
	if lines.Err() != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	if failure != "" {
		return "", 0, errors.New(failure)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", 0, errors.New("The model did not finish in time. Try again to reuse completed downloads.")
	}
	if waitErr != nil || lines.Err() != nil {
		return "", 0, errors.New("The model worker stopped before returning a preview. Your draft was not changed.")
	}
	return modelPreview(draft, answer)
}

func hiddenCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd
}

func localRuntimeEnv(overrides map[string]string) []string {
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		u := strings.ToUpper(key)
		if strings.HasPrefix(u, "UV_") || strings.HasPrefix(u, "PIP_") || strings.HasPrefix(u, "HF_") ||
			strings.HasPrefix(u, "PYTHON") || u == "TRANSFORMERS_CACHE" || u == "VIRTUAL_ENV" {
			continue
		}
		env = append(env, entry)
	}
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	return env
}

func installModelSupport(root string) (string, error) {
	files, err := modelSupport.ReadDir("model_support")
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	data := make(map[string][]byte)
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		b, err := modelSupport.ReadFile("model_support/" + file.Name())
		if err != nil {
			return "", err
		}
		hash.Write([]byte(file.Name()))
		hash.Write(b)
		data[file.Name()] = b
	}
	dir := filepath.Join(root, fmt.Sprintf("helper-%x", hash.Sum(nil)[:12]))
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	for name, b := range data {
		file := filepath.Join(dir, name)
		if current, e := os.ReadFile(file); e == nil && bytes.Equal(b, current) {
			continue
		}
		if err = os.WriteFile(file, b, 0600); err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, "runner.py"), nil
}

func exists(file string) bool {
	s, err := os.Stat(file)
	return err == nil && s.Mode().IsRegular()
}

func prepareRuntime(ctx context.Context, dir string, progress func(string)) (string, error) {
	python := filepath.Join(dir, "venv", "Scripts", "python.exe")
	ready := filepath.Join(dir, ".ready")
	if exists(python) && exists(ready) {
		return python, nil
	}
	uv := filepath.Join(dir, "uv.exe")
	if !exists(uv) {
		progress("Downloading the local model runtime. First-time setup needs internet.")
		if err := downloadUV(ctx, dir, uv); err != nil {
			return "", fmt.Errorf("Could not download or verify the local runtime: %w", err)
		}
	}
	env := localRuntimeEnv(map[string]string{
		"UV_PYTHON_INSTALL_DIR": filepath.Join(dir, "python"),
		"UV_CACHE_DIR":          filepath.Join(dir, "downloads"),
		"UV_HTTP_TIMEOUT":       "120",
		"UV_NO_CONFIG":          "1",
	})
	run := func(args ...string) error {
		cmd := hiddenCommand(ctx, uv, args...)
		cmd.Env = env
		cmd.Dir = dir
		return cmd.Run()
	}
	if !exists(python) {
		progress("Setting up a private Python runtime. No system installation is required.")
		if err := run("venv", "--python", "3.12.11", "--managed-python", filepath.Join(dir, "venv")); err != nil {
			return "", fmt.Errorf("Could not set up private Python. Check the internet connection and disk space (%w).", err)
		}
	}
	progress("Installing CPU inference libraries. This is a one-time setup.")
	if err := run("pip", "install", "--python", python, "--index-url", "https://download.pytorch.org/whl/cpu", "torch==2.7.1"); err != nil {
		return "", fmt.Errorf("Could not install the CPU inference library. Check internet and disk space (%w).", err)
	}
	if err := run("pip", "install", "--python", python, "--index-url", "https://pypi.org/simple", "--index", "https://download.pytorch.org/whl/cpu", "torch==2.7.1+cpu",
		"transformers==4.53.3", "sentencepiece==0.2.0", "accelerate==1.8.1", "protobuf==6.31.1",
		"huggingface-hub==0.36.2", "safetensors==0.6.2", "tqdm==4.67.1"); err != nil {
		return "", fmt.Errorf("Could not install the spelling model libraries. Check internet and disk space (%w).", err)
	}
	probe := hiddenCommand(ctx, python, "-I", "-c", "import torch, transformers, sentencepiece, accelerate, safetensors")
	probe.Env, probe.Dir = env, dir
	if err := probe.Run(); err != nil {
		return "", errors.New("The local inference libraries could not start. Use a 64-bit Windows PC or select Windows Spell Checking.")
	}
	if err := os.WriteFile(ready, []byte("1\n"), 0600); err != nil {
		return "", err
	}
	return python, nil
}

func downloadUV(ctx context.Context, dir, target string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", uvURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 24<<20))
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != uvDigest {
		return errors.New("runtime archive checksum did not match")
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range z.File {
		if path.Base(f.Name) != "uv.exe" || !f.Mode().IsRegular() || f.UncompressedSize64 > 80<<20 {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		defer r.Close()
		tmp, err := os.CreateTemp(dir, "uv-*.partial")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		_, copyErr := io.Copy(tmp, r)
		closeErr := tmp.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if exists(target) {
			return nil
		}
		return os.Rename(tmp.Name(), target)
	}
	return errors.New("runtime archive was missing uv.exe")
}
