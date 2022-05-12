package interpreter

import (
	"context"
	"fmt"
	"math"
	"math/bits"
	"reflect"
	"strings"
	"sync"
	"unsafe"

	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/buildoptions"
	"github.com/tetratelabs/wazero/internal/moremath"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasmdebug"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

var callStackCeiling = buildoptions.CallStackCeiling

// engine is an interpreter implementation of wasm.Engine
type engine struct {
	enabledFeatures wasm.Features
	codes           map[wasm.ModuleID][]*code // guarded by mutex.
	mux             sync.RWMutex
}

func NewEngine(enabledFeatures wasm.Features) wasm.Engine {
	return &engine{
		enabledFeatures: enabledFeatures,
		codes:           map[wasm.ModuleID][]*code{},
	}
}

// DeleteCompiledModule implements the same method as documented on wasm.Engine.
func (e *engine) DeleteCompiledModule(m *wasm.Module) {
	e.deleteCodes(m)
}

func (e *engine) deleteCodes(module *wasm.Module) {
	e.mux.Lock()
	defer e.mux.Unlock()
	delete(e.codes, module.ID)
}

func (e *engine) addCodes(module *wasm.Module, fs []*code) {
	e.mux.Lock()
	defer e.mux.Unlock()
	e.codes[module.ID] = fs
}

func (e *engine) getCodes(module *wasm.Module) (fs []*code, ok bool) {
	e.mux.RLock()
	defer e.mux.RUnlock()
	fs, ok = e.codes[module.ID]
	return
}

// moduleEngine implements wasm.ModuleEngine
type moduleEngine struct {
	// name is the name the module was instantiated with used for error handling.
	name string

	// codes are the compiled functions in a module instances.
	// The index is module instance-scoped.
	functions []*function

	// parentEngine holds *engine from which this module engine is created from.
	parentEngine          *engine
	importedFunctionCount uint32
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

func (ce *callEngine) pushValue(v uint64) {
	ce.stack = append(ce.stack, v)
}

func (ce *callEngine) popValue() (v uint64) {
	// No need to check stack bound
	// as we can assume that all the operations
	// are valid thanks to validateFunction
	// at module validation phase
	// and wazeroir translation
	// before compilation.
	stackTopIndex := len(ce.stack) - 1
	v = ce.stack[stackTopIndex]
	ce.stack = ce.stack[:stackTopIndex]
	return
}

// peekValues peeks api.ValueType values from the stack and returns them in reverse order.
func (ce *callEngine) peekValues(count int) []uint64 {
	if count == 0 {
		return nil
	}
	stackTopIndex := len(ce.stack) - 1
	peeked := ce.stack[stackTopIndex-count : stackTopIndex]
	values := make([]uint64, 0, count)
	for i := count - 1; i >= 0; i-- {
		values = append(values, peeked[i])
	}
	return values
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
		panic(wasmruntime.ErrRuntimeCallStackOverflow)
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
	// pc is the program counter representing the current position in code.body.
	pc uint64
	// f is the compiled function used in this function frame.
	f *function
}

type code struct {
	body   []*interpreterOp
	hostFn *reflect.Value
}

type function struct {
	source *wasm.FunctionInstance
	body   []*interpreterOp
	hostFn *reflect.Value
}

// functionFromUintptr resurrects the original *function from the given uintptr
// which comes from either funcref table or OpcodeRefFunc instruction.
func functionFromUintptr(ptr uintptr) *function {
	// Wraps ptrs as the double pointer in order to avoid the unsafe access as detected by race detector.
	//
	// For example, if we have (*function)(unsafe.Pointer(ptr)) instead, then the race detector's "checkptr"
	// subroutine wanrs as "checkptr: pointer arithmetic result points to invalid allocation"
	// https://github.com/golang/go/blob/1ce7fcf139417d618c2730010ede2afb41664211/src/runtime/checkptr.go#L69
	var wrapped *uintptr = &ptr
	return *(**function)(unsafe.Pointer(wrapped))
}

func (c *code) instantiate(f *wasm.FunctionInstance) *function {
	return &function{
		source: f,
		body:   c.body,
		hostFn: c.hostFn,
	}
}

// interpreterOp is the compilation (engine.lowerIR) result of a wazeroir.Operation.
//
// Not all operations result in an interpreterOp, e.g. wazeroir.OperationI32ReinterpretFromF32, and some operations are
// more complex than others, e.g. wazeroir.OperationBrTable.
//
// Note: This is a form of union type as it can store fields needed for any operation. Hence, most fields are opaque and
// only relevant when in context of its kind.
type interpreterOp struct {
	// kind determines how to interpret the other fields in this struct.
	kind   wazeroir.OperationKind
	b1, b2 byte
	b3     bool
	us     []uint64
	rs     []*wazeroir.InclusiveRange
}

// CompileModule implements the same method as documented on wasm.Engine.
func (e *engine) CompileModule(ctx context.Context, module *wasm.Module) error {
	if _, ok := e.getCodes(module); ok { // cache hit!
		return nil
	}

	funcs := make([]*code, 0, len(module.FunctionSection))
	if module.IsHostModule() {
		// If this is the host module, there's nothing to do as the runtime representation of
		// host function in interpreter is its Go function itself as opposed to Wasm functions,
		// which need to be compiled down to wazeroir.
		for _, hf := range module.HostFunctionSection {
			funcs = append(funcs, &code{hostFn: hf})
		}
	} else {
		irs, err := wazeroir.CompileFunctions(ctx, e.enabledFeatures, module)
		if err != nil {
			return err
		}
		for i, ir := range irs {
			compiled, err := e.lowerIR(ir)
			if err != nil {
				return fmt.Errorf("function[%d/%d] failed to convert wazeroir operations: %w", i, len(module.FunctionSection)-1, err)
			}
			funcs = append(funcs, compiled)
		}
	}
	e.addCodes(module, funcs)
	return nil

}

// NewModuleEngine implements the same method as documented on wasm.Engine.
func (e *engine) NewModuleEngine(name string, module *wasm.Module, importedFunctions, moduleFunctions []*wasm.FunctionInstance, tables []*wasm.TableInstance, tableInits []wasm.TableInitEntry) (wasm.ModuleEngine, error) {
	imported := uint32(len(importedFunctions))
	me := &moduleEngine{
		name:                  name,
		parentEngine:          e,
		importedFunctionCount: imported,
	}

	for _, f := range importedFunctions {
		cf := f.Module.Engine.(*moduleEngine).functions[f.Idx]
		me.functions = append(me.functions, cf)
	}

	codes, ok := e.getCodes(module)
	if !ok {
		return nil, fmt.Errorf("source module for %s must be compiled before instantiation", name)
	}

	for i, c := range codes {
		f := moduleFunctions[i]
		insntantiatedcode := c.instantiate(f)
		me.functions = append(me.functions, insntantiatedcode)
	}

	for _, init := range tableInits {
		references := tables[init.TableIndex].References
		if int(init.Offset)+len(init.FunctionIndexes) > len(references) {
			return me, wasm.ErrElementOffsetOutOfBounds
		}

		for i, funcindex := range init.FunctionIndexes {
			if funcindex != nil {
				references[init.Offset+uint32(i)] = uintptr(unsafe.Pointer(me.functions[*funcindex]))
			}
		}
	}
	return me, nil
}

// lowerIR lowers the wazeroir operations to engine friendly struct.
func (e *engine) lowerIR(ir *wazeroir.CompilationResult) (*code, error) {
	ops := ir.Operations
	ret := &code{}
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
			op.us[0] = uint64(o.TypeIndex)
			op.us[1] = uint64(o.TableIndex)
		case *wazeroir.OperationDrop:
			op.rs = make([]*wazeroir.InclusiveRange, 1)
			op.rs[0] = o.Depth
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
			op.b3 = o.NonTrapping
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
		case *wazeroir.OperationMemoryInit:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.DataIndex)
		case *wazeroir.OperationDataDrop:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.DataIndex)
		case *wazeroir.OperationMemoryCopy:
		case *wazeroir.OperationMemoryFill:
		case *wazeroir.OperationTableInit:
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.ElemIndex)
			op.us[1] = uint64(o.TableIndex)
		case *wazeroir.OperationElemDrop:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.ElemIndex)
		case *wazeroir.OperationTableCopy:
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.SrcTableIndex)
			op.us[1] = uint64(o.DstTableIndex)
		case *wazeroir.OperationRefFunc:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.FunctionIndex)
		case *wazeroir.OperationTableGet:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.TableIndex)
		case *wazeroir.OperationTableSet:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.TableIndex)
		case *wazeroir.OperationTableSize:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.TableIndex)
		case *wazeroir.OperationTableGrow:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.TableIndex)
		case *wazeroir.OperationTableFill:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.TableIndex)
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

