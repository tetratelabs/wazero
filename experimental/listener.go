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
	// Function describes the function called by the current frame.
	Function() InternalFunction
	// ProgramCounter returns the program counter associated with the
	// function call.
	ProgramCounter() ProgramCounter
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
	// NewFunctionListener returns a FunctionListener for a defined function.
	// If nil is returned, no listener will be notified.
	NewFunctionListener(api.FunctionDefinition) FunctionListener
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

// NewFunctionListener satisfies the FunctionListenerFactory interface, calls f.
func (f FunctionListenerFactoryFunc) NewFunctionListener(def api.FunctionDefinition) FunctionListener {
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

func (multi multiFunctionListenerFactory) NewFunctionListener(def api.FunctionDefinition) FunctionListener {
	var lstns []FunctionListener
	for _, factory := range multi {
		if lstn := factory.NewFunctionListener(def); lstn != nil {
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
	fns    []InternalFunction
	params parameters
}

func (si *stackIterator) Next() bool {
	if si.base != nil {
		si.pcs = si.pcs[:0]
		si.fns = si.fns[:0]
		si.params.clear()

		for si.base.Next() {
			si.pcs = append(si.pcs, uint64(si.base.ProgramCounter()))
			si.fns = append(si.fns, si.base.Function())
			si.params.append(si.base.Parameters())
		}

		si.base = nil
	}
	si.index++
	return si.index < len(si.pcs)
}

func (si *stackIterator) ProgramCounter() ProgramCounter {
	return ProgramCounter(si.pcs[si.index])
}

func (si *stackIterator) Function() InternalFunction {
	return si.fns[si.index]
}

func (si *stackIterator) Parameters() []uint64 {
	return si.params.index(si.index)
}

// StackFrame represents a frame on the call stack.
type StackFrame struct {
	Function     api.Function
	Params       []uint64
	Results      []uint64
	PC           uint64
	SourceOffset uint64
}

type internalFunction struct {
	definition   api.FunctionDefinition
	sourceOffset uint64
}

func (f internalFunction) Definition() api.FunctionDefinition {
	return f.definition
}

func (f internalFunction) SourceOffsetForPC(pc ProgramCounter) uint64 {
	return f.sourceOffset
}

// stackFrameIterator is an implementation of the experimental.stackFrameIterator
// interface.
type stackFrameIterator struct {
	index int
	stack []StackFrame
	fndef []api.FunctionDefinition
}

func (si *stackFrameIterator) Next() bool {
	si.index++
	return si.index < len(si.stack)
}

func (si *stackFrameIterator) Function() InternalFunction {
	return internalFunction{
		definition:   si.fndef[si.index],
		sourceOffset: si.stack[si.index].SourceOffset,
	}
}

func (si *stackFrameIterator) ProgramCounter() ProgramCounter {
	return ProgramCounter(si.stack[si.index].PC)
}

func (si *stackFrameIterator) Parameters() []uint64 {
	return si.stack[si.index].Params
}

// NewStackIterator constructs a stack iterator from a list of stack frames.
// The top most frame is the last one.
func NewStackIterator(stack ...StackFrame) StackIterator {
	si := &stackFrameIterator{
		index: -1,
		stack: make([]StackFrame, len(stack)),
		fndef: make([]api.FunctionDefinition, len(stack)),
	}
	for i := range stack {
		si.stack[i] = stack[len(stack)-(i+1)]
	}
	// The size of function definition is only one pointer which should allow
	// the compiler to optimize the conversion to api.FunctionDefinition; but
	// the presence of internal.WazeroOnlyType, despite being defined as an
	// empty struct, forces a heap allocation that we amortize by caching the
	// result.
	for i, frame := range stack {
		si.fndef[i] = frame.Function.Definition()
	}
	return si
}

// BenchmarkFunctionListener implements a benchmark for function listeners.
//
// The benchmark calls Before and After methods repeatedly using the provided
// module an stack frames to invoke the methods.
//
// The stack frame is a representation of the call stack that the Before method
// will be invoked with. The top of the stack is stored at index zero. The stack
// must contain at least one frame or the benchmark will fail.
func BenchmarkFunctionListener(n int, module api.Module, stack []StackFrame, listener FunctionListener) {
	if len(stack) == 0 {
		panic("cannot benchmark function listener with an empty stack")
	}

	functionDefinition := stack[0].Function.Definition()
	functionParams := stack[0].Params
	functionResults := stack[0].Results
	stackIterator := &stackIterator{base: NewStackIterator(stack...)}
	ctx := context.Background()

	for i := 0; i < n; i++ {
		stackIterator.index = -1
		callContext := listener.Before(ctx, module, functionDefinition, functionParams, stackIterator)
		listener.After(callContext, module, functionDefinition, nil, functionResults)
	}
}

// TODO: We need to add tests to enginetest to ensure contexts nest. A good test can use a combination of call and call
// indirect in terms of depth and breadth. The test could show a tree 3 calls deep where the there are a couple calls at
// each depth under the root. The main thing this can help prevent is accidentally swapping the context internally.

// TODO: Errors aren't handled, and the After hook should accept one along with the result values.

// TODO: The context parameter of the After hook is not the same as the Before hook. This means interceptor patterns
// are awkward. e.g. something like timing is difficult as it requires propagating a stack. Otherwise, nested calls will
// overwrite each other's "since" time. Propagating a stack is further awkward as the After hook needs to know the
// position to read from which might be subtle.
