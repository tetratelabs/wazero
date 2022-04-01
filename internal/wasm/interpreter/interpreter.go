package interpreter

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tetratelabs/wazero/internal/moremath"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/buildoptions"
	"github.com/tetratelabs/wazero/internal/wazeroir"
	"github.com/tetratelabs/wazero/sys"
)

var callStackCeiling = buildoptions.CallStackCeiling

// engine is an interpreter implementation of wasm.Engine
type engine struct {
	compiledFunctions map[*wasm.FunctionInstance]*compiledFunction // guarded by mutex.
	mux               sync.RWMutex
}

func NewEngine() wasm.Engine {
	return &engine{
		compiledFunctions: make(map[*wasm.FunctionInstance]*compiledFunction),
	}
}

func (e *engine) deleteCompiledFunction(f *wasm.FunctionInstance) {
	e.mux.Lock()
	defer e.mux.Unlock()
	delete(e.compiledFunctions, f)
}

func (e *engine) getCompiledFunction(f *wasm.FunctionInstance) (cf *compiledFunction, ok bool) {
	e.mux.RLock()
	defer e.mux.RUnlock()
	cf, ok = e.compiledFunctions[f]
	return
}

func (e *engine) addCompiledFunction(f *wasm.FunctionInstance, cf *compiledFunction) {
	e.mux.Lock()
	defer e.mux.Unlock()
	e.compiledFunctions[f] = cf
}

// moduleEngine implements wasm.ModuleEngine
type moduleEngine struct {
	// name is the name the module was instantiated with used for error handling.
	name string

	// compiledFunctions are the compiled functions in a module instances.
	// The index is module instance-scoped.
	compiledFunctions []*compiledFunction

	// parentEngine holds *engine from which this module engine is created from.
	parentEngine          *engine
	importedFunctionCount uint32

	// closed is the pointer used both to guard moduleEngine.CloseWithExitCode and to store the exit code.
	//
	// The update value is 1 + exitCode << 32. This ensures an exit code of zero isn't mistaken for never closed.
	//
	// Note: Exclusively reading and updating this with atomics guarantees cross-goroutine observations.
	// See /RATIONALE.md
	closed uint64
}

// callEngine holds context per moduleEngine.Call, and shared across all the
// function calls originating from the same moduleEngine.Call execution.
type callEngine struct {
	// stack contains the operands.
	// Note that all the values are represented as uint64.
	stack []uint64

	// frames are the function call stack.
	frames []*callFrame
}

func (me *moduleEngine) newCallEngine() *callEngine {
	return &callEngine{}
}

func (ce *callEngine) push(v uint64) {
	ce.stack = append(ce.stack, v)
}

func (ce *callEngine) pop() (v uint64) {
	// No need to check stack bound
	// as we can assume that all the operations
	// are valid thanks to validateFunction
	// at module validation phase
	// and wazeroir translation
	// before compilation.
	v = ce.stack[len(ce.stack)-1]
	ce.stack = ce.stack[:len(ce.stack)-1]
	return
}

func (ce *callEngine) drop(r *wazeroir.InclusiveRange) {
	// No need to check stack bound
	// as we can assume that all the operations
	// are valid thanks to validateFunction
	// at module validation phase
	// and wazeroir translation
	// before compilation.
	if r == nil {
		return
	} else if r.Start == 0 {
		ce.stack = ce.stack[:len(ce.stack)-1-r.End]
	} else {
		newStack := ce.stack[:len(ce.stack)-1-r.End]
		newStack = append(newStack, ce.stack[len(ce.stack)-r.Start:]...)
		ce.stack = newStack
	}
}

func (ce *callEngine) pushFrame(frame *callFrame) {
	if callStackCeiling <= len(ce.frames) {
		panic(wasm.ErrRuntimeCallStackOverflow)
	}
	ce.frames = append(ce.frames, frame)
}

func (ce *callEngine) popFrame() (frame *callFrame) {
	// No need to check stack bound as we can assume that all the operations are valid thanks to validateFunction at
	// module validation phase and wazeroir translation before compilation.
	oneLess := len(ce.frames) - 1
	frame = ce.frames[oneLess]
	ce.frames = ce.frames[:oneLess]
	return
}

type callFrame struct {
	// pc is the program counter representing the current position in compiledFunction.body.
	pc uint64
	// f is the compiled function used in this function frame.
	f *compiledFunction
}

type compiledFunction struct {
	moduleEngine *moduleEngine
	funcInstance *wasm.FunctionInstance
	body         []*interpreterOp
	hostFn       *reflect.Value
}

// Non-interface union of all the wazeroir operations.
type interpreterOp struct {
	kind   wazeroir.OperationKind
	b1, b2 byte
	us     []uint64
	rs     []*wazeroir.InclusiveRange
}

// NewModuleEngine implements the same method as documented on wasm.Engine.
func (e *engine) NewModuleEngine(name string, importedFunctions, moduleFunctions []*wasm.FunctionInstance, table *wasm.TableInstance, tableInit map[wasm.Index]wasm.Index) (wasm.ModuleEngine, error) {
	imported := uint32(len(importedFunctions))
	me := &moduleEngine{
		name:                  name,
		compiledFunctions:     make([]*compiledFunction, 0, imported+uint32(len(moduleFunctions))),
		parentEngine:          e,
		importedFunctionCount: imported,
	}

	for idx, f := range importedFunctions {
		cf, ok := e.getCompiledFunction(f)
		if !ok {
			return nil, fmt.Errorf("import[%d] func[%s.%s]: uncompiled", idx, f.Module.Name, f.Name)
		}
		me.compiledFunctions = append(me.compiledFunctions, cf)
	}

	for i, f := range moduleFunctions {
		var compiled *compiledFunction
		if f.Kind == wasm.FunctionKindWasm {
			ir, err := wazeroir.Compile(f)
			if err != nil {
				me.doClose() // safe because the reference to me was never leaked.
				// TODO(Adrian): extract Module.funcDesc so that errors here have more context
				return nil, fmt.Errorf("function[%d/%d] failed to lower to wazeroir: %w", i, len(moduleFunctions)-1, err)
			}

			compiled, err = e.lowerIROps(f, ir.Operations)
			if err != nil {
				me.doClose() // safe because the reference to me was never leaked.
				return nil, fmt.Errorf("function[%d/%d] failed to convert wazeroir operations: %w", i, len(moduleFunctions)-1, err)
			}
		} else {
			compiled = &compiledFunction{hostFn: f.GoFunc, funcInstance: f}
		}
		compiled.moduleEngine = me
		me.compiledFunctions = append(me.compiledFunctions, compiled)

		// Add the compiled function to the store-wide engine as well so that
		// the future importing module can refer the function instance.
		e.addCompiledFunction(f, compiled)
	}

	for elemIdx, funcidx := range tableInit { // Initialize any elements with compiled functions
		table.Table[elemIdx] = me.compiledFunctions[funcidx]
	}
	return me, nil
}