// CreateFuncElementInstance implements the same method as documented on wasm.ModuleEngine.
func (me *moduleEngine) CreateFuncElementInstance(indexes []*wasm.Index) *wasm.ElementInstance {
	refs := make([]wasm.Reference, len(indexes))
	for i, index := range indexes {
		if index != nil {
			refs[i] = uintptr(unsafe.Pointer(me.functions[*index]))
		}
	}
	return &wasm.ElementInstance{
		References: refs,
		Type:       wasm.RefTypeFuncref,
	}
}

// InitializeFuncrefGlobals implements the same method as documented on wasm.InitializeFuncrefGlobals.
func (me *moduleEngine) InitializeFuncrefGlobals(globals []*wasm.GlobalInstance) {
	for _, g := range globals {
		if g.Type.ValType == wasm.ValueTypeFuncref {
			if int64(g.Val) == wasm.GlobalInstanceNullFuncRefValue {
				g.Val = 0 // Null funcref is expressed as zero.
			} else {
				// Lowers the stored function index into the interpreter specific function's opaque pointer.
				g.Val = uint64(uintptr(unsafe.Pointer(me.functions[g.Val])))
			}
		}
	}
}

// Call implements the same method as documented on wasm.ModuleEngine.
func (me *moduleEngine) Call(ctx context.Context, m *wasm.CallContext, f *wasm.FunctionInstance, params ...uint64) (results []uint64, err error) {
	// Note: The input parameters are pre-validated, so a compiled function is only absent on close. Updates to
	// code on close aren't locked, neither is this read.
	compiled := me.functions[f.Idx]
	if compiled == nil { // Lazy check the cause as it could be because the module was already closed.
		if err = m.FailIfClosed(); err == nil {
			panic(fmt.Errorf("BUG: %s.codes[%d] was nil before close", me.name, f.Idx))
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
			err = m.FailIfClosed()
		}
		// TODO: ^^ Will not fail if the function was imported from a closed module.

		if v := recover(); v != nil {
			builder := wasmdebug.NewErrorBuilder()
			frameCount := len(ce.frames)
			for i := 0; i < frameCount; i++ {
				frame := ce.popFrame()
				fn := frame.f.source
				builder.AddFrame(fn.DebugName, fn.ParamTypes(), fn.ResultTypes())
			}
			err = builder.FromRecovered(v)
		}
	}()

	if f.Kind == wasm.FunctionKindWasm {
		if f.FunctionListener != nil {
			ctx = f.FunctionListener.Before(ctx, params)
		}
		for _, param := range params {
			ce.pushValue(param)
		}
		ce.callNativeFunc(ctx, m, compiled)
		results = wasm.PopValues(len(f.Type.Results), ce.popValue)
		if f.FunctionListener != nil {
			// TODO: This doesn't get the error due to use of panic to propagate them.
			f.FunctionListener.After(ctx, nil, results)
		}
	} else {
		results = ce.callGoFunc(ctx, m, compiled, params)
	}
	return
}

func (ce *callEngine) callGoFunc(ctx context.Context, callCtx *wasm.CallContext, f *function, params []uint64) (results []uint64) {
	if len(ce.frames) > 0 {
		// Use the caller's memory, which might be different from the defining module on an imported function.
		callCtx = callCtx.WithMemory(ce.frames[len(ce.frames)-1].f.source.Module.Memory)
	}
	if f.source.FunctionListener != nil {
		ctx = f.source.FunctionListener.Before(ctx, params)
	}
	frame := &callFrame{f: f}
	ce.pushFrame(frame)
	results = wasm.CallGoFunc(ctx, callCtx, f.source, params)
	ce.popFrame()
	if f.source.FunctionListener != nil {
		// TODO: This doesn't get the error due to use of panic to propagate them.
		f.source.FunctionListener.After(ctx, nil, results)
	}
	return
}

