package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineDiagnosisDoesNotAuthenticateOrExposeCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"bots":[{"id":"test","app_id":"fake-app-id","app_secret":"private-secret","mode":"webhook"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := run([]string{"--config", path, "--bot", "test"}, &out); code != 0 {
		t.Fatal(code, out.String())
	}
	if !strings.Contains(out.String(), "no QQ requests made") || strings.Contains(out.String(), "private-secret") || strings.Contains(out.String(), "fake-app-id") {
		t.Fatal(out.String())
	}
	for _, tc := range []struct {
		args []string
		code int
	}{{nil, 2}, {[]string{"--config", path, "--bot", "missing"}, 4}, {[]string{"--config", path + "missing", "--bot", "test"}, 3}} {
		if code := run(tc.args, &out); code != tc.code {
			t.Fatal(code, tc.code)
		}
	}
}