// Release implements wasm.Engine Release
func (me *moduleEngine) Release() error {
	// Release all the function instances declared in this module.
	for _, cf := range me.compiledFunctions[me.importedFunctionCount:] {
		me.parentEngine.deleteCompiledFunction(cf.funcInstance)
	}
	return nil
}

// lowerIROps lowers the wazeroir operations to engine friendly struct.
func (e *engine) lowerIROps(f *wasm.FunctionInstance,
	ops []wazeroir.Operation) (*compiledFunction, error) {
	ret := &compiledFunction{funcInstance: f}
	labelAddress := map[string]uint64{}
	onLabelAddressResolved := map[string][]func(addr uint64){}
	for _, original := range ops {
		op := &interpreterOp{kind: original.Kind()}
		switch o := original.(type) {
		case *wazeroir.OperationUnreachable:
		case *wazeroir.OperationLabel:
			labelKey := o.Label.String()
			address := uint64(len(ret.body))
			labelAddress[labelKey] = address
			for _, cb := range onLabelAddressResolved[labelKey] {
				cb(address)
			}
			delete(onLabelAddressResolved, labelKey)
			// We just ignore the label operation
			// as we translate branch operations to the direct address jmp.
			continue
		case *wazeroir.OperationBr:
			op.us = make([]uint64, 1)
			if o.Target.IsReturnTarget() {
				// Jmp to the end of the possible binary.
				op.us[0] = math.MaxUint64
			} else {
				labelKey := o.Target.String()
				addr, ok := labelAddress[labelKey]
				if !ok {
					// If this is the forward jump (e.g. to the continuation of if, etc.),
					// the target is not emitted yet, so resolve the address later.
					onLabelAddressResolved[labelKey] = append(onLabelAddressResolved[labelKey],
						func(addr uint64) {
							op.us[0] = addr
						},
					)
				} else {
					op.us[0] = addr
				}
			}
		case *wazeroir.OperationBrIf:
			op.rs = make([]*wazeroir.InclusiveRange, 2)
			op.us = make([]uint64, 2)
			for i, target := range []*wazeroir.BranchTargetDrop{o.Then, o.Else} {
				op.rs[i] = target.ToDrop
				if target.Target.IsReturnTarget() {
					// Jmp to the end of the possible binary.
					op.us[i] = math.MaxUint64
				} else {
					labelKey := target.Target.String()
					addr, ok := labelAddress[labelKey]
					if !ok {
						i := i
						// If this is the forward jump (e.g. to the continuation of if, etc.),
						// the target is not emitted yet, so resolve the address later.
						onLabelAddressResolved[labelKey] = append(onLabelAddressResolved[labelKey],
							func(addr uint64) {
								op.us[i] = addr
							},
						)
					} else {
						op.us[i] = addr
					}
				}
			}
		case *wazeroir.OperationBrTable:
			targets := append([]*wazeroir.BranchTargetDrop{o.Default}, o.Targets...)
			op.rs = make([]*wazeroir.InclusiveRange, len(targets))
			op.us = make([]uint64, len(targets))
			for i, target := range targets {
				op.rs[i] = target.ToDrop
				if target.Target.IsReturnTarget() {
					// Jmp to the end of the possible binary.
					op.us[i] = math.MaxUint64
				} else {
					labelKey := target.Target.String()
					addr, ok := labelAddress[labelKey]
					if !ok {
						i := i // pin index for later resolution
						// If this is the forward jump (e.g. to the continuation of if, etc.),
						// the target is not emitted yet, so resolve the address later.
						onLabelAddressResolved[labelKey] = append(onLabelAddressResolved[labelKey],
							func(addr uint64) {
								op.us[i] = addr
							},
						)
					} else {
						op.us[i] = addr
					}
				}
			}
		case *wazeroir.OperationCall:
			op.us = make([]uint64, 1)
			op.us = []uint64{uint64(o.FunctionIndex)}
		case *wazeroir.OperationCallIndirect:
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.TableIndex)
			op.us[1] = uint64(f.Module.Types[o.TypeIndex].TypeID)
		case *wazeroir.OperationDrop:
			op.rs = make([]*wazeroir.InclusiveRange, 1)
			op.rs[0] = o.Range
		case *wazeroir.OperationSelect:
		case *wazeroir.OperationPick:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.Depth)
		case *wazeroir.OperationSwap:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.Depth)
		case *wazeroir.OperationGlobalGet:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.Index)
		case *wazeroir.OperationGlobalSet:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.Index)
		case *wazeroir.OperationLoad:
			op.b1 = byte(o.Type)
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offset)
		case *wazeroir.OperationLoad8:
			op.b1 = byte(o.Type)
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offset)
		case *wazeroir.OperationLoad16:
			op.b1 = byte(o.Type)
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offset)
		case *wazeroir.OperationLoad32:
			if o.Signed {
				op.b1 = 1
			}
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offset)
		case *wazeroir.OperationStore:
			op.b1 = byte(o.Type)
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offset)
		case *wazeroir.OperationStore8:
			op.b1 = byte(o.Type)
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offset)
		case *wazeroir.OperationStore16:
			op.b1 = byte(o.Type)
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offset)
		case *wazeroir.OperationStore32:
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offset)
		case *wazeroir.OperationMemorySize:
		case *wazeroir.OperationMemoryGrow:
		case *wazeroir.OperationConstI32:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.Value)
		case *wazeroir.OperationConstI64:
			op.us = make([]uint64, 1)
			op.us[0] = o.Value
		case *wazeroir.OperationConstF32:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(math.Float32bits(o.Value))
		case *wazeroir.OperationConstF64:
			op.us = make([]uint64, 1)
			op.us[0] = math.Float64bits(o.Value)
		case *wazeroir.OperationEq:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationNe:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationEqz:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationLt:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationGt:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationLe:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationGe:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationAdd:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationSub:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationMul:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationClz:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationCtz:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationPopcnt:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationDiv:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationRem:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationAnd:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationOr:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationXor:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationShl:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationShr:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationRotl:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationRotr:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationAbs:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationNeg:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationCeil:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationFloor:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationTrunc:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationNearest:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationSqrt:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationMin:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationMax:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationCopysign:
			op.b1 = byte(o.Type)
		case *wazeroir.OperationI32WrapFromI64:
		case *wazeroir.OperationITruncFromF:
			op.b1 = byte(o.InputType)
			op.b2 = byte(o.OutputType)
		case *wazeroir.OperationFConvertFromI:
			op.b1 = byte(o.InputType)
			op.b2 = byte(o.OutputType)
		case *wazeroir.OperationF32DemoteFromF64:
		case *wazeroir.OperationF64PromoteFromF32:
		case *wazeroir.OperationI32ReinterpretFromF32,
			*wazeroir.OperationI64ReinterpretFromF64,
			*wazeroir.OperationF32ReinterpretFromI32,
			*wazeroir.OperationF64ReinterpretFromI64:
			// Reinterpret ops are essentially nop for engine mode
			// because we treat all values as uint64, and the reinterpret is only used at module
			// validation phase where we check type soundness of all the operations.
			// So just eliminate the ops.
			continue
		case *wazeroir.OperationExtend:
			if o.Signed {
				op.b1 = 1
			}
		case *wazeroir.OperationSignExtend32From8, *wazeroir.OperationSignExtend32From16, *wazeroir.OperationSignExtend64From8,
			*wazeroir.OperationSignExtend64From16, *wazeroir.OperationSignExtend64From32:
		default:
			return nil, fmt.Errorf("unreachable: a bug in wazeroir engine")
		}
		ret.body = append(ret.body, op)
	}

	if len(onLabelAddressResolved) > 0 {
		keys := make([]string, 0, len(onLabelAddressResolved))
		for key := range onLabelAddressResolved {
			keys = append(keys, key)
		}
		return nil, fmt.Errorf("labels are not defined: %s", strings.Join(keys, ","))
	}
	return ret, nil
}

