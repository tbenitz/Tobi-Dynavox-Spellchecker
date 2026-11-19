package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFirstUseModelSetupAndOfflineReuse(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "true" && os.Getenv("SPELLCHECK_MODEL_SMOKE") != "1" {
		t.Skip("set SPELLCHECK_MODEL_SMOKE=1 to exercise an actual fresh model download")
	}
	t.Setenv("LOCALAPPDATA", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	for i := 0; i < 2; i++ {
		var stages []string
		if i == 1 {
			t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
			t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
			t.Setenv("ALL_PROXY", "http://127.0.0.1:1")
			t.Setenv("NO_PROXY", "")
		}
		got, changes, err := spellModel(ctx, engineAt(6), "helllo", func(s string) {
			stages = append(stages, s)
			t.Log(s)
		})
		if err != nil || got != "hello" || changes != 1 {
			t.Fatalf("check %d returned %q, %d, %v", i+1, got, changes, err)
		}
		didDownload := strings.Contains(strings.Join(stages, "\n"), "Downloading selected model")
		if didDownload != (i == 0) {
			t.Fatalf("check %d unexpectedly reported download=%v", i+1, didDownload)
		}
	}
}