func (ce *callEngine) callNativeFunc(ctx context.Context, callCtx *wasm.CallContext, f *function) {
	frame := &callFrame{f: f}
	moduleInst := f.source.Module
	memoryInst := moduleInst.Memory
	globals := moduleInst.Globals
	tables := moduleInst.Tables
	typeIDs := f.source.Module.TypeIDs
	functions := f.source.Module.Engine.(*moduleEngine).functions
	dataInstances := f.source.Module.DataInstances
	elementInstances := f.source.Module.ElementInstances
	listener := f.source.FunctionListener
	ce.pushFrame(frame)
	bodyLen := uint64(len(frame.f.body))
	for frame.pc < bodyLen {
		op := frame.f.body[frame.pc]
		// TODO: add description of each operation/case
		// on, for example, how many args are used,
		// how the stack is modified, etc.
		switch op.kind {
		case wazeroir.OperationKindUnreachable:
			panic(wasmruntime.ErrRuntimeUnreachable)
		case wazeroir.OperationKindBr:
			{
				frame.pc = op.us[0]
			}
		case wazeroir.OperationKindBrIf:
			{
				if ce.popValue() > 0 {
					ce.drop(op.rs[0])
					frame.pc = op.us[0]
				} else {
					ce.drop(op.rs[1])
					frame.pc = op.us[1]
				}
			}
		case wazeroir.OperationKindBrTable:
			{
				if v := uint64(ce.popValue()); v < uint64(len(op.us)-1) {
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
				f := functions[op.us[0]]
				if f.hostFn != nil {
					ce.callGoFuncWithStack(ctx, callCtx, f)
				} else if listener != nil {
					ctx = ce.callNativeFuncWithListener(ctx, callCtx, f, listener)
				} else {
					ce.callNativeFunc(ctx, callCtx, f)
				}
				frame.pc++
			}
		case wazeroir.OperationKindCallIndirect:
			{
				offset := ce.popValue()
				table := tables[op.us[1]]
				if offset >= uint64(len(table.References)) {
					panic(wasmruntime.ErrRuntimeInvalidTableAccess)
				}
				rawPtr := table.References[offset]
				if rawPtr == 0 {
					panic(wasmruntime.ErrRuntimeInvalidTableAccess)
				}

				tf := functionFromUintptr(rawPtr)
				if tf.source.TypeID != typeIDs[op.us[0]] {
					panic(wasmruntime.ErrRuntimeIndirectCallTypeMismatch)
				}

				// Call in.
				if tf.hostFn != nil {
					ce.callGoFuncWithStack(ctx, callCtx, tf)
				} else if listener != nil {
					ctx = ce.callNativeFuncWithListener(ctx, callCtx, f, listener)
				} else {
					ce.callNativeFunc(ctx, callCtx, tf)
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
				c := ce.popValue()
				v2 := ce.popValue()
				if c == 0 {
					_ = ce.popValue()
					ce.pushValue(v2)
				}
				frame.pc++
			}
		case wazeroir.OperationKindPick:
			{
				ce.pushValue(ce.stack[len(ce.stack)-1-int(op.us[0])])
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
				g := globals[op.us[0]] // TODO: Not yet traceable as it doesn't use the types in global.go
				ce.pushValue(g.Val)
				frame.pc++
			}
		case wazeroir.OperationKindGlobalSet:
			{
				g := globals[op.us[0]] // TODO: Not yet traceable as it doesn't use the types in global.go
				g.Val = ce.popValue()
				frame.pc++
			}
		case wazeroir.OperationKindLoad:
			{
				offset := ce.popMemoryOffset(op)
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeF32:
					if val, ok := memoryInst.ReadUint32Le(ctx, offset); !ok {
						panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
					} else {
						ce.pushValue(uint64(val))
					}
				case wazeroir.UnsignedTypeI64, wazeroir.UnsignedTypeF64:
					if val, ok := memoryInst.ReadUint64Le(ctx, offset); !ok {
						panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
					} else {
						ce.pushValue(val)
					}
				}
				frame.pc++
			}
		case wazeroir.OperationKindLoad8:
			{
				val, ok := memoryInst.ReadByte(ctx, ce.popMemoryOffset(op))
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}

				switch wazeroir.SignedInt(op.b1) {
				case wazeroir.SignedInt32, wazeroir.SignedInt64:
					ce.pushValue(uint64(int8(val)))
				case wazeroir.SignedUint32, wazeroir.SignedUint64:
					ce.pushValue(uint64(val))
				}
				frame.pc++
			}
		case wazeroir.OperationKindLoad16:
			{
				val, ok := memoryInst.ReadUint16Le(ctx, ce.popMemoryOffset(op))
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}

				switch wazeroir.SignedInt(op.b1) {
				case wazeroir.SignedInt32, wazeroir.SignedInt64:
					ce.pushValue(uint64(int16(val)))
				case wazeroir.SignedUint32, wazeroir.SignedUint64:
					ce.pushValue(uint64(val))
				}
				frame.pc++
			}
		case wazeroir.OperationKindLoad32:
			{
				val, ok := memoryInst.ReadUint32Le(ctx, ce.popMemoryOffset(op))
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}

				if op.b1 == 1 { // Signed
					ce.pushValue(uint64(int32(val)))
				} else {
					ce.pushValue(uint64(val))
				}
				frame.pc++
			}
		case wazeroir.OperationKindStore:
			{
				val := ce.popValue()
				offset := ce.popMemoryOffset(op)
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeF32:
					if !memoryInst.WriteUint32Le(ctx, offset, uint32(val)) {
						panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
					}
				case wazeroir.UnsignedTypeI64, wazeroir.UnsignedTypeF64:
					if !memoryInst.WriteUint64Le(ctx, offset, val) {
						panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
					}
				}
				frame.pc++
			}
		case wazeroir.OperationKindStore8:
			{
				val := byte(ce.popValue())
				offset := ce.popMemoryOffset(op)
				if !memoryInst.WriteByte(ctx, offset, val) {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				frame.pc++
			}
		case wazeroir.OperationKindStore16:
			{
				val := uint16(ce.popValue())
				offset := ce.popMemoryOffset(op)
				if !memoryInst.WriteUint16Le(ctx, offset, val) {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				frame.pc++
			}
		case wazeroir.OperationKindStore32:
			{
				val := uint32(ce.popValue())
				offset := ce.popMemoryOffset(op)
				if !memoryInst.WriteUint32Le(ctx, offset, val) {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				frame.pc++
			}
		case wazeroir.OperationKindMemorySize:
			{
				ce.pushValue(uint64(memoryInst.PageSize(ctx)))
				frame.pc++
			}
		case wazeroir.OperationKindMemoryGrow:
			{
				n := ce.popValue()
				if res, ok := memoryInst.Grow(ctx, uint32(n)); !ok {
					ce.pushValue(uint64(0xffffffff)) // = -1 in signed 32-bit integer.
				} else {
					ce.pushValue(uint64(res))
				}
				frame.pc++
			}
		case wazeroir.OperationKindConstI32, wazeroir.OperationKindConstI64,
			wazeroir.OperationKindConstF32, wazeroir.OperationKindConstF64:
			{
				ce.pushValue(op.us[0])
				frame.pc++
			}
		case wazeroir.OperationKindEq:
			{
				var b bool
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeI64:
					v2, v1 := ce.popValue(), ce.popValue()
					b = v1 == v2
				case wazeroir.UnsignedTypeF32:
					v2, v1 := ce.popValue(), ce.popValue()
					b = math.Float32frombits(uint32(v2)) == math.Float32frombits(uint32(v1))
				case wazeroir.UnsignedTypeF64:
					v2, v1 := ce.popValue(), ce.popValue()
					b = math.Float64frombits(v2) == math.Float64frombits(v1)
				}
				if b {
					ce.pushValue(1)
				} else {
					ce.pushValue(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindNe:
			{
				var b bool
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeI64:
					v2, v1 := ce.popValue(), ce.popValue()
					b = v1 != v2
				case wazeroir.UnsignedTypeF32:
					v2, v1 := ce.popValue(), ce.popValue()
					b = math.Float32frombits(uint32(v2)) != math.Float32frombits(uint32(v1))
				case wazeroir.UnsignedTypeF64:
					v2, v1 := ce.popValue(), ce.popValue()
					b = math.Float64frombits(v2) != math.Float64frombits(v1)
				}
				if b {
					ce.pushValue(1)
				} else {
					ce.pushValue(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindEqz:
			{
				if ce.popValue() == 0 {
					ce.pushValue(1)
				} else {
					ce.pushValue(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindLt:
			{
				v2 := ce.popValue()
				v1 := ce.popValue()
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
					ce.pushValue(1)
				} else {
					ce.pushValue(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindGt:
			{
				v2 := ce.popValue()
				v1 := ce.popValue()
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
					ce.pushValue(1)
				} else {
					ce.pushValue(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindLe:
			{
				v2 := ce.popValue()
				v1 := ce.popValue()
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
					ce.pushValue(1)
				} else {
					ce.pushValue(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindGe:
			{
				v2 := ce.popValue()
				v1 := ce.popValue()
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
					ce.pushValue(1)
				} else {
					ce.pushValue(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindAdd:
			{
				v2 := ce.popValue()
				v1 := ce.popValue()
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32:
					v := uint32(v1) + uint32(v2)
					ce.pushValue(uint64(v))
				case wazeroir.UnsignedTypeI64:
					ce.pushValue(v1 + v2)
				case wazeroir.UnsignedTypeF32:
					v := math.Float32frombits(uint32(v1)) + math.Float32frombits(uint32(v2))
					ce.pushValue(uint64(math.Float32bits(v)))
				case wazeroir.UnsignedTypeF64:
					v := math.Float64frombits(v1) + math.Float64frombits(v2)
					ce.pushValue(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindSub:
			{
				v2 := ce.popValue()
				v1 := ce.popValue()
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32:
					ce.pushValue(uint64(uint32(v1) - uint32(v2)))
				case wazeroir.UnsignedTypeI64:
					ce.pushValue(v1 - v2)
				case wazeroir.UnsignedTypeF32:
					v := math.Float32frombits(uint32(v1)) - math.Float32frombits(uint32(v2))
					ce.pushValue(uint64(math.Float32bits(v)))
				case wazeroir.UnsignedTypeF64:
					v := math.Float64frombits(v1) - math.Float64frombits(v2)
					ce.pushValue(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindMul:
			{
				v2 := ce.popValue()
				v1 := ce.popValue()
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32:
					ce.pushValue(uint64(uint32(v1) * uint32(v2)))
				case wazeroir.UnsignedTypeI64:
					ce.pushValue(v1 * v2)
				case wazeroir.UnsignedTypeF32:
					v := math.Float32frombits(uint32(v2)) * math.Float32frombits(uint32(v1))
					ce.pushValue(uint64(math.Float32bits(v)))
				case wazeroir.UnsignedTypeF64:
					v := math.Float64frombits(v2) * math.Float64frombits(v1)
					ce.pushValue(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindClz:
			{
				v := ce.popValue()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.pushValue(uint64(bits.LeadingZeros32(uint32(v))))
				} else {
					// UnsignedInt64
					ce.pushValue(uint64(bits.LeadingZeros64(v)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindCtz:
			{
				v := ce.popValue()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.pushValue(uint64(bits.TrailingZeros32(uint32(v))))
				} else {
					// UnsignedInt64
					ce.pushValue(uint64(bits.TrailingZeros64(v)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindPopcnt:
			{
				v := ce.popValue()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.pushValue(uint64(bits.OnesCount32(uint32(v))))
				} else {
					// UnsignedInt64
					ce.pushValue(uint64(bits.OnesCount64(v)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindDiv:
			{
				// If an integer, check we won't divide by zero.
				t := wazeroir.SignedType(op.b1)
				v2, v1 := ce.popValue(), ce.popValue()
				switch t {
				case wazeroir.SignedTypeFloat32, wazeroir.SignedTypeFloat64: // not integers
				default:
					if v2 == 0 {
						panic(wasmruntime.ErrRuntimeIntegerDivideByZero)
					}
				}

				switch t {
				case wazeroir.SignedTypeInt32:
					d := int32(v2)
					n := int32(v1)
					if n == math.MinInt32 && d == -1 {
						panic(wasmruntime.ErrRuntimeIntegerOverflow)
					}
					ce.pushValue(uint64(uint32(n / d)))
				case wazeroir.SignedTypeInt64:
					d := int64(v2)
					n := int64(v1)
					if n == math.MinInt64 && d == -1 {
						panic(wasmruntime.ErrRuntimeIntegerOverflow)
					}
					ce.pushValue(uint64(n / d))
				case wazeroir.SignedTypeUint32:
					d := uint32(v2)
					n := uint32(v1)
					ce.pushValue(uint64(n / d))
				case wazeroir.SignedTypeUint64:
					d := v2
					n := v1
					ce.pushValue(n / d)
				case wazeroir.SignedTypeFloat32:
					d := v2
					n := v1
					v := math.Float32frombits(uint32(n)) / math.Float32frombits(uint32(d))
					ce.pushValue(uint64(math.Float32bits(v)))
				case wazeroir.SignedTypeFloat64:
					d := v2
					n := v1
					v := math.Float64frombits(n) / math.Float64frombits(d)
					ce.pushValue(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindRem:
			{
				v2, v1 := ce.popValue(), ce.popValue()
				if v2 == 0 {
					panic(wasmruntime.ErrRuntimeIntegerDivideByZero)
				}
				switch wazeroir.SignedInt(op.b1) {
				case wazeroir.SignedInt32:
					d := int32(v2)
					n := int32(v1)
					ce.pushValue(uint64(uint32(n % d)))
				case wazeroir.SignedInt64:
					d := int64(v2)
					n := int64(v1)
					ce.pushValue(uint64(n % d))
				case wazeroir.SignedUint32:
					d := uint32(v2)
					n := uint32(v1)
					ce.pushValue(uint64(n % d))
				case wazeroir.SignedUint64:
					d := v2
					n := v1
					ce.pushValue(n % d)
				}
				frame.pc++
			}
		case wazeroir.OperationKindAnd:
			{
				v2 := ce.popValue()
				v1 := ce.popValue()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.pushValue(uint64(uint32(v2) & uint32(v1)))
				} else {
					// UnsignedInt64
					ce.pushValue(uint64(v2 & v1))
				}
				frame.pc++
			}
		case wazeroir.OperationKindOr:
			{
				v2 := ce.popValue()
				v1 := ce.popValue()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.pushValue(uint64(uint32(v2) | uint32(v1)))
				} else {
					// UnsignedInt64
					ce.pushValue(uint64(v2 | v1))
				}
				frame.pc++
			}
		case wazeroir.OperationKindXor:
			{
				v2 := ce.popValue()
				v1 := ce.popValue()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.pushValue(uint64(uint32(v2) ^ uint32(v1)))
				} else {
					// UnsignedInt64
					ce.pushValue(uint64(v2 ^ v1))
				}
				frame.pc++
			}
		case wazeroir.OperationKindShl:
			{
				v2 := ce.popValue()
				v1 := ce.popValue()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.pushValue(uint64(uint32(v1) << (uint32(v2) % 32)))
				} else {
					// UnsignedInt64
					ce.pushValue(v1 << (v2 % 64))
				}
				frame.pc++
			}
		case wazeroir.OperationKindShr:
			{
				v2 := ce.popValue()
				v1 := ce.popValue()
				switch wazeroir.SignedInt(op.b1) {
				case wazeroir.SignedInt32:
					ce.pushValue(uint64(int32(v1) >> (uint32(v2) % 32)))
				case wazeroir.SignedInt64:
					ce.pushValue(uint64(int64(v1) >> (v2 % 64)))
				case wazeroir.SignedUint32:
					ce.pushValue(uint64(uint32(v1) >> (uint32(v2) % 32)))
				case wazeroir.SignedUint64:
					ce.pushValue(v1 >> (v2 % 64))
				}
				frame.pc++
			}
		case wazeroir.OperationKindRotl:
			{
				v2 := ce.popValue()
				v1 := ce.popValue()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.pushValue(uint64(bits.RotateLeft32(uint32(v1), int(v2))))
				} else {
					// UnsignedInt64
					ce.pushValue(uint64(bits.RotateLeft64(v1, int(v2))))
				}
				frame.pc++
			}
		case wazeroir.OperationKindRotr:
			{
				v2 := ce.popValue()
				v1 := ce.popValue()
				if op.b1 == 0 {
					// UnsignedInt32
					ce.pushValue(uint64(bits.RotateLeft32(uint32(v1), -int(v2))))
				} else {
					// UnsignedInt64
					ce.pushValue(uint64(bits.RotateLeft64(v1, -int(v2))))
				}
				frame.pc++
			}
		case wazeroir.OperationKindAbs:
			{
				if op.b1 == 0 {
					// Float32
					const mask uint32 = 1 << 31
					ce.pushValue(uint64(uint32(ce.popValue()) &^ mask))
				} else {
					// Float64
					const mask uint64 = 1 << 63
					ce.pushValue(uint64(ce.popValue() &^ mask))
				}
				frame.pc++
			}
		case wazeroir.OperationKindNeg:
			{
				if op.b1 == 0 {
					// Float32
					v := -math.Float32frombits(uint32(ce.popValue()))
					ce.pushValue(uint64(math.Float32bits(v)))
				} else {
					// Float64
					v := -math.Float64frombits(ce.popValue())
					ce.pushValue(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindCeil:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Ceil(float64(math.Float32frombits(uint32(ce.popValue()))))
					ce.pushValue(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Ceil(float64(math.Float64frombits(ce.popValue())))
					ce.pushValue(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindFloor:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Floor(float64(math.Float32frombits(uint32(ce.popValue()))))
					ce.pushValue(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Floor(float64(math.Float64frombits(ce.popValue())))
					ce.pushValue(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindTrunc:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Trunc(float64(math.Float32frombits(uint32(ce.popValue()))))
					ce.pushValue(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Trunc(float64(math.Float64frombits(ce.popValue())))
					ce.pushValue(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindNearest:
			{
				if op.b1 == 0 {
					// Float32
					f := math.Float32frombits(uint32(ce.popValue()))
					ce.pushValue(uint64(math.Float32bits(moremath.WasmCompatNearestF32(f))))
				} else {
					// Float64
					f := math.Float64frombits(ce.popValue())
					ce.pushValue(math.Float64bits(moremath.WasmCompatNearestF64(f)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindSqrt:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Sqrt(float64(math.Float32frombits(uint32(ce.popValue()))))
					ce.pushValue(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Sqrt(float64(math.Float64frombits(ce.popValue())))
					ce.pushValue(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindMin:
			{
				if op.b1 == 0 {
					// Float32
					v2 := math.Float32frombits(uint32(ce.popValue()))
					v1 := math.Float32frombits(uint32(ce.popValue()))
					ce.pushValue(uint64(math.Float32bits(float32(moremath.WasmCompatMin(float64(v1), float64(v2))))))
				} else {
					v2 := math.Float64frombits(ce.popValue())
					v1 := math.Float64frombits(ce.popValue())
					ce.pushValue(math.Float64bits(moremath.WasmCompatMin(v1, v2)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindMax:
			{

				if op.b1 == 0 {
					// Float32
					v2 := math.Float32frombits(uint32(ce.popValue()))
					v1 := math.Float32frombits(uint32(ce.popValue()))
					ce.pushValue(uint64(math.Float32bits(float32(moremath.WasmCompatMax(float64(v1), float64(v2))))))
				} else {
					// Float64
					v2 := math.Float64frombits(ce.popValue())
					v1 := math.Float64frombits(ce.popValue())
					ce.pushValue(math.Float64bits(moremath.WasmCompatMax(v1, v2)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindCopysign:
			{
				if op.b1 == 0 {
					// Float32
					v2 := uint32(ce.popValue())
					v1 := uint32(ce.popValue())
					const signbit = 1 << 31
					ce.pushValue(uint64(v1&^signbit | v2&signbit))
				} else {
					// Float64
					v2 := ce.popValue()
					v1 := ce.popValue()
					const signbit = 1 << 63
					ce.pushValue(v1&^signbit | v2&signbit)
				}
				frame.pc++
			}
		case wazeroir.OperationKindI32WrapFromI64:
			{
				ce.pushValue(uint64(uint32(ce.popValue())))
				frame.pc++
			}
		case wazeroir.OperationKindITruncFromF:
			{
				if op.b1 == 0 {
					// Float32
					switch wazeroir.SignedInt(op.b2) {
					case wazeroir.SignedInt32:
						v := math.Trunc(float64(math.Float32frombits(uint32(ce.popValue()))))
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							if op.b3 {
								// non-trapping conversion must cast nan to zero.
								v = 0
							} else {
								panic(wasmruntime.ErrRuntimeInvalidConversionToInteger)
							}
						} else if v < math.MinInt32 || v > math.MaxInt32 {
							if op.b3 {
								// non-trapping conversion must "saturate" the value for overflowing sources.
								if v < 0 {
									v = math.MinInt32
								} else {
									v = math.MaxInt32
								}
							} else {
								panic(wasmruntime.ErrRuntimeIntegerOverflow)
							}
						}
						ce.pushValue(uint64(uint32(int32(v))))
					case wazeroir.SignedInt64:
						v := math.Trunc(float64(math.Float32frombits(uint32(ce.popValue()))))
						res := int64(v)
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							if op.b3 {
								// non-trapping conversion must cast nan to zero.
								res = 0
							} else {
								panic(wasmruntime.ErrRuntimeInvalidConversionToInteger)
							}
						} else if v < math.MinInt64 || v >= math.MaxInt64 {
							// Note: math.MaxInt64 is rounded up to math.MaxInt64+1 in 64-bit float representation,
							// and that's why we use '>=' not '>' to check overflow.
							if op.b3 {
								// non-trapping conversion must "saturate" the value for overflowing sources.
								if v < 0 {
									res = math.MinInt64
								} else {
									res = math.MaxInt64
								}
							} else {
								panic(wasmruntime.ErrRuntimeIntegerOverflow)
							}
						}
						ce.pushValue(uint64(res))
					case wazeroir.SignedUint32:
						v := math.Trunc(float64(math.Float32frombits(uint32(ce.popValue()))))
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							if op.b3 {
								// non-trapping conversion must cast nan to zero.
								v = 0
							} else {
								panic(wasmruntime.ErrRuntimeInvalidConversionToInteger)
							}
						} else if v < 0 || v > math.MaxUint32 {
							if op.b3 {
								// non-trapping conversion must "saturate" the value for overflowing source.
								if v < 0 {
									v = 0
								} else {
									v = math.MaxUint32
								}
							} else {
								panic(wasmruntime.ErrRuntimeIntegerOverflow)
							}
						}
						ce.pushValue(uint64(uint32(v)))
					case wazeroir.SignedUint64:
						v := math.Trunc(float64(math.Float32frombits(uint32(ce.popValue()))))
						res := uint64(v)
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							if op.b3 {
								// non-trapping conversion must cast nan to zero.
								res = 0
							} else {
								panic(wasmruntime.ErrRuntimeInvalidConversionToInteger)
							}
						} else if v < 0 || v >= math.MaxUint64 {
							// Note: math.MaxUint64 is rounded up to math.MaxUint64+1 in 64-bit float representation,
							// and that's why we use '>=' not '>' to check overflow.
							if op.b3 {
								// non-trapping conversion must "saturate" the value for overflowing source.
								if v < 0 {
									res = 0
								} else {
									res = math.MaxUint64
								}
							} else {
								panic(wasmruntime.ErrRuntimeIntegerOverflow)
							}
						}
						ce.pushValue(res)
					}
				} else {
					// Float64
					switch wazeroir.SignedInt(op.b2) {
					case wazeroir.SignedInt32:
						v := math.Trunc(math.Float64frombits(ce.popValue()))
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							if op.b3 {
								// non-trapping conversion must cast nan to zero.
								v = 0
							} else {
								panic(wasmruntime.ErrRuntimeInvalidConversionToInteger)
							}
						} else if v < math.MinInt32 || v > math.MaxInt32 {
							if op.b3 {
								// non-trapping conversion must "saturate" the value for overflowing source.
								if v < 0 {
									v = math.MinInt32
								} else {
									v = math.MaxInt32
								}
							} else {
								panic(wasmruntime.ErrRuntimeIntegerOverflow)
							}
						}
						ce.pushValue(uint64(uint32(int32(v))))
					case wazeroir.SignedInt64:
						v := math.Trunc(math.Float64frombits(ce.popValue()))
						res := int64(v)
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							if op.b3 {
								// non-trapping conversion must cast nan to zero.
								res = 0
							} else {
								panic(wasmruntime.ErrRuntimeInvalidConversionToInteger)
							}
						} else if v < math.MinInt64 || v >= math.MaxInt64 {
							// Note: math.MaxInt64 is rounded up to math.MaxInt64+1 in 64-bit float representation,
							// and that's why we use '>=' not '>' to check overflow.
							if op.b3 {
								// non-trapping conversion must "saturate" the value for overflowing source.
								if v < 0 {
									res = math.MinInt64
								} else {
									res = math.MaxInt64
								}
							} else {
								panic(wasmruntime.ErrRuntimeIntegerOverflow)
							}
						}
						ce.pushValue(uint64(res))
					case wazeroir.SignedUint32:
						v := math.Trunc(math.Float64frombits(ce.popValue()))
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							if op.b3 {
								// non-trapping conversion must cast nan to zero.
								v = 0
							} else {
								panic(wasmruntime.ErrRuntimeInvalidConversionToInteger)
							}
						} else if v < 0 || v > math.MaxUint32 {
							if op.b3 {
								// non-trapping conversion must "saturate" the value for overflowing source.
								if v < 0 {
									v = 0
								} else {
									v = math.MaxUint32
								}
							} else {
								panic(wasmruntime.ErrRuntimeIntegerOverflow)
							}
						}
						ce.pushValue(uint64(uint32(v)))
					case wazeroir.SignedUint64:
						v := math.Trunc(math.Float64frombits(ce.popValue()))
						res := uint64(v)
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							if op.b3 {
								// non-trapping conversion must cast nan to zero.
								res = 0
							} else {
								panic(wasmruntime.ErrRuntimeInvalidConversionToInteger)
							}
						} else if v < 0 || v >= math.MaxUint64 {
							// Note: math.MaxUint64 is rounded up to math.MaxUint64+1 in 64-bit float representation,
							// and that's why we use '>=' not '>' to check overflow.
							if op.b3 {
								// non-trapping conversion must "saturate" the value for overflowing source.
								if v < 0 {
									res = 0
								} else {
									res = math.MaxUint64
								}
							} else {
								panic(wasmruntime.ErrRuntimeIntegerOverflow)
							}
						}
						ce.pushValue(res)
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
						v := float32(int32(ce.popValue()))
						ce.pushValue(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(int32(ce.popValue()))
						ce.pushValue(math.Float64bits(v))
					}
				case wazeroir.SignedInt64:
					if op.b2 == 0 {
						// Float32
						v := float32(int64(ce.popValue()))
						ce.pushValue(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(int64(ce.popValue()))
						ce.pushValue(math.Float64bits(v))
					}
				case wazeroir.SignedUint32:
					if op.b2 == 0 {
						// Float32
						v := float32(uint32(ce.popValue()))
						ce.pushValue(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(uint32(ce.popValue()))
						ce.pushValue(math.Float64bits(v))
					}
				case wazeroir.SignedUint64:
					if op.b2 == 0 {
						// Float32
						v := float32(ce.popValue())
						ce.pushValue(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(ce.popValue())
						ce.pushValue(math.Float64bits(v))
					}
				}
				frame.pc++
			}
		case wazeroir.OperationKindF32DemoteFromF64:
			{
				v := float32(math.Float64frombits(ce.popValue()))
				ce.pushValue(uint64(math.Float32bits(v)))
				frame.pc++
			}
		case wazeroir.OperationKindF64PromoteFromF32:
			{
				v := float64(math.Float32frombits(uint32(ce.popValue())))
				ce.pushValue(math.Float64bits(v))
				frame.pc++
			}
		case wazeroir.OperationKindExtend:
			{
				if op.b1 == 1 {
					// Signed.
					v := int64(int32(ce.popValue()))
					ce.pushValue(uint64(v))
				} else {
					v := uint64(uint32(ce.popValue()))
					ce.pushValue(v)
				}
				frame.pc++
			}

		case wazeroir.OperationKindSignExtend32From8:
			{
				v := int32(int8(ce.popValue()))
				ce.pushValue(uint64(v))
				frame.pc++
			}
		case wazeroir.OperationKindSignExtend32From16:
			{
				v := int32(int16(ce.popValue()))
				ce.pushValue(uint64(v))
				frame.pc++
			}
		case wazeroir.OperationKindSignExtend64From8:
			{
				v := int64(int8(ce.popValue()))
				ce.pushValue(uint64(v))
				frame.pc++
			}
		case wazeroir.OperationKindSignExtend64From16:
			{
				v := int64(int16(ce.popValue()))
				ce.pushValue(uint64(v))
				frame.pc++
			}
		case wazeroir.OperationKindSignExtend64From32:
			{
				v := int64(int32(ce.popValue()))
				ce.pushValue(uint64(v))
				frame.pc++
			}
		case wazeroir.OperationKindMemoryInit:
			dataInstance := dataInstances[op.us[0]]
			copySize := ce.popValue()
			inDataOffset := ce.popValue()
			inMemoryOffset := ce.popValue()
			if inDataOffset+copySize > uint64(len(dataInstance)) ||
				inMemoryOffset+copySize > uint64(len(memoryInst.Buffer)) {
				panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
			} else if copySize != 0 {
				copy(memoryInst.Buffer[inMemoryOffset:inMemoryOffset+copySize], dataInstance[inDataOffset:])
			}
			frame.pc++
		case wazeroir.OperationKindDataDrop:
			dataInstances[op.us[0]] = nil
			frame.pc++
		case wazeroir.OperationKindMemoryCopy:
			memLen := uint64(len(memoryInst.Buffer))
			copySize := ce.popValue()
			sourceOffset := ce.popValue()
			destinationOffset := ce.popValue()
			if sourceOffset+copySize > memLen || destinationOffset+copySize > memLen {
				panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
			} else if copySize != 0 {
				copy(memoryInst.Buffer[destinationOffset:],
					memoryInst.Buffer[sourceOffset:sourceOffset+copySize])
			}
			frame.pc++
		case wazeroir.OperationKindMemoryFill:
			fillSize := ce.popValue()
			value := byte(ce.popValue())
			offset := ce.popValue()
			if fillSize+offset > uint64(len(memoryInst.Buffer)) {
				panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
			} else if fillSize != 0 {
				// Uses the copy trick for faster filling buffer.
				// https://gist.github.com/taylorza/df2f89d5f9ab3ffd06865062a4cf015d
				buf := memoryInst.Buffer[offset : offset+fillSize]
				buf[0] = value
				for i := 1; i < len(buf); i *= 2 {
					copy(buf[i:], buf[:i])
				}
			}
			frame.pc++
		case wazeroir.OperationKindTableInit:
			elementInstance := elementInstances[op.us[0]]
			copySize := ce.popValue()
			inElementOffset := ce.popValue()
			inTableOffset := ce.popValue()
			table := tables[op.us[1]]
			if inElementOffset+copySize > uint64(len(elementInstance.References)) ||
				inTableOffset+copySize > uint64(len(table.References)) {
				panic(wasmruntime.ErrRuntimeInvalidTableAccess)
			} else if copySize != 0 {
				copy(table.References[inTableOffset:inTableOffset+copySize], elementInstance.References[inElementOffset:])
			}
			frame.pc++
		case wazeroir.OperationKindElemDrop:
			elementInstances[op.us[0]].References = nil
			frame.pc++
		case wazeroir.OperationKindTableCopy:
			srcTable, dstTable := tables[op.us[0]].References, tables[op.us[1]].References
			copySize := ce.popValue()
			sourceOffset := ce.popValue()
			destinationOffset := ce.popValue()
			if sourceOffset+copySize > uint64(len(srcTable)) || destinationOffset+copySize > uint64(len(dstTable)) {
				panic(wasmruntime.ErrRuntimeInvalidTableAccess)
			} else if copySize != 0 {
				copy(dstTable[destinationOffset:], srcTable[sourceOffset:sourceOffset+copySize])
			}
			frame.pc++
		case wazeroir.OperationKindRefFunc:
			ce.pushValue(uint64(uintptr(unsafe.Pointer(functions[op.us[0]]))))
			frame.pc++
		case wazeroir.OperationKindTableGet:
			table := tables[op.us[0]]

			offset := ce.popValue()
			if offset > uint64(len(table.References)) {
				panic(wasmruntime.ErrRuntimeInvalidTableAccess)
			}

			ce.pushValue(uint64(table.References[offset]))
			frame.pc++
		case wazeroir.OperationKindTableSet:
			table := tables[op.us[0]]
			ref := ce.popValue()

			offset := ce.popValue()
			if offset >= uint64(len(table.References)) {
				panic(wasmruntime.ErrRuntimeInvalidTableAccess)
			}

			table.References[offset] = uintptr(ref) // externrefs are opaque uint64.
			frame.pc++
		case wazeroir.OperationKindTableSize:
			table := tables[op.us[0]]
			ce.pushValue(uint64(len(table.References)))
			frame.pc++
		case wazeroir.OperationKindTableGrow:
			table := tables[op.us[0]]
			num, ref := ce.popValue(), ce.popValue()
			ret := table.Grow(ctx, uint32(num), uintptr(ref))
			ce.pushValue(uint64(ret))
			frame.pc++
		case wazeroir.OperationKindTableFill:
			table := tables[op.us[0]]
			num := ce.popValue()
			ref := uintptr(ce.popValue())
			offset := ce.popValue()
			if num+offset > uint64(len(table.References)) {
				panic(wasmruntime.ErrRuntimeInvalidTableAccess)
			} else if num > 0 {
				// Uses the copy trick for faster filling the region with the value.
				// https://gist.github.com/taylorza/df2f89d5f9ab3ffd06865062a4cf015d
				targetRegion := table.References[offset : offset+num]
				targetRegion[0] = ref
				for i := 1; i < len(targetRegion); i *= 2 {
					copy(targetRegion[i:], targetRegion[:i])
				}
			}
			frame.pc++
		}
	}
	ce.popFrame()
}

func (ce *callEngine) callNativeFuncWithListener(ctx context.Context, callCtx *wasm.CallContext, f *function, fnl experimental.FunctionListener) context.Context {
	ctx = fnl.Before(ctx, ce.peekValues(len(f.source.Type.Params)))
	ce.callNativeFunc(ctx, callCtx, f)
	// TODO: This doesn't get the error due to use of panic to propagate them.
	fnl.After(ctx, nil, ce.peekValues(len(f.source.Type.Results)))
	return ctx
}

// popMemoryOffset takes a memory offset off the stack for use in load and store instructions.
// As the top of stack value is 64-bit, this ensures it is in range before returning it.
func (ce *callEngine) popMemoryOffset(op *interpreterOp) uint32 {
	// TODO: Document what 'us' is and why we expect to look at value 1.
	offset := op.us[1] + ce.popValue()
	if offset > math.MaxUint32 {
		panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
	}
	return uint32(offset)
}

func (ce *callEngine) callGoFuncWithStack(ctx context.Context, callCtx *wasm.CallContext, f *function) {
	params := wasm.PopGoFuncParams(f.source, ce.popValue)
	results := ce.callGoFunc(ctx, callCtx, f, params)
	for _, v := range results {
		ce.pushValue(v)
	}
}
