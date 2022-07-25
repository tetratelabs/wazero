package experimental

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// FunctionListenerFactoryKey is a context.Context Value key. Its associated value should be a FunctionListenerFactory.
//
// Note: This is interpreter-only for now!
//
// See https://github.com/tetratelabs/wazero/issues/451
type FunctionListenerFactoryKey struct{}

// FunctionListenerFactory returns FunctionListeners to be notified when a
// function is called.
type FunctionListenerFactory interface {
	// NewListener returns a FunctionListener for a defined function. If nil is
	// returned, no listener will be notified.
	NewListener(api.FunctionDefinition) FunctionListener
}

// FunctionListener can be registered for any function via
// FunctionListenerFactory to be notified when the function is called.
type FunctionListener interface {
	// Before is invoked before a function is called. The returned context will
	// be used as the context of this function call.
	//
	// # Params
	//
	//   - ctx: the context of the caller function which must be the same
	//	   instance or parent of the result.
	//   - def: the function definition.
	//   - paramValues:  api.ValueType encoded parameters.
	Before(ctx context.Context, def api.FunctionDefinition, paramValues []uint64) context.Context

	// After is invoked after a function is called.
	//
	// # Params
	//
	//   - ctx: the context returned by Before.
	//   - def: the function definition.
	//   - err: nil if the function didn't err
	//   - resultValues: api.ValueType encoded results.
	After(ctx context.Context, def api.FunctionDefinition, err error, resultValues []uint64)
}

// TODO: We need to add tests to enginetest to ensure contexts nest. A good test can use a combination of call and call
// indirect in terms of depth and breadth. The test could show a tree 3 calls deep where the there are a couple calls at
// each depth under the root. The main thing this can help prevent is accidentally swapping the context internally.

// TODO: Errors aren't handled, and the After hook should accept one along with the result values.

// TODO: The context parameter of the After hook is not the same as the Before hook. This means interceptor patterns
// are awkward. Ex. something like timing is difficult as it requires propagating a stack. Otherwise, nested calls will
// overwrite each other's "since" time. Propagating a stack is further awkward as the After hook needs to know the
// position to read from which might be subtle.
