package sdk

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// APIVersion is the supported major version of the Go plugin contract.
const APIVersion = 1

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$`)

// ValidID reports whether an ID or command name is valid.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// Manifest's api_version is a major compatibility contract, separate from plugin semver.
type Manifest struct {
	APIVersion   int      `json:"api_version"`
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description,omitempty"`
	Author       string   `json:"author,omitempty"`
	Runtime      string   `json:"runtime"`
	Permissions  []string `json:"permissions"`
	ConfigSchema Schema   `json:"config_schema"`
}

// Validate checks identity, compatibility, permissions and configuration schema.
func (m Manifest) Validate() error {
	if m.APIVersion != APIVersion {
		return errors.New("unsupported SDK API version")
	}
	if !ValidID(m.ID) {
		return errors.New("invalid plugin ID")
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("plugin name required")
	}
	if !versionPattern.MatchString(m.Version) {
		return errors.New("explicit major.minor.patch plugin version required")
	}
	if m.Runtime != "go" {
		return errors.New("only compiled Go plugins are supported")
	}
	if m.ConfigSchema.Type != "" && m.ConfigSchema.Type != "object" {
		return errors.New("config schema root must be object")
	}
	if err := m.ConfigSchema.Check(0); err != nil {
		return err
	}
	for _, p := range m.Permissions {
		switch p {
		case "events.subscribe", "commands.register", "message.send", "message.recall",
			"group.manage", "config.read", "state.read", "state.write", "log.write":
		default:
			return errors.New("unknown permission: " + p)
		}
	}
	if _, err := json.Marshal(m); err != nil {
		return errors.New("manifest must contain JSON-compatible values")
	}
	return nil
}

// Allows reports whether the manifest declares a framework capability.
func (m Manifest) Allows(p string) bool {
	for _, v := range m.Permissions {
		if v == p {
			return true
		}
	}
	return false
}

// Meta is the author-facing identity of a plugin. Version must be explicit semver.
type Meta struct {
	ID          string
	Name        string
	Version     string
	Description string
	Author      string
}

// Validate checks metadata using the same identity rules as an advanced Manifest.
func (m Meta) Validate() error { return m.manifest().Validate() }
func (m Meta) manifest() Manifest {
	return Manifest{APIVersion: APIVersion, ID: m.ID, Name: m.Name, Version: m.Version,
		Description: m.Description, Author: m.Author, Runtime: "go", ConfigSchema: Schema{Type: "object"}}
}

// Definition describes a Go plugin. New must return a fresh Extension for each
// Bot and sandbox run; mutable singletons are unsupported.
type Definition struct {
	Manifest Manifest
	New      func() Extension
}

// Validate checks a definition without constructing or executing the plugin.
// Callback registration errors are returned later by Load through the Manager.
func (d Definition) Validate() error {
	if err := d.Manifest.Validate(); err != nil {
		return err
	}
	if d.New == nil {
		return errors.New("plugin constructor required")
	}
	return nil
}
