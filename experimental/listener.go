package experimental

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// StackIterator allows iterating on each function of the call stack, starting
// from the top. At least one call to Next() is required to start the iteration.
//
// Note: The iterator provides a view of the call stack at the time of
// iteration. As a result, parameter values may be different than the ones their
// function was called with.
type StackIterator interface {
	// Next moves the iterator to the next function in the stack. Returns
	// false if it reached the bottom of the stack.
	Next() bool
	// FunctionDefinition returns the function type of the current function.
	FunctionDefinition() api.FunctionDefinition
	// SourceOffset computes the offset in the module Code section where the
	// call occured (translated for native functions), or the beginning of
	// function for the top of the stack. Returns 0 if the source offset
	// cannot be calculated.
	//
	// The source offset is meant to help map the function calls to their
	// location in the original source files.
	SourceOffset() uint64
	// Parameters returns api.ValueType-encoded parameters of the current
	// function. Do not modify the content of the slice, and copy out any
	// value you need.
	Parameters() []uint64
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
	//   - stackIterator: iterator on the call stack. At least one entry is
	//     guaranteed (the called function), whose Args() will be equal to
	//     paramValues. The iterator will be reused between calls to Before.
	//
	// Note: api.Memory is meant for inspection, not modification.
	// mod can be cast to InternalModule to read non-exported globals.
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

// FunctionListenerFunc is a function type implementing the FunctionListener
// interface, making it possible to use regular functions and methods as
// listeners of function invocation.
//
// The FunctionListener interface declares two methods (Before and After),
// but this type invokes its value only when Before is called. It is best
// suites for cases where the host does not need to perform correlation
// between the start and end of the function call.
type FunctionListenerFunc func(context.Context, api.Module, api.FunctionDefinition, []uint64, StackIterator)

// Before satisfies the FunctionListener interface, calls f.
func (f FunctionListenerFunc) Before(ctx context.Context, mod api.Module, def api.FunctionDefinition, paramValues []uint64, stackIterator StackIterator) context.Context {
	f(ctx, mod, def, paramValues, stackIterator)
	return ctx
}

// After is declared to satisfy the FunctionListener interface, but it does
// nothing.
func (f FunctionListenerFunc) After(context.Context, api.Module, api.FunctionDefinition, error, []uint64) {
}

// FunctionListenerFactoryFunc is a function type implementing the
// FunctionListenerFactory interface, making it possible to use regular
// functions and methods as factory of function listeners.
type FunctionListenerFactoryFunc func(api.FunctionDefinition) FunctionListener

// NewListener satisfies the FunctionListenerFactory interface, calls f.
func (f FunctionListenerFactoryFunc) NewListener(def api.FunctionDefinition) FunctionListener {
	return f(def)
}

// MultiFunctionListenerFactory constructs a FunctionListenerFactory which
// combines the listeners created by each of the factories passed as arguments.
//
// This function is useful when multiple listeners need to be hooked to a module
// because the propagation mechanism based on installing a listener factory in
// the context.Context used when instantiating modules allows for a single
// listener to be installed.
//
// The stack iterator passed to the Before method is reset so that each listener
// can iterate the call stack independently without impacting the ability of
// other listeners to do so.
func MultiFunctionListenerFactory(factories ...FunctionListenerFactory) FunctionListenerFactory {
	multi := make(multiFunctionListenerFactory, len(factories))
	copy(multi, factories)
	return multi
}

type multiFunctionListenerFactory []FunctionListenerFactory

func (multi multiFunctionListenerFactory) NewListener(def api.FunctionDefinition) FunctionListener {
	var lstns []FunctionListener
	for _, factory := range multi {
		if lstn := factory.NewListener(def); lstn != nil {
			lstns = append(lstns, lstn)
		}
	}
	switch len(lstns) {
	case 0:
		return nil
	case 1:
		return lstns[0]
	default:
		return &multiFunctionListener{lstns: lstns}
	}
}

type multiFunctionListener struct {
	lstns []FunctionListener
	stack stackIterator
}

func (multi *multiFunctionListener) Before(ctx context.Context, mod api.Module, def api.FunctionDefinition, params []uint64, si StackIterator) context.Context {
	multi.stack.base = si
	for _, lstn := range multi.lstns {
		multi.stack.index = -1
		ctx = lstn.Before(ctx, mod, def, params, &multi.stack)
	}
	return ctx
}

func (multi *multiFunctionListener) After(ctx context.Context, mod api.Module, def api.FunctionDefinition, err error, results []uint64) {
	for _, lstn := range multi.lstns {
		lstn.After(ctx, mod, def, err, results)
	}
}

type parameters struct {
	values []uint64
	limits []uint8
}

func (ps *parameters) append(values []uint64) {
	ps.values = append(ps.values, values...)
	ps.limits = append(ps.limits, uint8(len(ps.values)))
}

func (ps *parameters) clear() {
	ps.values = ps.values[:0]
	ps.limits = ps.limits[:0]
}

func (ps *parameters) index(i int) []uint64 {
	j := uint8(0)
	k := ps.limits[i]
	if i > 0 {
		j = ps.limits[i-1]
	}
	return ps.values[j:k:k]
}

type stackIterator struct {
	base   StackIterator
	index  int
	pcs    []uint64
	fns    []api.FunctionDefinition
	params parameters
}

func (si *stackIterator) Next() bool {
	if si.base != nil {
		si.pcs = si.pcs[:0]
		si.fns = si.fns[:0]
		si.params.clear()

		for si.base.Next() {
			si.pcs = append(si.pcs, si.base.SourceOffset())
			si.fns = append(si.fns, si.base.FunctionDefinition())
			si.params.append(si.base.Parameters())
		}

		si.base = nil
	}
	si.index++
	return si.index < len(si.pcs)
}

func (si *stackIterator) SourceOffset() uint64 {
	return si.pcs[si.index]
}

func (si *stackIterator) FunctionDefinition() api.FunctionDefinition {
	return si.fns[si.index]
}

func (si *stackIterator) Parameters() []uint64 {
	return si.params.index(si.index)
}

// TODO: We need to add tests to enginetest to ensure contexts nest. A good test can use a combination of call and call
// indirect in terms of depth and breadth. The test could show a tree 3 calls deep where the there are a couple calls at
// each depth under the root. The main thing this can help prevent is accidentally swapping the context internally.

// TODO: Errors aren't handled, and the After hook should accept one along with the result values.

// TODO: The context parameter of the After hook is not the same as the Before hook. This means interceptor patterns
// are awkward. e.g. something like timing is difficult as it requires propagating a stack. Otherwise, nested calls will
// overwrite each other's "since" time. Propagating a stack is further awkward as the After hook needs to know the
// position to read from which might be subtle.