// Name implements the same method as documented on wasm.ModuleEngine.
func (me *moduleEngine) Name() string {
	return me.name
}

// Call implements the same method as documented on wasm.ModuleEngine.
func (me *moduleEngine) Call(ctx *wasm.ModuleContext, f *wasm.FunctionInstance, params ...uint64) (results []uint64, err error) {
	// Note: The input parameters are pre-validated, so a compiled function is only absent on close. Updates to
	// compiledFunctions on close aren't locked, neither is this read.
	compiled := me.compiledFunctions[f.Index]
	if compiled == nil { // Lazy check the cause as it could be because the module was already closed.
		if err = failIfClosed(me); err == nil {
			panic(fmt.Errorf("BUG: %s.compiledFunctions[%d] was nil before close", me.name, f.Index))
		}
		return
	}

	paramSignature := f.Type.Params
	paramCount := len(params)
	if len(paramSignature) != paramCount {
		return nil, fmt.Errorf("expected %d params, but passed %d", len(paramSignature), paramCount)
	}

	ce := me.newCallEngine()
	defer func() {
		// If the module closed during the call, and the call didn't err for another reason, set an ExitError.
		if err == nil {
			err = failIfClosed(me)
		}
		// TODO: ^^ Will not fail if the function was imported from a closed module.

		if v := recover(); v != nil {
			if buildoptions.IsDebugMode {
				debug.PrintStack()
			}

			traces := make([]string, len(ce.frames))
			for i := 0; i < len(traces); i++ {
				frame := ce.popFrame()
				name := frame.f.funcInstance.Name
				// TODO: include DWARF symbols. See #58
				traces[i] = fmt.Sprintf("\t%d: %s", i, name)
			}

			runtimeErr, ok := v.(error)
			if ok {
				if runtimeErr.Error() == "runtime error: integer divide by zero" {
					runtimeErr = wasm.ErrRuntimeIntegerDivideByZero
				}
				err = fmt.Errorf("wasm runtime error: %w", runtimeErr)
			} else {
				err = fmt.Errorf("wasm runtime error: %v", v)
			}

			if len(traces) > 0 {
				err = fmt.Errorf("%w\nwasm backtrace:\n%s", err, strings.Join(traces, "\n"))
			}
		}
	}()

	for _, param := range params {
		ce.push(param)
	}
	if f.Kind == wasm.FunctionKindWasm {
		ce.callNativeFunc(ctx, compiled)
	} else {
		ce.callHostFunc(ctx, compiled)
	}
	results = make([]uint64, len(f.Type.Results))
	for i := range results {
		results[len(results)-1-i] = ce.pop()
	}
	return
}

// failIfClosed returns a sys.ExitError if moduleEngine.CloseWithExitCode was called.
func failIfClosed(me *moduleEngine) error {
	if closed := atomic.LoadUint64(&me.closed); closed != 0 {
		return sys.NewExitError(me.name, uint32(closed>>32)) // Unpack the high order bits as the exit code.
	}
	return nil
}

