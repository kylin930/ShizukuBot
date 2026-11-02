// Package plugins implements the trusted Go plugin runtime and static registry.
// Plugin implementations should import shizukubot/sdk.
package plugins

import (
	"shizukubot/events"
	"shizukubot/sdk"
)

// APIVersion is retained for source compatibility.
//
// Deprecated: use sdk.APIVersion.
const APIVersion = sdk.APIVersion

// Manifest is retained for source compatibility.
//
// Deprecated: use sdk.Manifest.
type Manifest = sdk.Manifest

// Definition is retained for source compatibility.
//
// Deprecated: use sdk.Definition.
type Definition = sdk.Definition

// Call is retained for source compatibility.
//
// Deprecated: use sdk.Call.
type Call = sdk.Call

// State is retained for source compatibility.
//
// Deprecated: use sdk.State.
type State = sdk.State

// Host is retained for source compatibility.
//
// Deprecated: use sdk.Host.
type Host = sdk.Host

// Extension is retained for source compatibility.
//
// Deprecated: use sdk.Extension.
type Extension = sdk.Extension

// Base is retained for source compatibility.
//
// Deprecated: use sdk.Base.
type Base = sdk.Base

// ValidID is retained for source compatibility.
//
// Deprecated: use sdk.ValidID.
func ValidID(id string) bool { return sdk.ValidID(id) }

// Registration is the immutable set of callbacks accepted by the native adapter.
type Registration struct {
	Commands []string      `json:"commands"`
	Events   []events.Type `json:"events"`
}
