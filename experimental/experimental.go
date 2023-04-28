// Package experimental includes features we aren't yet sure about. These are enabled with context.Context keys.
//
// Note: All features here may be changed or deleted at any time, so use with caution!
package experimental

import (
	"github.com/tetratelabs/wazero/api"
)

// InternalModule exposes additional module information not available through
// api.Module.
type InternalModule interface {
	// GlobalsCount returns the count of all globals in the module.
	GlobalsCount() int

	// ViewGlobal provides a read-only view for a given global index. Panics if idx > GlobalsCount().
	ViewGlobal(idx int) api.Global
}
