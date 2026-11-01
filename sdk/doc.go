// Package sdk defines the public API for trusted Go plugin packages.
// Start with CommandPlugin or EventPlugin; implement Extension for resourceful
// plugins that need full lifecycle hooks. Both use the same host-managed runtime.
// SDK permissions describe framework capabilities, not an OS security sandbox.
package sdk