func (ce *callEngine) callHostFunc(ctx *wasm.ModuleContext, f *compiledFunction) {
	tp := f.hostFn.Type()
	in := make([]reflect.Value, tp.NumIn())

	wasmParamOffset := 0
	if f.funcInstance.Kind != wasm.FunctionKindGoNoContext {
		wasmParamOffset = 1
	}
	for i := len(in) - 1; i >= wasmParamOffset; i-- {
		val := reflect.New(tp.In(i)).Elem()
		raw := ce.pop()
		kind := tp.In(i).Kind()
		switch kind {
		case reflect.Float32:
			val.SetFloat(float64(math.Float32frombits(uint32(raw))))
		case reflect.Float64:
			val.SetFloat(math.Float64frombits(raw))
		case reflect.Uint32, reflect.Uint64:
			val.SetUint(raw)
		case reflect.Int32, reflect.Int64:
			val.SetInt(int64(raw))
		}
		in[i] = val
	}

	// A host function is invoked with the calling frame's memory, which may be different if in another module.
	if len(ce.frames) > 0 {
		ctx = ctx.WithMemory(ce.frames[len(ce.frames)-1].f.funcInstance.Module.Memory)
	}

	// Handle any special parameter zero
	if val := wasm.GetHostFunctionCallContextValue(f.funcInstance.Kind, ctx); val != nil {
		in[0] = *val
	}

	frame := &callFrame{f: f}
	ce.pushFrame(frame)
	for _, ret := range f.hostFn.Call(in) {
		switch ret.Kind() {
		case reflect.Float32:
			ce.push(uint64(math.Float32bits(float32(ret.Float()))))
		case reflect.Float64:
			ce.push(math.Float64bits(ret.Float()))
		case reflect.Uint32, reflect.Uint64:
			ce.push(ret.Uint())
		case reflect.Int32, reflect.Int64:
			ce.push(uint64(ret.Int()))
		default:
			panic("invalid return type")
		}
	}
	ce.popFrame()
}

