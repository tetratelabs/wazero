// Package experimental includes features we aren't yet sure about. These are enabled with context.Context keys.
//
// Note: All features here may be changed or deleted at any time, so use with caution!
package experimental

import (
	"github.com/tetratelabs/wazero/api"
)

// InternalModule is an api.Module that exposes additional
// information.
type InternalModule interface {
	api.Module

	// NumGlobal returns the count of all globals in the
	// module.
	NumGlobal() int

	// Global provides a read-only view for a given global
	// index. Panics if idx > GlobalsCount().
	Global(idx int) api.Global
}

// FunctionDefinition is an extend interface of api.FunctionDefinition with
// additional experimental methods.
type FunctionDefinition interface {
	api.FunctionDefinition
	// HumanName is a human-readable name for this function, which may not be
	// the same as Name. It will fallback to DebugName() if not available.
	HumanName() string
}
