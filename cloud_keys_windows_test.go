package main

import (
	"bytes"
	"os"
	"testing"
)

func TestWindowsEncryptedProviderKeys(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	deepseek, _ := providerFor("deepseek")
	grok, _ := providerFor("grok")
	for _, p := range []cloudProvider{deepseek, grok} {
		key := "sk-fixture-" + p.id
		if _, err := readCloudKey(p); !os.IsNotExist(err) {
			t.Fatalf("missing key err=%v", err)
		}
		if err := saveCloudKey(p, key); err != nil {
			t.Fatal(err)
		}
		path, _ := cloudKeyPath(p)
		disk, err := os.ReadFile(path)
		if err != nil || bytes.Contains(disk, []byte(key)) {
			t.Fatal("key not encrypted")
		}
		if got, err := readCloudKey(p); err != nil || got != key {
			t.Fatalf("could not read key: %v", err)
		}
	}
	if err := removeCloudKey(deepseek); err != nil {
		t.Fatal(err)
	}
	if _, err := readCloudKey(deepseek); !os.IsNotExist(err) {
		t.Fatalf("key wasn't removed: %v", err)
	}
	if got, err := readCloudKey(grok); err != nil || got != "sk-fixture-grok" {
		t.Fatal("deletion affected the other provider")
	}
}