func (ce *callEngine) callNativeFunc(ctx *wasm.ModuleContext, f *compiledFunction) {
	frame := &callFrame{f: f}
	moduleInst := f.funcInstance.Module
	memoryInst := moduleInst.Memory
	globals := moduleInst.Globals
	table := moduleInst.Table
	compiledFunctions := f.moduleEngine.compiledFunctions
	ce.pushFrame(frame)
	bodyLen := uint64(len(frame.f.body))
	for frame.pc < bodyLen {
		op := frame.f.body[frame.pc]
		// TODO: add description of each operation/case
		// on, for example, how many args are used,
		// how the stack is modified, etc.
		switch op.kind {
		case wazeroir.OperationKindUnreachable:
			panic(wasm.ErrRuntimeUnreachable)
		case wazeroir.OperationKindBr:
			{
				frame.pc = op.us[0]
			}
		case wazeroir.OperationKindBrIf:
			{
				if ce.pop() > 0 {
					ce.drop(op.rs[0])
					frame.pc = op.us[0]
				} else {
					ce.drop(op.rs[1])
					frame.pc = op.us[1]
				}
			}
		case wazeroir.OperationKindBrTable:
			{
				if v := int(ce.pop()); v < len(op.us)-1 {
					ce.drop(op.rs[v+1])
					frame.pc = op.us[v+1]
				} else {
					// Default branch.
					ce.drop(op.rs[0])
					frame.pc = op.us[0]
				}
			}
		case wazeroir.OperationKindCall:
			{
				f := compiledFunctions[op.us[0]]
				if f.hostFn != nil {
					ce.callHostFunc(ctx, f)
				} else {
					ce.callNativeFunc(ctx, f)
				}
				frame.pc++
			}
		case wazeroir.OperationKindCallIndirect:
			{
				offset := ce.pop()
				if offset >= uint64(len(table.Table)) {
					panic(wasm.ErrRuntimeInvalidTableAccess)
				}
				targetCompiledFunction, ok := table.Table[offset].(*compiledFunction)
				if !ok {
					panic(wasm.ErrRuntimeInvalidTableAccess)
				} else if uint64(targetCompiledFunction.funcInstance.TypeID) != op.us[1] {
					panic(wasm.ErrRuntimeIndirectCallTypeMismatch)
				}

				// Call in.
				if targetCompiledFunction.hostFn != nil {
					ce.callHostFunc(ctx, targetCompiledFunction)
				} else {
					ce.callNativeFunc(ctx, targetCompiledFunction)
				}
				frame.pc++
			}
		case wazeroir.OperationKindDrop:
			{
				ce.drop(op.rs[0])
				frame.pc++
			}
		case wazeroir.OperationKindSelect:
			{
				c := ce.pop()
				v2 := ce.pop()
				if c == 0 {
					_ = ce.pop()
					ce.push(v2)
				}
				frame.pc++
			}
		case wazeroir.OperationKindPick:
			{
				ce.push(ce.stack[len(ce.stack)-1-int(op.us[0])])
				frame.pc++
			}
		case wazeroir.OperationKindSwap:
			{
				index := len(ce.stack) - 1 - int(op.us[0])
				ce.stack[len(ce.stack)-1], ce.stack[index] = ce.stack[index], ce.stack[len(ce.stack)-1]
				frame.pc++
			}
		case wazeroir.OperationKindGlobalGet:
			{
				g := globals[op.us[0]]
				ce.push(g.Val)
				frame.pc++
			}
		case wazeroir.OperationKindGlobalSet:
			{
				g := globals[op.us[0]]
				g.Val = ce.pop()
				frame.pc++
			}
		case wazeroir.OperationKindLoad:
			{
				base := op.us[1] + ce.pop()
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeF32:
					if uint64(len(memoryInst.Buffer)) < base+4 {
						panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
					}
					ce.push(uint64(binary.LittleEndian.Uint32(memoryInst.Buffer[base:])))
				case wazeroir.UnsignedTypeI64, wazeroir.UnsignedTypeF64:
					if uint64(len(memoryInst.Buffer)) < base+8 {
						panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
					}
					ce.push(binary.LittleEndian.Uint64(memoryInst.Buffer[base:]))
				}
				frame.pc++
			}
		case wazeroir.OperationKindLoad8:
			{
				base := op.us[1] + ce.pop()
				if uint64(len(memoryInst.Buffer)) < base+1 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				switch wazeroir.SignedInt(op.b1) {
				case wazeroir.SignedInt32, wazeroir.SignedInt64:
					ce.push(uint64(int8(memoryInst.Buffer[base])))
				case wazeroir.SignedUint32, wazeroir.SignedUint64:
					ce.push(uint64(uint8(memoryInst.Buffer[base])))
				}
				frame.pc++
			}
		case wazeroir.OperationKindLoad16:
			{
				base := op.us[1] + ce.pop()
				if uint64(len(memoryInst.Buffer)) < base+2 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				switch wazeroir.SignedInt(op.b1) {
				case wazeroir.SignedInt32, wazeroir.SignedInt64:
					ce.push(uint64(int16(binary.LittleEndian.Uint16(memoryInst.Buffer[base:]))))
				case wazeroir.SignedUint32, wazeroir.SignedUint64:
					ce.push(uint64(binary.LittleEndian.Uint16(memoryInst.Buffer[base:])))
				}
				frame.pc++
			}
		case wazeroir.OperationKindLoad32:
			{
				base := op.us[1] + ce.pop()
				if uint64(len(memoryInst.Buffer)) < base+4 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				if op.b1 == 1 {
					ce.push(uint64(int32(binary.LittleEndian.Uint32(memoryInst.Buffer[base:]))))
				} else {
					ce.push(uint64(binary.LittleEndian.Uint32(memoryInst.Buffer[base:])))
				}
				frame.pc++
			}
		case wazeroir.OperationKindStore:
			{
				val := ce.pop()
				base := op.us[1] + ce.pop()
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeF32:
					if uint64(len(memoryInst.Buffer)) < base+4 {
						panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
					}
					binary.LittleEndian.PutUint32(memoryInst.Buffer[base:], uint32(val))
				case wazeroir.UnsignedTypeI64, wazeroir.UnsignedTypeF64:
					if uint64(len(memoryInst.Buffer)) < base+8 {
						panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
					}
					binary.LittleEndian.PutUint64(memoryInst.Buffer[base:], val)
				}
				frame.pc++
			}
		case wazeroir.OperationKindStore8:
			{
				val := byte(ce.pop())
				base := op.us[1] + ce.pop()
				if uint64(len(memoryInst.Buffer)) < base+1 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				memoryInst.Buffer[base] = val
				frame.pc++
			}
		case wazeroir.OperationKindStore16:
			{
				val := uint16(ce.pop())
				base := op.us[1] + ce.pop()
				if uint64(len(memoryInst.Buffer)) < base+2 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				binary.LittleEndian.PutUint16(memoryInst.Buffer[base:], val)
				frame.pc++
			}
		case wazeroir.OperationKindStore32:
			{
				val := uint32(ce.pop())
				base := op.us[1] + ce.pop()
				if uint64(len(memoryInst.Buffer)) < base+4 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				binary.LittleEndian.PutUint32(memoryInst.Buffer[base:], val)
				frame.pc++
			}
		case wazeroir.OperationKindMemorySize:
			{
				ce.push(uint64(memoryInst.PageSize()))
				frame.pc++
			}
		case wazeroir.OperationKindMemoryGrow:
			{
				n := ce.pop()
				res := memoryInst.Grow(uint32(n))
				ce.push(uint64(res))
				frame.pc++
			}
		case wazeroir.OperationKindConstI32, wazeroir.OperationKindConstI64,
			wazeroir.OperationKindConstF32, wazeroir.OperationKindConstF64:
			{
				ce.push(op.us[0])
				frame.pc++
			}
		case wazeroir.OperationKindEq:
			{
				var b bool
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeI64:
					v2, v1 := ce.pop(), ce.pop()
					b = v1 == v2
				case wazeroir.UnsignedTypeF32:
					v2, v1 := ce.pop(), ce.pop()
					b = math.Float32frombits(uint32(v2)) == math.Float32frombits(uint32(v1))
				case wazeroir.UnsignedTypeF64:
					v2, v1 := ce.pop(), ce.pop()
					b = math.Float64frombits(v2) == math.Float64frombits(v1)
				}
				if b {
					ce.push(1)
				} else {
					ce.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindNe:
			{
				var b bool
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeI64:
					v2, v1 := ce.pop(), ce.pop()
					b = v1 != v2
				case wazeroir.UnsignedTypeF32:
					v2, v1 := ce.pop(), ce.pop()
					b = math.Float32frombits(uint32(v2)) != math.Float32frombits(uint32(v1))
				case wazeroir.UnsignedTypeF64:
					v2, v1 := ce.pop(), ce.pop()
					b = math.Float64frombits(v2) != math.Float64frombits(v1)
				}
				if b {
					ce.push(1)
				} else {
					ce.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindEqz:
			{
				if ce.pop() == 0 {
					ce.push(1)
				} else {
					ce.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindLt:
			{
				v2 := ce.pop()
				v1 := ce.pop()
				var b bool
				switch wazeroir.SignedType(op.b1) {
				case wazeroir.SignedTypeInt32:
					b = int32(v1) < int32(v2)
				case wazeroir.SignedTypeInt64:
					b = int64(v1) < int64(v2)
				case wazeroir.SignedTypeUint32, wazeroir.SignedTypeUint64:
					b = v1 < v2
				case wazeroir.SignedTypeFloat32:
					b = math.Float32frombits(uint32(v1)) < math.Float32frombits(uint32(v2))
				case wazeroir.SignedTypeFloat64:
					b = math.Float64frombits(v1) < math.Float64frombits(v2)
				}
				if b {
					ce.push(1)
				} else {
					ce.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindGt:
			{
				v2 := ce.pop()
				v1 := ce.pop()
				var b bool
				switch wazeroir.SignedType(op.b1) {
				case wazeroir.SignedTypeInt32:
					b = int32(v1) > int32(v2)
				case wazeroir.SignedTypeInt64:
					b = int64(v1) > int64(v2)
				case wazeroir.SignedTypeUint32, wazeroir.SignedTypeUint64:
					b = v1 > v2
				case wazeroir.SignedTypeFloat32:
					b = math.Float32frombits(uint32(v1)) > math.Float32frombits(uint32(v2))
				case wazeroir.SignedTypeFloat64:
					b = math.Float64frombits(v1) > math.Float64frombits(v2)
				}
				if b {
					ce.push(1)
				} else {
					ce.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindLe:
			{
				v2 := ce.pop()
				v1 := ce.pop()
				var b bool
				switch wazeroir.SignedType(op.b1) {
				case wazeroir.SignedTypeInt32:
					b = int32(v1) <= int32(v2)
				case wazeroir.SignedTypeInt64:
					b = int64(v1) <= int64(v2)
				case wazeroir.SignedTypeUint32, wazeroir.SignedTypeUint64:
					b = v1 <= v2
				case wazeroir.SignedTypeFloat32:
					b = math.Float32frombits(uint32(v1)) <= math.Float32frombits(uint32(v2))
				case wazeroir.SignedTypeFloat64:
					b = math.Float64frombits(v1) <= math.Float64frombits(v2)
				}
				if b {
					ce.push(1)
				} else {
					ce.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindGe:
			{
				v2 := ce.pop()
				v1 := ce.pop()
				var b bool
				switch wazeroir.SignedType(op.b1) {
				case wazeroir.SignedTypeInt32:
					b = int32(v1) >= int32(v2)
				case wazeroir.SignedTypeInt64:
					b = int64(v1) >= int64(v2)
				case wazeroir.SignedTypeUint32, wazeroir.SignedTypeUint64:
					b = v1 >= v2
				case wazeroir.SignedTypeFloat32:
					b = math.Float32frombits(uint32(v1)) >= math.Float32frombits(uint32(v2))
				case wazeroir.SignedTypeFloat64:
					b = math.Float64frombits(v1) >= math.Float64frombits(v2)
				}
				if b {
					ce.push(1)
				} else {
					ce.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindAdd:
			{
				v2 := ce.pop()
				v1 := ce.pop()
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32:
					v := uint32(v1) + uint32(v2)
					ce.push(uint64(v))
				case wazeroir.UnsignedTypeI64:
					ce.push(v1 + v2)
				case wazeroir.UnsignedTypeF32:
					v := math.Float32frombits(uint32(v1)) + math.Float32frombits(uint32(v2))
					ce.push(uint64(math.Float32bits(v)))
				case wazeroir.UnsignedTypeF64:
					v := math.Float64frombits(v1) + math.Float64frombits(v2)
					ce.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindSub:
			{
				v2 := ce.pop()
				v1 := ce.pop()
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32:
					ce.push(uint64(uint32(v1) - uint32(v2)))
				case wazeroir.UnsignedTypeI64:
					ce.push(v1 - v2)
				case wazeroir.UnsignedTypeF32:
					v := math.Float32frombits(uint32(v1)) - math.Float32frombits(uint32(v2))
					ce.push(uint64(math.Float32bits(v)))
				case wazeroir.UnsignedTypeF64:
					v := math.Float64frombits(v1) - math.Float64frombits(v2)
					ce.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindMul:
			{
				v2 := ce.pop()
				v1 := ce.pop()
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32:
					ce.push(uint64(uint32(v1) * uint32(v2)))
				case wazeroir.UnsignedTypeI64:
					ce.push(v1 * v2)
				case wazeroir.UnsignedTypeF32:
					v := math.Float32frombits(uint32(v2)) * math.Float32frombits(uint32(v1))
					ce.push(uint64(math.Float32bits(v)))
				case wazeroir.UnsignedTypeF64:
					v := math.Float64frombits(v2) * math.Float64frombits(v1)
					ce.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindClz:
			{
				v := ce.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.push(uint64(bits.LeadingZeros32(uint32(v))))
				} else {
					// UnsignedInt64
					ce.push(uint64(bits.LeadingZeros64(v)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindCtz:
			{
				v := ce.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.push(uint64(bits.TrailingZeros32(uint32(v))))
				} else {
					// UnsignedInt64
					ce.push(uint64(bits.TrailingZeros64(v)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindPopcnt:
			{
				v := ce.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.push(uint64(bits.OnesCount32(uint32(v))))
				} else {
					// UnsignedInt64
					ce.push(uint64(bits.OnesCount64(v)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindDiv:
			{
				switch wazeroir.SignedType(op.b1) {
				case wazeroir.SignedTypeInt32:
					v2 := int32(ce.pop())
					v1 := int32(ce.pop())
					if v1 == math.MinInt32 && v2 == -1 {
						panic(wasm.ErrRuntimeIntegerOverflow)
					}
					ce.push(uint64(uint32(v1 / v2)))
				case wazeroir.SignedTypeInt64:
					v2 := int64(ce.pop())
					v1 := int64(ce.pop())
					if v1 == math.MinInt64 && v2 == -1 {
						panic(wasm.ErrRuntimeIntegerOverflow)
					}
					ce.push(uint64(v1 / v2))
				case wazeroir.SignedTypeUint32:
					v2 := uint32(ce.pop())
					v1 := uint32(ce.pop())
					ce.push(uint64(v1 / v2))
				case wazeroir.SignedTypeUint64:
					v2 := ce.pop()
					v1 := ce.pop()
					ce.push(v1 / v2)
				case wazeroir.SignedTypeFloat32:
					v2 := ce.pop()
					v1 := ce.pop()
					v := math.Float32frombits(uint32(v1)) / math.Float32frombits(uint32(v2))
					ce.push(uint64(math.Float32bits(v)))
				case wazeroir.SignedTypeFloat64:
					v2 := ce.pop()
					v1 := ce.pop()
					v := math.Float64frombits(v1) / math.Float64frombits(v2)
					ce.push(uint64(math.Float64bits(v)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindRem:
			{
				switch wazeroir.SignedInt(op.b1) {
				case wazeroir.SignedInt32:
					v2 := int32(ce.pop())
					v1 := int32(ce.pop())
					ce.push(uint64(uint32(v1 % v2)))
				case wazeroir.SignedInt64:
					v2 := int64(ce.pop())
					v1 := int64(ce.pop())
					ce.push(uint64(v1 % v2))
				case wazeroir.SignedUint32:
					v2 := uint32(ce.pop())
					v1 := uint32(ce.pop())
					ce.push(uint64(v1 % v2))
				case wazeroir.SignedUint64:
					v2 := ce.pop()
					v1 := ce.pop()
					ce.push(v1 % v2)
				}
				frame.pc++
			}
		case wazeroir.OperationKindAnd:
			{
				v2 := ce.pop()
				v1 := ce.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.push(uint64(uint32(v2) & uint32(v1)))
				} else {
					// UnsignedInt64
					ce.push(uint64(v2 & v1))
				}
				frame.pc++
			}
		case wazeroir.OperationKindOr:
			{
				v2 := ce.pop()
				v1 := ce.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.push(uint64(uint32(v2) | uint32(v1)))
				} else {
					// UnsignedInt64
					ce.push(uint64(v2 | v1))
				}
				frame.pc++
			}
		case wazeroir.OperationKindXor:
			{
				v2 := ce.pop()
				v1 := ce.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.push(uint64(uint32(v2) ^ uint32(v1)))
				} else {
					// UnsignedInt64
					ce.push(uint64(v2 ^ v1))
				}
				frame.pc++
			}
		case wazeroir.OperationKindShl:
			{
				v2 := ce.pop()
				v1 := ce.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.push(uint64(uint32(v1) << (uint32(v2) % 32)))
				} else {
					// UnsignedInt64
					ce.push(v1 << (v2 % 64))
				}
				frame.pc++
			}
		case wazeroir.OperationKindShr:
			{
				v2 := ce.pop()
				v1 := ce.pop()
				switch wazeroir.SignedInt(op.b1) {
				case wazeroir.SignedInt32:
					ce.push(uint64(int32(v1) >> (uint32(v2) % 32)))
				case wazeroir.SignedInt64:
					ce.push(uint64(int64(v1) >> (v2 % 64)))
				case wazeroir.SignedUint32:
					ce.push(uint64(uint32(v1) >> (uint32(v2) % 32)))
				case wazeroir.SignedUint64:
					ce.push(v1 >> (v2 % 64))
				}
				frame.pc++
			}
		case wazeroir.OperationKindRotl:
			{
				v2 := ce.pop()
				v1 := ce.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.push(uint64(bits.RotateLeft32(uint32(v1), int(v2))))
				} else {
					// UnsignedInt64
					ce.push(uint64(bits.RotateLeft64(v1, int(v2))))
				}
				frame.pc++
			}
		case wazeroir.OperationKindRotr:
			{
				v2 := ce.pop()
				v1 := ce.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.push(uint64(bits.RotateLeft32(uint32(v1), -int(v2))))
				} else {
					// UnsignedInt64
					ce.push(uint64(bits.RotateLeft64(v1, -int(v2))))
				}
				frame.pc++
			}
		case wazeroir.OperationKindAbs:
			{
				if op.b1 == 0 {
					// Float32
					const mask uint32 = 1 << 31
					ce.push(uint64(uint32(ce.pop()) &^ mask))
				} else {
					// Float64
					const mask uint64 = 1 << 63
					ce.push(uint64(ce.pop() &^ mask))
				}
				frame.pc++
			}
		case wazeroir.OperationKindNeg:
			{
				if op.b1 == 0 {
					// Float32
					v := -math.Float32frombits(uint32(ce.pop()))
					ce.push(uint64(math.Float32bits(v)))
				} else {
					// Float64
					v := -math.Float64frombits(ce.pop())
					ce.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindCeil:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Ceil(float64(math.Float32frombits(uint32(ce.pop()))))
					ce.push(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Ceil(float64(math.Float64frombits(ce.pop())))
					ce.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindFloor:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Floor(float64(math.Float32frombits(uint32(ce.pop()))))
					ce.push(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Floor(float64(math.Float64frombits(ce.pop())))
					ce.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindTrunc:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Trunc(float64(math.Float32frombits(uint32(ce.pop()))))
					ce.push(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Trunc(float64(math.Float64frombits(ce.pop())))
					ce.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindNearest:
			{
				if op.b1 == 0 {
					// Float32
					f := math.Float32frombits(uint32(ce.pop()))
					ce.push(uint64(math.Float32bits(moremath.WasmCompatNearestF32(f))))
				} else {
					// Float64
					f := math.Float64frombits(ce.pop())
					ce.push(math.Float64bits(moremath.WasmCompatNearestF64(f)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindSqrt:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Sqrt(float64(math.Float32frombits(uint32(ce.pop()))))
					ce.push(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Sqrt(float64(math.Float64frombits(ce.pop())))
					ce.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindMin:
			{
				if op.b1 == 0 {
					// Float32
					v2 := math.Float32frombits(uint32(ce.pop()))
					v1 := math.Float32frombits(uint32(ce.pop()))
					ce.push(uint64(math.Float32bits(float32(moremath.WasmCompatMin(float64(v1), float64(v2))))))
				} else {
					v2 := math.Float64frombits(ce.pop())
					v1 := math.Float64frombits(ce.pop())
					ce.push(math.Float64bits(moremath.WasmCompatMin(v1, v2)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindMax:
			{

				if op.b1 == 0 {
					// Float32
					v2 := math.Float32frombits(uint32(ce.pop()))
					v1 := math.Float32frombits(uint32(ce.pop()))
					ce.push(uint64(math.Float32bits(float32(moremath.WasmCompatMax(float64(v1), float64(v2))))))
				} else {
					// Float64
					v2 := math.Float64frombits(ce.pop())
					v1 := math.Float64frombits(ce.pop())
					ce.push(math.Float64bits(moremath.WasmCompatMax(v1, v2)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindCopysign:
			{
				if op.b1 == 0 {
					// Float32
					v2 := uint32(ce.pop())
					v1 := uint32(ce.pop())
					const signbit = 1 << 31
					ce.push(uint64(v1&^signbit | v2&signbit))
				} else {
					// Float64
					v2 := ce.pop()
					v1 := ce.pop()
					const signbit = 1 << 63
					ce.push(v1&^signbit | v2&signbit)
				}
				frame.pc++
			}
		case wazeroir.OperationKindI32WrapFromI64:
			{
				ce.push(uint64(uint32(ce.pop())))
				frame.pc++
			}
		case wazeroir.OperationKindITruncFromF:
			{
				if op.b1 == 0 {
					// Float32
					switch wazeroir.SignedInt(op.b2) {
					case wazeroir.SignedInt32:
						v := math.Trunc(float64(math.Float32frombits(uint32(ce.pop()))))
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < math.MinInt32 || v > math.MaxInt32 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						ce.push(uint64(int32(v)))
					case wazeroir.SignedInt64:
						v := math.Trunc(float64(math.Float32frombits(uint32(ce.pop()))))
						res := int64(v)
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < math.MinInt64 || v >= math.MaxInt64 {
							// Note: math.MaxInt64 is rounded up to math.MaxInt64+1 in 64-bit float representation,
							// and that's why we use '>=' not '>' to check overflow.
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						ce.push(uint64(res))
					case wazeroir.SignedUint32:
						v := math.Trunc(float64(math.Float32frombits(uint32(ce.pop()))))
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < 0 || v > math.MaxUint32 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						ce.push(uint64(uint32(v)))
					case wazeroir.SignedUint64:
						v := math.Trunc(float64(math.Float32frombits(uint32(ce.pop()))))
						res := uint64(v)
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < 0 || v >= math.MaxUint64 {
							// Note: math.MaxUint64 is rounded up to math.MaxUint64+1 in 64-bit float representation,
							// and that's why we use '>=' not '>' to check overflow.
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						ce.push(res)
					}
				} else {
					// Float64
					switch wazeroir.SignedInt(op.b2) {
					case wazeroir.SignedInt32:
						v := math.Trunc(math.Float64frombits(ce.pop()))
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < math.MinInt32 || v > math.MaxInt32 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						ce.push(uint64(int32(v)))
					case wazeroir.SignedInt64:
						v := math.Trunc(math.Float64frombits(ce.pop()))
						res := int64(v)
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < math.MinInt64 || v >= math.MaxInt64 {
							// Note: math.MaxInt64 is rounded up to math.MaxInt64+1 in 64-bit float representation,
							// and that's why we use '>=' not '>' to check overflow.
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						ce.push(uint64(res))
					case wazeroir.SignedUint32:
						v := math.Trunc(math.Float64frombits(ce.pop()))
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < 0 || v > math.MaxUint32 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						ce.push(uint64(uint32(v)))
					case wazeroir.SignedUint64:
						v := math.Trunc(math.Float64frombits(ce.pop()))
						res := uint64(v)
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < 0 || v >= math.MaxUint64 {
							// Note: math.MaxUint64 is rounded up to math.MaxUint64+1 in 64-bit float representation,
							// and that's why we use '>=' not '>' to check overflow.
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						ce.push(res)
					}
				}
				frame.pc++
			}
		case wazeroir.OperationKindFConvertFromI:
			{
				switch wazeroir.SignedInt(op.b1) {
				case wazeroir.SignedInt32:
					if op.b2 == 0 {
						// Float32
						v := float32(int32(ce.pop()))
						ce.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(int32(ce.pop()))
						ce.push(math.Float64bits(v))
					}
				case wazeroir.SignedInt64:
					if op.b2 == 0 {
						// Float32
						v := float32(int64(ce.pop()))
						ce.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(int64(ce.pop()))
						ce.push(math.Float64bits(v))
					}
				case wazeroir.SignedUint32:
					if op.b2 == 0 {
						// Float32
						v := float32(uint32(ce.pop()))
						ce.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(uint32(ce.pop()))
						ce.push(math.Float64bits(v))
					}
				case wazeroir.SignedUint64:
					if op.b2 == 0 {
						// Float32
						v := float32(ce.pop())
						ce.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(ce.pop())
						ce.push(math.Float64bits(v))
					}
				}
				frame.pc++
			}
		case wazeroir.OperationKindF32DemoteFromF64:
			{
				v := float32(math.Float64frombits(ce.pop()))
				ce.push(uint64(math.Float32bits(v)))
				frame.pc++
			}
		case wazeroir.OperationKindF64PromoteFromF32:
			{
				v := float64(math.Float32frombits(uint32(ce.pop())))
				ce.push(math.Float64bits(v))
				frame.pc++
			}
		case wazeroir.OperationKindExtend:
			{
				if op.b1 == 1 {
					// Signed.
					v := int64(int32(ce.pop()))
					ce.push(uint64(v))
				} else {
					v := uint64(uint32(ce.pop()))
					ce.push(v)
				}
				frame.pc++
			}

		case wazeroir.OperationKindSignExtend32From8:
			{
				v := int32(int8(ce.pop()))
				ce.push(uint64(v))
				frame.pc++
			}
		case wazeroir.OperationKindSignExtend32From16:
			{
				v := int32(int16(ce.pop()))
				ce.push(uint64(v))
				frame.pc++
			}
		case wazeroir.OperationKindSignExtend64From8:
			{
				v := int64(int8(ce.pop()))
				ce.push(uint64(v))
				frame.pc++
			}
		case wazeroir.OperationKindSignExtend64From16:
			{
				v := int64(int16(ce.pop()))
				ce.push(uint64(v))
				frame.pc++
			}
		case wazeroir.OperationKindSignExtend64From32:
			{
				v := int64(int32(ce.pop()))
				ce.push(uint64(v))
				frame.pc++
			}
		}
	}
	ce.popFrame()
}

// CloseWithExitCode implements the same method as documented on wasm.ModuleEngine.
func (me *moduleEngine) CloseWithExitCode(exitCode uint32) (bool, error) {
	closed := uint64(1) + uint64(exitCode)<<32 // Store exitCode as high-order bits.
	if !atomic.CompareAndSwapUint64(&me.closed, 0, closed) {
		return false, nil
	}
	me.doClose()
	return true, nil
}

// doClose releases all the function instances declared in this module.
func (me *moduleEngine) doClose() {
	for _, cf := range me.compiledFunctions[me.importedFunctionCount:] {
		me.parentEngine.deleteCompiledFunction(cf.funcInstance)
	}
}
