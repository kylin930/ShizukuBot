package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Compile the generated catalog against real Simple and Advanced SDK packages.
// A second generation with a colliding ID must fail through normal Registry validation.
func TestGeneratedRegistryCompilesNewSDKAndRejectsDuplicateIDs(t *testing.T) {
	root := t.TempDir()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	mod := "module generatedcheck\n\ngo 1.26\nrequire shizukubot v0.0.0\nreplace shizukubot => " + filepath.ToSlash(repo) + "\n"
	files := map[string]string{
		"go.mod": mod,
		"plugins/catalog.go": `package plugins
import runtime "shizukubot/plugins"
type Registry = runtime.Registry
func NewRegistry()*Registry{return runtime.NewRegistry()}`,

		"plugins/simple/plugin.go": `package simple
import("context";"shizukubot/sdk")
func Definition()sdk.Definition{return sdk.CommandPlugin(sdk.Meta{ID:"simple",Name:"Simple",Version:"1.0.0"},"ping",func(ctx context.Context,c sdk.Call)error{return c.Reply(ctx,"pong")})}`,
		"plugins/advanced/plugin.go": `package advanced
import "shizukubot/sdk"
func Definition()sdk.Definition{return sdk.Definition{Manifest:sdk.Manifest{APIVersion:sdk.APIVersion,ID:"advanced",Name:"Advanced",Version:"1.0.0",Runtime:"go",ConfigSchema:sdk.Schema{Type:"object"}},New:func()sdk.Extension{return &sdk.Base{}}}}`,
		"main.go": `package main
import("generatedcheck/pluginregistry";"os")
func main(){_,err:=pluginregistry.New();if err!=nil{os.Stderr.WriteString(err.Error());os.Exit(1)}}`,
	}
	for path, data := range files {
		target := filepath.Join(root, path)
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(target, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.MkdirAll(filepath.Join(root, "pluginregistry"), 0700); err != nil {
		t.Fatal(err)
	}
	buildAndRun := func() ([]byte, error) {
		t.Helper()
		if err := generate(root, "pluginregistry/generated.go"); err != nil {
			t.Fatal(err)
		}
		build := exec.Command("go", "build", "-mod=mod", "-buildvcs=false", "-o", filepath.Join(root, "check"), ".")
		build.Dir = root
		if raw, err := build.CombinedOutput(); err != nil {
			t.Fatalf("generated SDK registry failed to compile: %v %s", err, raw)
		}
		return exec.Command(filepath.Join(root, "check")).CombinedOutput()
	}
	if raw, err := buildAndRun(); err != nil {
		t.Fatal(err, string(raw))
	}
	duplicate := strings.Replace(files["plugins/advanced/plugin.go"], `ID:"advanced"`, `ID:"simple"`, 1)
	if err = os.WriteFile(filepath.Join(root, "plugins/advanced/plugin.go"), []byte(duplicate), 0600); err != nil {
		t.Fatal(err)
	}
	if raw, err := buildAndRun(); err == nil || !strings.Contains(string(raw), "duplicate plugin ID") {
		t.Fatal("duplicate registry did not fail", err, string(raw))
	}
}
