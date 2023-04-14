package experimental

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// StackIterator allows iterating on each function of the call stack, starting
// from the top. At least one call to Next() is required to start the iteration.
//
// Example:
//
//	for it.Next() {
//		fmt.Printf("function: %s, args: %v", it.FnType(), it.Args())
//	}
type StackIterator interface {
	// Next moves the iterator to the next function in the stack. Returns false
	// if it reached the bottom of the stack.
	Next() bool
	// FnType returns the function type of the current function.
	FnType() api.FunctionDefinition
	// Args returns the arguments of the current function, if any.
	Args() []uint64
}

// FunctionListenerFactoryKey is a context.Context Value key. Its associated value should be a FunctionListenerFactory.
//
// See https://github.com/tetratelabs/wazero/issues/451
type FunctionListenerFactoryKey struct{}

// FunctionListenerFactory returns FunctionListeners to be notified when a
// function is called.
type FunctionListenerFactory interface {
	// NewListener returns a FunctionListener for a defined function. If nil is
	// returned, no listener will be notified.
	NewListener(api.FunctionDefinition) FunctionListener
	// ^^ A single instance can be returned to avoid instantiating a listener
	// per function, especially as they may be thousands of functions. Shared
	// listeners use their FunctionDefinition parameter to clarify.
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
	//   - mod: the calling module.
	//   - def: the function definition.
	//   - paramValues:  api.ValueType encoded parameters.
	//
	// Note: api.Memory is meant for inspection, not modification.
	Before(ctx context.Context, mod api.Module, def api.FunctionDefinition, paramValues []uint64, stackIterator StackIterator) context.Context

	// After is invoked after a function is called.
	//
	// # Params
	//
	//   - ctx: the context returned by Before.
	//   - mod: the calling module.
	//   - def: the function definition.
	//   - err: nil if the function didn't err
	//   - resultValues: api.ValueType encoded results.
	//
	// Note: api.Memory is meant for inspection, not modification.
	After(ctx context.Context, mod api.Module, def api.FunctionDefinition, err error, resultValues []uint64)
}

// TODO: We need to add tests to enginetest to ensure contexts nest. A good test can use a combination of call and call
// indirect in terms of depth and breadth. The test could show a tree 3 calls deep where the there are a couple calls at
// each depth under the root. The main thing this can help prevent is accidentally swapping the context internally.

// TODO: Errors aren't handled, and the After hook should accept one along with the result values.

// TODO: The context parameter of the After hook is not the same as the Before hook. This means interceptor patterns
// are awkward. e.g. something like timing is difficult as it requires propagating a stack. Otherwise, nested calls will
// overwrite each other's "since" time. Propagating a stack is further awkward as the After hook needs to know the
// position to read from which might be subtle.
