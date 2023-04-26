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
	// ViewGlobals return a copy of all the globals defined in the module. It
	// includes non-exported globals.
	ViewGlobals() []api.Global
}
