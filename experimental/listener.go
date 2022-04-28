package experimental

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// FunctionListenerFactoryKey is a context.Context Value key. Its associated value should be a FunctionListenerFactory.
// Note: This is interpreter-only for now!
type FunctionListenerFactoryKey struct{}

// FunctionListenerFactory returns FunctionListeners to be notified when a function is called.
type FunctionListenerFactory interface {
	// NewListener returns a FunctionListener for a defined function. If nil is returned, no
	// listener will be notified.
	NewListener(FunctionDefinition) FunctionListener
}

// FunctionListener can be registered for any function via FunctionListenerFactory to
// be notified when the function is called.
type FunctionListener interface {
	// Before is invoked before a function is called. ctx is the context of the caller function.
	// The returned context will be used as the context of this function call. To add context
	// information for this function call, add it to ctx and return the updated context. If
	// no context information is needed, return ctx as is.
	Before(context.Context, []uint64) context.Context

	// After is invoked after a function is called. ctx is the context of this function call.
	After(context.Context, []uint64)
}

// FunctionDefinition includes information about a function available pre-instantiation.
type FunctionDefinition interface {
	// ModuleName is the possibly empty name of the module defining this function.
	ModuleName() string

	// Index is the position in the module's function index namespace, imports first.
	Index() uint32

	// Name is the module-defined name of the function, which is not necessarily the same as its export name.
	Name() string

	// ExportNames include all exported names for the given function.
	ExportNames() []string

	// ParamTypes are the parameters of the function.
	ParamTypes() []api.ValueType

	// ParamNames are index-correlated with ParamTypes or nil if not available for one or more parameters.
	ParamNames() []string

	// ResultTypes are the results of the function.
	ResultTypes() []api.ValueType
}
