package experimentalapi

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// FunctionListenerFactoryKey is a context.Context Value key. Its associated value should be a FunctionListenerFactory.
type FunctionListenerFactoryKey struct{}

// FunctionListenerFactory returns FunctionListeners to be notified when a function is called.
type FunctionListenerFactory interface {
	// NewListener returns a FunctionListener for a defined function. If nil is returned, no
	// listener will be notified.
	NewListener(info FunctionInfo) FunctionListener
}

// FunctionListener can be registered for any function via FunctionListenerFactory to
// be notified when the function is called.
type FunctionListener interface {
	// Before is invoked before a function is called. ctx is the context of the caller function.
	// The returned context will be used as the context of this function call. To add context
	// information for this function call, add it to ctx and return the updated context. If
	// no context information is needed, return ctx as is.
	Before(ctx context.Context) context.Context

	// After is invoked after a function is called. ctx is the context of this function call.
	After(ctx context.Context)
}

// ValueInfo are information about the definition of a parameter or return value.
type ValueInfo struct {
	// The name of the value. Empty if name is not available.
	Name string

	// The type of the value.
	Type api.ValueType
}

func (info ValueInfo) String() string {
	n := info.Name
	if len(n) == 0 {
		n = "<unknown>"
	}
	return fmt.Sprintf("%v: %v", n, api.ValueTypeName(info.Type))
}

// FunctionInfo are information about a function available pre-instantiation.
type FunctionInfo struct {
	// The name of the module the function is defined in.
	ModuleName string

	// The name of the function. This will be the name of the export for an exported function.
	// Empty if name is not available.
	Name string

	// The function parameters.
	Params []ValueInfo

	// The function return values.
	Returns []ValueInfo
}
