package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyDefaultsAndNewSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"webui":{"enabled":true,"port":4545,"password":"private-secret"},"bots":[{"id":"a","app_secret":"bot-secret"}],"builtin_commands":{"help":"custom help","status":"custom status","shizuku":"custom info"}}`), 0600)
	c, e := LoadConfig(path)
	if e != nil {
		t.Fatal(e)
	}
	if c.Statistics.RetentionDays != 90 || c.WebUI.Appearance.Theme.Mode != "auto" || c.BuiltinCommands.Help != "custom help" {
		t.Fatal(c)
	}
	c.WebUI.Appearance = Appearance{Background: Background{Type: "url", Value: "https://example.com/a.png"}, Theme: Theme{Mode: "dark", PrimaryColor: "#123456"}}
	c.Statistics.RetentionDays = 0
	if e = SaveConfig(path, c); e != nil {
		t.Fatal(e)
	}
	loaded, e := LoadConfig(path)
	if e != nil || loaded.Bots[0].AppSecret != "bot-secret" || loaded.Statistics.RetentionDays != 0 {
		t.Fatal(loaded, e)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "#123456") {
		t.Fatal("appearance lost")
	}
}
