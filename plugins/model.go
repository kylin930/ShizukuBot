// Package plugins defines versioned code plugins independently of any editor or QQ SDK.
package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"shizukubot/events"
	"strings"
)

const APIVersion = 1

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$`)

func ValidID(id string) bool { return idPattern.MatchString(id) }

// Manifest's api_version is a major compatibility contract, separate from plugin semver.
type Manifest struct {
	APIVersion   int      `json:"api_version"`
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description,omitempty"`
	Author       string   `json:"author,omitempty"`
	Entry        string   `json:"entry"`
	Runtime      string   `json:"runtime"`
	Permissions  []string `json:"permissions"`
	ConfigSchema Schema   `json:"config_schema"`
}

func (m Manifest) Validate() error {
	if m.APIVersion != APIVersion || !ValidID(m.ID) || m.Name == "" || !versionPattern.MatchString(m.Version) {
		return errors.New("invalid manifest identity or API version")
	}
	if m.Runtime != "javascript" && m.Runtime != "go" {
		return errors.New("unsupported runtime")
	}
	if m.Runtime == "javascript" && (!filepath.IsLocal(m.Entry) || strings.Contains(m.Entry, "\\") || !strings.HasSuffix(m.Entry, ".js")) {
		return errors.New("entry must be a local built JavaScript file")
	}
	for _, p := range m.Permissions {
		switch p {
		case "events.subscribe", "commands.register", "message.send", "config.read", "state.read", "state.write", "log.write":
		default:
			return errors.New("unknown permission: " + p)
		}
	}
	return nil
}
func (m Manifest) Allows(p string) bool {
	for _, v := range m.Permissions {
		if v == p {
			return true
		}
	}
	return false
}

// Call is the capability-limited context visible to trusted extensions.
type Call struct {
	Event  events.Event
	Args   []string
	Config map[string]any
	State  State
	Log    func(string)
}

// State is isolated per plugin and bot. Values must be JSON serializable.
type State interface {
	Get(context.Context, string) (json.RawMessage, error)
	Set(context.Context, string, json.RawMessage) error
}

// Host lets Go extensions register callbacks during Load or Start.
type Host interface {
	Command(string, func(context.Context, Call) error) error
	On(events.Type, func(context.Context, Call) error) error
}

// Extension is a statically registered, trusted Go plugin. Methods honor context.
type Extension interface {
	Load(context.Context, Host) error
	Start(context.Context) error
	HandleEvent(context.Context, Call) error
	Stop(context.Context) error
	Unload(context.Context) error
}

// Registration is immutable after startup; names may coexist across plugins.
type Registration struct {
	Commands []string      `json:"commands"`
	Events   []events.Type `json:"events"`
}

// Runtime is implemented by a Go extension instance or an out-of-process worker.
type Runtime interface {
	Start(context.Context) (Registration, error)
	Handle(context.Context, Call) error
	Stop(context.Context) error
}
type Factory func(Manifest, string, map[string]any, State) (Runtime, error)
