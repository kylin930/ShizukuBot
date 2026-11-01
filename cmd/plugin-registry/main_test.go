package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratorDeterministicAndNoSourceExecution(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/bot\ngo 1.26\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"z", "a", "testdata/ignored"} {
		dir := filepath.Join(root, "plugins", name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		// An init panic proves generation parses sources without executing them.
		if err := os.WriteFile(filepath.Join(dir, "plugin.go"), []byte("package example\nfunc init(){panic(\"must not run\")}\nfunc Definition()int{return 0}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := generate(root, "generated.go"); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(root, "generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(first)
	if strings.Contains(text, "ignored") || !strings.Contains(text, `p0 "example.test/bot/plugins/a"`) || !strings.Contains(text, `p1 "example.test/bot/plugins/z"`) {
		t.Fatal(text)
	}
	if err = generate(root, "generated.go"); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(root, "generated.go"))
	if err != nil || string(second) != text {
		t.Fatal("non-deterministic", err)
	}
}
