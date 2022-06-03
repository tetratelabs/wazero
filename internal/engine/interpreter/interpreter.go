package interpreter

import (
	"context"
	"encoding/binary"
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

// CompiledModuleCount implements the same method as documented on wasm.Engine.
func (e *engine) CompiledModuleCount() uint32 {
	return uint32(len(e.codes))
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
			op.b3 = o.IsTargetVector
		case *wazeroir.OperationSwap:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.Depth)
			op.b3 = o.IsTargetVector
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
		case *wazeroir.OperationV128Const:
			op.us = make([]uint64, 2)
			op.us[0] = o.Lo
			op.us[1] = o.Hi
		case *wazeroir.OperationV128Add:
			op.b1 = o.Shape
		case *wazeroir.OperationV128Sub:
			op.b1 = o.Shape
		case *wazeroir.OperationV128Load:
			op.b1 = o.Type
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offset)
		case *wazeroir.OperationV128LoadLane:
			op.b1 = o.LaneSize
			op.b2 = o.LaneIndex
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offset)
		case *wazeroir.OperationV128Store:
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offset)
		case *wazeroir.OperationV128StoreLane:
			op.b1 = o.LaneSize
			op.b2 = o.LaneIndex
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offset)
		case *wazeroir.OperationV128ExtractLane:
			op.b1 = o.Shape
			op.b2 = o.LaneIndex
			op.b3 = o.Signed
		case *wazeroir.OperationV128ReplaceLane:
			op.b1 = o.Shape
			op.b2 = o.LaneIndex
		case *wazeroir.OperationV128Splat:
			op.b1 = o.Shape
		case *wazeroir.OperationV128Shuffle:
			op.us = make([]uint64, 16)
			for i, l := range o.Lanes {
				op.us[i] = uint64(l)
			}
		case *wazeroir.OperationV128Swizzle:
		case *wazeroir.OperationV128AnyTrue:
		case *wazeroir.OperationV128AllTrue:
			op.b1 = o.Shape
		case *wazeroir.OperationV128BitMask:
			op.b1 = o.Shape
		case *wazeroir.OperationV128And:
		case *wazeroir.OperationV128Not:
		case *wazeroir.OperationV128Or:
		case *wazeroir.OperationV128Xor:
		case *wazeroir.OperationV128Bitselect:
		case *wazeroir.OperationV128AndNot:
		case *wazeroir.OperationV128Shr:
			op.b1 = o.Shape
			op.b3 = o.Signed
		case *wazeroir.OperationV128Shl:
			op.b1 = o.Shape
		case *wazeroir.OperationV128Cmp:
			op.b1 = o.Type
		default:
			panic(fmt.Errorf("BUG: unimplemented operation %s", op.kind.String()))
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

	paramSignature := f.Type.ParamNumInUint64
	paramCount := len(params)
	if paramSignature != paramCount {
		return nil, fmt.Errorf("expected %d params, but passed %d", paramSignature, paramCount)
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
		results = wasm.PopValues(f.Type.ResultNumInUint64, ce.popValue)
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
			frame.pc = op.us[0]
		case wazeroir.OperationKindBrIf:
			if ce.popValue() > 0 {
				ce.drop(op.rs[0])
				frame.pc = op.us[0]
			} else {
				ce.drop(op.rs[1])
				frame.pc = op.us[1]
			}
		case wazeroir.OperationKindBrTable:
			if v := uint64(ce.popValue()); v < uint64(len(op.us)-1) {
				ce.drop(op.rs[v+1])
				frame.pc = op.us[v+1]
			} else {
				// Default branch.
				ce.drop(op.rs[0])
				frame.pc = op.us[0]
			}
		case wazeroir.OperationKindCall:
			f := functions[op.us[0]]
			if f.hostFn != nil {
				ce.callGoFuncWithStack(ctx, callCtx, f)
			} else if listener != nil {
				ctx = ce.callNativeFuncWithListener(ctx, callCtx, f, listener)
			} else {
				ce.callNativeFunc(ctx, callCtx, f)
			}
			frame.pc++
		case wazeroir.OperationKindCallIndirect:
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
		case wazeroir.OperationKindDrop:
			ce.drop(op.rs[0])
			frame.pc++
		case wazeroir.OperationKindSelect:
			c := ce.popValue()
			v2 := ce.popValue()
			if c == 0 {
				_ = ce.popValue()
				ce.pushValue(v2)
			}
			frame.pc++
		case wazeroir.OperationKindPick:
			index := len(ce.stack) - 1 - int(op.us[0])
			ce.pushValue(ce.stack[index])
			if op.b3 { // V128 value target.
				ce.pushValue(ce.stack[index+1])
			}
			frame.pc++
		case wazeroir.OperationKindSwap:
			if op.b3 { // V128 value target.
				lowIndex := len(ce.stack) - 1 - int(op.us[0])
				ce.stack[len(ce.stack)-2], ce.stack[lowIndex] = ce.stack[lowIndex], ce.stack[len(ce.stack)-2]
				ce.stack[len(ce.stack)-1], ce.stack[lowIndex+1] = ce.stack[lowIndex+1], ce.stack[len(ce.stack)-1]
			} else {
				index := len(ce.stack) - 1 - int(op.us[0])
				ce.stack[len(ce.stack)-1], ce.stack[index] = ce.stack[index], ce.stack[len(ce.stack)-1]
			}
			frame.pc++
		case wazeroir.OperationKindGlobalGet:
			g := globals[op.us[0]]
			ce.pushValue(g.Val)
			if g.Type.ValType == wasm.ValueTypeV128 {
				ce.pushValue(g.ValHi)
			}
			frame.pc++
		case wazeroir.OperationKindGlobalSet:
			g := globals[op.us[0]]
			if g.Type.ValType == wasm.ValueTypeV128 {
				g.ValHi = ce.popValue()
			}
			g.Val = ce.popValue()
			frame.pc++
		case wazeroir.OperationKindLoad:
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
		case wazeroir.OperationKindLoad8:
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
		case wazeroir.OperationKindLoad16:
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
		case wazeroir.OperationKindLoad32:
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
		case wazeroir.OperationKindStore:
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
		case wazeroir.OperationKindStore8:
			val := byte(ce.popValue())
			offset := ce.popMemoryOffset(op)
			if !memoryInst.WriteByte(ctx, offset, val) {
				panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
			}
			frame.pc++
		case wazeroir.OperationKindStore16:
			val := uint16(ce.popValue())
			offset := ce.popMemoryOffset(op)
			if !memoryInst.WriteUint16Le(ctx, offset, val) {
				panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
			}
			frame.pc++
		case wazeroir.OperationKindStore32:
			val := uint32(ce.popValue())
			offset := ce.popMemoryOffset(op)
			if !memoryInst.WriteUint32Le(ctx, offset, val) {
				panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
			}
			frame.pc++
		case wazeroir.OperationKindMemorySize:
			ce.pushValue(uint64(memoryInst.PageSize(ctx)))
			frame.pc++
		case wazeroir.OperationKindMemoryGrow:
			n := ce.popValue()
			if res, ok := memoryInst.Grow(ctx, uint32(n)); !ok {
				ce.pushValue(uint64(0xffffffff)) // = -1 in signed 32-bit integer.
			} else {
				ce.pushValue(uint64(res))
			}
			frame.pc++
		case wazeroir.OperationKindConstI32, wazeroir.OperationKindConstI64,
			wazeroir.OperationKindConstF32, wazeroir.OperationKindConstF64:
			ce.pushValue(op.us[0])
			frame.pc++
		case wazeroir.OperationKindEq:
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
		case wazeroir.OperationKindNe:
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
		case wazeroir.OperationKindEqz:
			if ce.popValue() == 0 {
				ce.pushValue(1)
			} else {
				ce.pushValue(0)
			}
			frame.pc++
		case wazeroir.OperationKindLt:
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
		case wazeroir.OperationKindGt:
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
		case wazeroir.OperationKindLe:
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
		case wazeroir.OperationKindGe:
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
		case wazeroir.OperationKindAdd:
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
		case wazeroir.OperationKindSub:
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
		case wazeroir.OperationKindMul:
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
		case wazeroir.OperationKindClz:
			v := ce.popValue()
			if op.b1 == 0 {
				// UnsignedInt32
				ce.pushValue(uint64(bits.LeadingZeros32(uint32(v))))
			} else {
				// UnsignedInt64
				ce.pushValue(uint64(bits.LeadingZeros64(v)))
			}
			frame.pc++
		case wazeroir.OperationKindCtz:
			v := ce.popValue()
			if op.b1 == 0 {
				// UnsignedInt32
				ce.pushValue(uint64(bits.TrailingZeros32(uint32(v))))
			} else {
				// UnsignedInt64
				ce.pushValue(uint64(bits.TrailingZeros64(v)))
			}
			frame.pc++
		case wazeroir.OperationKindPopcnt:
			v := ce.popValue()
			if op.b1 == 0 {
				// UnsignedInt32
				ce.pushValue(uint64(bits.OnesCount32(uint32(v))))
			} else {
				// UnsignedInt64
				ce.pushValue(uint64(bits.OnesCount64(v)))
			}
			frame.pc++
		case wazeroir.OperationKindDiv:
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
		case wazeroir.OperationKindRem:
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
		case wazeroir.OperationKindAnd:
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
		case wazeroir.OperationKindOr:
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
		case wazeroir.OperationKindXor:
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
		case wazeroir.OperationKindShl:
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
		case wazeroir.OperationKindShr:
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
		case wazeroir.OperationKindRotl:
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
		case wazeroir.OperationKindRotr:
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
		case wazeroir.OperationKindAbs:
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
		case wazeroir.OperationKindNeg:
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
		case wazeroir.OperationKindCeil:
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
		case wazeroir.OperationKindFloor:
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
		case wazeroir.OperationKindTrunc:
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
		case wazeroir.OperationKindNearest:
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
		case wazeroir.OperationKindSqrt:
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
		case wazeroir.OperationKindMin:
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
		case wazeroir.OperationKindMax:
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
		case wazeroir.OperationKindCopysign:
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
		case wazeroir.OperationKindI32WrapFromI64:
			ce.pushValue(uint64(uint32(ce.popValue())))
			frame.pc++
		case wazeroir.OperationKindITruncFromF:
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
		case wazeroir.OperationKindFConvertFromI:
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
		case wazeroir.OperationKindF32DemoteFromF64:
			v := float32(math.Float64frombits(ce.popValue()))
			ce.pushValue(uint64(math.Float32bits(v)))
			frame.pc++
		case wazeroir.OperationKindF64PromoteFromF32:
			v := float64(math.Float32frombits(uint32(ce.popValue())))
			ce.pushValue(math.Float64bits(v))
			frame.pc++
		case wazeroir.OperationKindExtend:
			if op.b1 == 1 {
				// Signed.
				v := int64(int32(ce.popValue()))
				ce.pushValue(uint64(v))
			} else {
				v := uint64(uint32(ce.popValue()))
				ce.pushValue(v)
			}
			frame.pc++
		case wazeroir.OperationKindSignExtend32From8:
			v := int32(int8(ce.popValue()))
			ce.pushValue(uint64(v))
			frame.pc++
		case wazeroir.OperationKindSignExtend32From16:
			v := int32(int16(ce.popValue()))
			ce.pushValue(uint64(v))
			frame.pc++
		case wazeroir.OperationKindSignExtend64From8:
			v := int64(int8(ce.popValue()))
			ce.pushValue(uint64(v))
			frame.pc++
		case wazeroir.OperationKindSignExtend64From16:
			v := int64(int16(ce.popValue()))
			ce.pushValue(uint64(v))
			frame.pc++
		case wazeroir.OperationKindSignExtend64From32:
			v := int64(int32(ce.popValue()))
			ce.pushValue(uint64(v))
			frame.pc++
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
			if offset >= uint64(len(table.References)) {
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
		case wazeroir.OperationKindV128Const:
			lo, hi := op.us[0], op.us[1]
			ce.pushValue(lo)
			ce.pushValue(hi)
			frame.pc++
		case wazeroir.OperationKindV128Add:
			xHigh, xLow := ce.popValue(), ce.popValue()
			yHigh, yLow := ce.popValue(), ce.popValue()
			switch op.b1 {
			case wazeroir.ShapeI8x16:
				ce.pushValue(
					uint64(uint8(xLow>>8)+uint8(yLow>>8))<<8 | uint64(uint8(xLow)+uint8(yLow)) |
						uint64(uint8(xLow>>24)+uint8(yLow>>24))<<24 | uint64(uint8(xLow>>16)+uint8(yLow>>16))<<16 |
						uint64(uint8(xLow>>40)+uint8(yLow>>40))<<40 | uint64(uint8(xLow>>32)+uint8(yLow>>32))<<32 |
						uint64(uint8(xLow>>56)+uint8(yLow>>56))<<56 | uint64(uint8(xLow>>48)+uint8(yLow>>48))<<48,
				)
				ce.pushValue(
					uint64(uint8(xHigh>>8)+uint8(yHigh>>8))<<8 | uint64(uint8(xHigh)+uint8(yHigh)) |
						uint64(uint8(xHigh>>24)+uint8(yHigh>>24))<<24 | uint64(uint8(xHigh>>16)+uint8(yHigh>>16))<<16 |
						uint64(uint8(xHigh>>40)+uint8(yHigh>>40))<<40 | uint64(uint8(xHigh>>32)+uint8(yHigh>>32))<<32 |
						uint64(uint8(xHigh>>56)+uint8(yHigh>>56))<<56 | uint64(uint8(xHigh>>48)+uint8(yHigh>>48))<<48,
				)
			case wazeroir.ShapeI16x8:
				ce.pushValue(
					uint64(uint16(xLow>>16+yLow>>16))<<16 | uint64(uint16(xLow)+uint16(yLow)) |
						uint64(uint16(xLow>>48+yLow>>48))<<48 | uint64(uint16(xLow>>32+yLow>>32))<<32,
				)
				ce.pushValue(
					uint64(uint16(xHigh>>16)+uint16(yHigh>>16))<<16 | uint64(uint16(xHigh)+uint16(yHigh)) |
						uint64(uint16(xHigh>>48)+uint16(yHigh>>48))<<48 | uint64(uint16(xHigh>>32)+uint16(yHigh>>32))<<32,
				)
			case wazeroir.ShapeI32x4:
				ce.pushValue(uint64(uint32(xLow>>32)+uint32(yLow>>32))<<32 | uint64(uint32(xLow)+uint32(yLow)))
				ce.pushValue(uint64(uint32(xHigh>>32)+uint32(yHigh>>32))<<32 | uint64(uint32(xHigh)+uint32(yHigh)))
			case wazeroir.ShapeI64x2:
				ce.pushValue(xLow + yLow)
				ce.pushValue(xHigh + yHigh)
			}
			frame.pc++
		case wazeroir.OperationKindV128Sub:
			yHigh, yLow := ce.popValue(), ce.popValue()
			xHigh, xLow := ce.popValue(), ce.popValue()
			switch op.b1 {
			case wazeroir.ShapeI8x16:
				ce.pushValue(
					uint64(uint8(xLow>>8)-uint8(yLow>>8))<<8 | uint64(uint8(xLow)-uint8(yLow)) |
						uint64(uint8(xLow>>24)-uint8(yLow>>24))<<24 | uint64(uint8(xLow>>16)-uint8(yLow>>16))<<16 |
						uint64(uint8(xLow>>40)-uint8(yLow>>40))<<40 | uint64(uint8(xLow>>32)-uint8(yLow>>32))<<32 |
						uint64(uint8(xLow>>56)-uint8(yLow>>56))<<56 | uint64(uint8(xLow>>48)-uint8(yLow>>48))<<48,
				)
				ce.pushValue(
					uint64(uint8(xHigh>>8)-uint8(yHigh>>8))<<8 | uint64(uint8(xHigh)-uint8(yHigh)) |
						uint64(uint8(xHigh>>24)-uint8(yHigh>>24))<<24 | uint64(uint8(xHigh>>16)-uint8(yHigh>>16))<<16 |
						uint64(uint8(xHigh>>40)-uint8(yHigh>>40))<<40 | uint64(uint8(xHigh>>32)-uint8(yHigh>>32))<<32 |
						uint64(uint8(xHigh>>56)-uint8(yHigh>>56))<<56 | uint64(uint8(xHigh>>48)-uint8(yHigh>>48))<<48,
				)
			case wazeroir.ShapeI16x8:
				ce.pushValue(
					uint64(uint16(xLow>>16)-uint16(yLow>>16))<<16 | uint64(uint16(xLow)-uint16(yLow)) |
						uint64(uint16(xLow>>48)-uint16(yLow>>48))<<48 | uint64(uint16(xLow>>32)-uint16(yLow>>32))<<32,
				)
				ce.pushValue(
					uint64(uint16(xHigh>>16)-uint16(yHigh>>16))<<16 | uint64(uint16(xHigh)-uint16(yHigh)) |
						uint64(uint16(xHigh>>48)-uint16(yHigh>>48))<<48 | uint64(uint16(xHigh>>32)-uint16(yHigh>>32))<<32,
				)
			case wazeroir.ShapeI32x4:
				ce.pushValue(uint64(uint32(xLow>>32-yLow>>32))<<32 | uint64(uint32(xLow)-uint32(yLow)))
				ce.pushValue(uint64(uint32(xHigh>>32-yHigh>>32))<<32 | uint64(uint32(xHigh)-uint32(yHigh)))
			case wazeroir.ShapeI64x2:
				ce.pushValue(xLow - yLow)
				ce.pushValue(xHigh - yHigh)
			}
			frame.pc++
		case wazeroir.OperationKindV128Load:
			offset := ce.popMemoryOffset(op)
			switch op.b1 {
			case wazeroir.LoadV128Type128:
				lo, ok := memoryInst.ReadUint64Le(ctx, offset)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				ce.pushValue(lo)
				hi, ok := memoryInst.ReadUint64Le(ctx, offset+8)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				ce.pushValue(hi)
			case wazeroir.LoadV128Type8x8s:
				data, ok := memoryInst.Read(ctx, offset, 8)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				ce.pushValue(
					uint64(uint16(int8(data[3])))<<48 | uint64(uint16(int8(data[2])))<<32 | uint64(uint16(int8(data[1])))<<16 | uint64(uint16(int8(data[0]))),
				)
				ce.pushValue(
					uint64(uint16(int8(data[7])))<<48 | uint64(uint16(int8(data[6])))<<32 | uint64(uint16(int8(data[5])))<<16 | uint64(uint16(int8(data[4]))),
				)
			case wazeroir.LoadV128Type8x8u:
				data, ok := memoryInst.Read(ctx, offset, 8)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				ce.pushValue(
					uint64(data[3])<<48 | uint64(data[2])<<32 | uint64(data[1])<<16 | uint64(data[0]),
				)
				ce.pushValue(
					uint64(data[7])<<48 | uint64(data[6])<<32 | uint64(data[5])<<16 | uint64(data[4]),
				)
			case wazeroir.LoadV128Type16x4s:
				data, ok := memoryInst.Read(ctx, offset, 8)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				ce.pushValue(
					uint64(int16(binary.LittleEndian.Uint16(data[2:])))<<32 |
						uint64(uint32(int16(binary.LittleEndian.Uint16(data)))),
				)
				ce.pushValue(
					uint64(uint32(int16(binary.LittleEndian.Uint16(data[6:]))))<<32 |
						uint64(uint32(int16(binary.LittleEndian.Uint16(data[4:])))),
				)
			case wazeroir.LoadV128Type16x4u:
				data, ok := memoryInst.Read(ctx, offset, 8)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				ce.pushValue(
					uint64(binary.LittleEndian.Uint16(data[2:]))<<32 | uint64(binary.LittleEndian.Uint16(data)),
				)
				ce.pushValue(
					uint64(binary.LittleEndian.Uint16(data[6:]))<<32 | uint64(binary.LittleEndian.Uint16(data[4:])),
				)
			case wazeroir.LoadV128Type32x2s:
				data, ok := memoryInst.Read(ctx, offset, 8)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				ce.pushValue(uint64(int32(binary.LittleEndian.Uint32(data))))
				ce.pushValue(uint64(int32(binary.LittleEndian.Uint32(data[4:]))))
			case wazeroir.LoadV128Type32x2u:
				data, ok := memoryInst.Read(ctx, offset, 8)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				ce.pushValue(uint64(binary.LittleEndian.Uint32(data)))
				ce.pushValue(uint64(binary.LittleEndian.Uint32(data[4:])))
			case wazeroir.LoadV128Type8Splat:
				v, ok := memoryInst.ReadByte(ctx, offset)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				v8 := uint64(v)<<56 | uint64(v)<<48 | uint64(v)<<40 | uint64(v)<<32 |
					uint64(v)<<24 | uint64(v)<<16 | uint64(v)<<8 | uint64(v)
				ce.pushValue(v8)
				ce.pushValue(v8)
			case wazeroir.LoadV128Type16Splat:
				v, ok := memoryInst.ReadUint16Le(ctx, offset)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				v4 := uint64(v)<<48 | uint64(v)<<32 | uint64(v)<<16 | uint64(v)
				ce.pushValue(v4)
				ce.pushValue(v4)
			case wazeroir.LoadV128Type32Splat:
				v, ok := memoryInst.ReadUint32Le(ctx, offset)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				vv := uint64(v)<<32 | uint64(v)
				ce.pushValue(vv)
				ce.pushValue(vv)
			case wazeroir.LoadV128Type64Splat:
				lo, ok := memoryInst.ReadUint64Le(ctx, offset)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				ce.pushValue(lo)
				ce.pushValue(lo)
			case wazeroir.LoadV128Type32zero:
				lo, ok := memoryInst.ReadUint32Le(ctx, offset)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				ce.pushValue(uint64(lo))
				ce.pushValue(0)
			case wazeroir.LoadV128Type64zero:
				lo, ok := memoryInst.ReadUint64Le(ctx, offset)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				ce.pushValue(lo)
				ce.pushValue(0)
			}
			frame.pc++
		case wazeroir.OperationKindV128LoadLane:
			hi, lo := ce.popValue(), ce.popValue()
			offset := ce.popMemoryOffset(op)
			switch op.b1 {
			case 8:
				b, ok := memoryInst.ReadByte(ctx, offset)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				if op.b2 < 8 {
					s := op.b2 << 3
					lo = (lo & ^(0xff << s)) | uint64(b)<<s
				} else {
					s := (op.b2 - 8) << 3
					hi = (hi & ^(0xff << s)) | uint64(b)<<s
				}
			case 16:
				b, ok := memoryInst.ReadUint16Le(ctx, offset)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				if op.b2 < 4 {
					s := op.b2 << 4
					lo = (lo & ^(0xff_ff << s)) | uint64(b)<<s
				} else {
					s := (op.b2 - 4) << 4
					hi = (hi & ^(0xff_ff << s)) | uint64(b)<<s
				}
			case 32:
				b, ok := memoryInst.ReadUint32Le(ctx, offset)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				if op.b2 < 2 {
					s := op.b2 << 5
					lo = (lo & ^(0xff_ff_ff_ff << s)) | uint64(b)<<s
				} else {
					s := (op.b2 - 2) << 5
					hi = (hi & ^(0xff_ff_ff_ff << s)) | uint64(b)<<s
				}
			case 64:
				b, ok := memoryInst.ReadUint64Le(ctx, offset)
				if !ok {
					panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				if op.b2 == 0 {
					lo = b
				} else {
					hi = b
				}
			}
			ce.pushValue(lo)
			ce.pushValue(hi)
			frame.pc++
		case wazeroir.OperationKindV128Store:
			hi, lo := ce.popValue(), ce.popValue()
			offset := ce.popMemoryOffset(op)
			if ok := memoryInst.WriteUint64Le(ctx, offset, lo); !ok {
				panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
			}
			if ok := memoryInst.WriteUint64Le(ctx, offset+8, hi); !ok {
				panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
			}
			frame.pc++
		case wazeroir.OperationKindV128StoreLane:
			hi, lo := ce.popValue(), ce.popValue()
			offset := ce.popMemoryOffset(op)
			var ok bool
			switch op.b1 {
			case 8:
				if op.b2 < 8 {
					ok = memoryInst.WriteByte(ctx, offset, byte(lo>>(op.b2*8)))
				} else {
					ok = memoryInst.WriteByte(ctx, offset, byte(hi>>((op.b2-8)*8)))
				}
			case 16:
				if op.b2 < 4 {
					ok = memoryInst.WriteUint16Le(ctx, offset, uint16(lo>>(op.b2*16)))
				} else {
					ok = memoryInst.WriteUint16Le(ctx, offset, uint16(hi>>((op.b2-4)*16)))
				}
			case 32:
				if op.b2 < 2 {
					ok = memoryInst.WriteUint32Le(ctx, offset, uint32(lo>>(op.b2*32)))
				} else {
					ok = memoryInst.WriteUint32Le(ctx, offset, uint32(hi>>((op.b2-2)*32)))
				}
			case 64:
				if op.b2 == 0 {
					ok = memoryInst.WriteUint64Le(ctx, offset, lo)
				} else {
					ok = memoryInst.WriteUint64Le(ctx, offset, hi)
				}
			}
			if !ok {
				panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
			}
			frame.pc++
		case wazeroir.OperationKindV128ReplaceLane:
			v := ce.popValue()
			hi, lo := ce.popValue(), ce.popValue()
			switch op.b1 {
			case wazeroir.ShapeI8x16:
				if op.b2 < 8 {
					s := op.b2 << 3
					lo = (lo & ^(0xff << s)) | uint64(byte(v))<<s
				} else {
					s := (op.b2 - 8) << 3
					hi = (hi & ^(0xff << s)) | uint64(byte(v))<<s
				}
			case wazeroir.ShapeI16x8:
				if op.b2 < 4 {
					s := op.b2 << 4
					lo = (lo & ^(0xff_ff << s)) | uint64(uint16(v))<<s
				} else {
					s := (op.b2 - 4) << 4
					hi = (hi & ^(0xff_ff << s)) | uint64(uint16(v))<<s
				}
			case wazeroir.ShapeI32x4, wazeroir.ShapeF32x4:
				if op.b2 < 2 {
					s := op.b2 << 5
					lo = (lo & ^(0xff_ff_ff_ff << s)) | uint64(uint32(v))<<s
				} else {
					s := (op.b2 - 2) << 5
					hi = (hi & ^(0xff_ff_ff_ff << s)) | uint64(uint32(v))<<s
				}
			case wazeroir.ShapeI64x2, wazeroir.ShapeF64x2:
				if op.b2 == 0 {
					lo = v
				} else {
					hi = v
				}
			}
			ce.pushValue(lo)
			ce.pushValue(hi)
			frame.pc++
		case wazeroir.OperationKindV128ExtractLane:
			hi, lo := ce.popValue(), ce.popValue()
			var v uint64
			switch op.b1 {
			case wazeroir.ShapeI8x16:
				var u8 byte
				if op.b2 < 8 {
					u8 = byte(lo >> (op.b2 * 8))
				} else {
					u8 = byte(hi >> ((op.b2 - 8) * 8))
				}
				if op.b3 {
					// sign-extend.
					v = uint64(int8(u8))
				} else {
					v = uint64(u8)
				}
			case wazeroir.ShapeI16x8:
				var u16 uint16
				if op.b2 < 4 {
					u16 = uint16(lo >> (op.b2 * 16))
				} else {
					u16 = uint16(hi >> ((op.b2 - 4) * 16))
				}
				if op.b3 {
					// sign-extend.
					v = uint64(int16(u16))
				} else {
					v = uint64(u16)
				}
			case wazeroir.ShapeI32x4, wazeroir.ShapeF32x4:
				if op.b2 < 2 {
					v = uint64(uint32(lo >> (op.b2 * 32)))
				} else {
					v = uint64(uint32(hi >> ((op.b2 - 2) * 32)))
				}
			case wazeroir.ShapeI64x2, wazeroir.ShapeF64x2:
				if op.b2 == 0 {
					v = lo
				} else {
					v = hi
				}
			}
			ce.pushValue(v)
			frame.pc++
		case wazeroir.OperationKindV128Splat:
			v := ce.popValue()
			var hi, lo uint64
			switch op.b1 {
			case wazeroir.ShapeI8x16:
				v8 := uint64(byte(v))<<56 | uint64(byte(v))<<48 | uint64(byte(v))<<40 | uint64(byte(v))<<32 |
					uint64(byte(v))<<24 | uint64(byte(v))<<16 | uint64(byte(v))<<8 | uint64(byte(v))
				hi, lo = v8, v8
			case wazeroir.ShapeI16x8:
				v4 := uint64(uint16(v))<<48 | uint64(uint16(v))<<32 | uint64(uint16(v))<<16 | uint64(uint16(v))
				hi, lo = v4, v4
			case wazeroir.ShapeI32x4, wazeroir.ShapeF32x4:
				v2 := uint64(uint32(v))<<32 | uint64(uint32(v))
				lo, hi = v2, v2
			case wazeroir.ShapeI64x2, wazeroir.ShapeF64x2:
				lo, hi = v, v
			}
			ce.pushValue(lo)
			ce.pushValue(hi)
			frame.pc++
		case wazeroir.OperationKindV128Swizzle:
			idxHi, idxLo := ce.popValue(), ce.popValue()
			baseHi, baseLo := ce.popValue(), ce.popValue()
			var newVal [16]byte
			for i := 0; i < 16; i++ {
				var id byte
				if i < 8 {
					id = byte(idxLo >> (i * 8))
				} else {
					id = byte(idxHi >> ((i - 8) * 8))
				}
				if id < 8 {
					newVal[i] = byte(baseLo >> (id * 8))
				} else if id < 16 {
					newVal[i] = byte(baseHi >> ((id - 8) * 8))
				}
			}
			ce.pushValue(binary.LittleEndian.Uint64(newVal[:8]))
			ce.pushValue(binary.LittleEndian.Uint64(newVal[8:]))
			frame.pc++
		case wazeroir.OperationKindV128Shuffle:
			xHi, xLo, yHi, yLo := ce.popValue(), ce.popValue(), ce.popValue(), ce.popValue()
			var newVal [16]byte
			for i, l := range op.us {
				if l < 8 {
					newVal[i] = byte(yLo >> (l * 8))
				} else if l < 16 {
					newVal[i] = byte(yHi >> ((l - 8) * 8))
				} else if l < 24 {
					newVal[i] = byte(xLo >> ((l - 16) * 8))
				} else if l < 32 {
					newVal[i] = byte(xHi >> ((l - 24) * 8))
				}
			}
			ce.pushValue(binary.LittleEndian.Uint64(newVal[:8]))
			ce.pushValue(binary.LittleEndian.Uint64(newVal[8:]))
			frame.pc++
		case wazeroir.OperationKindV128AnyTrue:
			hi, lo := ce.popValue(), ce.popValue()
			if hi != 0 || lo != 0 {
				ce.pushValue(1)
			} else {
				ce.pushValue(0)
			}
			frame.pc++
		case wazeroir.OperationKindV128AllTrue:
			hi, lo := ce.popValue(), ce.popValue()
			var ret bool
			switch op.b1 {
			case wazeroir.ShapeI8x16:
				ret = (uint8(lo) != 0) && (uint8(lo>>8) != 0) && (uint8(lo>>16) != 0) && (uint8(lo>>24) != 0) &&
					(uint8(lo>>32) != 0) && (uint8(lo>>40) != 0) && (uint8(lo>>48) != 0) && (uint8(lo>>56) != 0) &&
					(uint8(hi) != 0) && (uint8(hi>>8) != 0) && (uint8(hi>>16) != 0) && (uint8(hi>>24) != 0) &&
					(uint8(hi>>32) != 0) && (uint8(hi>>40) != 0) && (uint8(hi>>48) != 0) && (uint8(hi>>56) != 0)
			case wazeroir.ShapeI16x8:
				ret = (uint16(lo) != 0) && (uint16(lo>>16) != 0) && (uint16(lo>>32) != 0) && (uint16(lo>>48) != 0) &&
					(uint16(hi) != 0) && (uint16(hi>>16) != 0) && (uint16(hi>>32) != 0) && (uint16(hi>>48) != 0)
			case wazeroir.ShapeI32x4:
				ret = (uint32(lo) != 0) && (uint32(lo>>32) != 0) &&
					(uint32(hi) != 0) && (uint32(hi>>32) != 0)
			case wazeroir.ShapeI64x2:
				ret = (lo != 0) &&
					(hi != 0)
			}
			if ret {
				ce.pushValue(1)
			} else {
				ce.pushValue(0)
			}
			frame.pc++
		case wazeroir.OperationKindV128BitMask:
			// https://github.com/WebAssembly/spec/blob/main/proposals/simd/SIMD.md#bitmask-extraction
			hi, lo := ce.popValue(), ce.popValue()
			var res uint64
			switch op.b1 {
			case wazeroir.ShapeI8x16:
				for i := 0; i < 8; i++ {
					if int8(lo>>(i*8)) < 0 {
						res |= 1 << i
					}
				}
				for i := 0; i < 8; i++ {
					if int8(hi>>(i*8)) < 0 {
						res |= 1 << (i + 8)
					}
				}
			case wazeroir.ShapeI16x8:
				for i := 0; i < 4; i++ {
					if int8(lo>>(i*16)) < 0 {
						res |= 1 << i
					}
				}
				for i := 0; i < 4; i++ {
					if int8(hi>>(i*16)) < 0 {
						res |= 1 << (i + 4)
					}
				}
			case wazeroir.ShapeI32x4:
				for i := 0; i < 2; i++ {
					if int8(lo>>(i*32)) < 0 {
						res |= 1 << i
					}
				}
				for i := 0; i < 2; i++ {
					if int8(hi>>(i*32)) < 0 {
						res |= 1 << (i + 2)
					}
				}
			case wazeroir.ShapeI64x2:
				if int64(lo) < 0 {
					res |= 0b01
				}
				if int(hi) < 0 {
					res |= 0b10
				}
			}
			ce.pushValue(res)
			frame.pc++
		case wazeroir.OperationKindV128And:
			x2Hi, x2Lo := ce.popValue(), ce.popValue()
			x1Hi, x1Lo := ce.popValue(), ce.popValue()
			ce.pushValue(x1Lo & x2Lo)
			ce.pushValue(x1Hi & x2Hi)
			frame.pc++
		case wazeroir.OperationKindV128Not:
			hi, lo := ce.popValue(), ce.popValue()
			ce.pushValue(^lo)
			ce.pushValue(^hi)
			frame.pc++
		case wazeroir.OperationKindV128Or:
			x2Hi, x2Lo := ce.popValue(), ce.popValue()
			x1Hi, x1Lo := ce.popValue(), ce.popValue()
			ce.pushValue(x1Lo | x2Lo)
			ce.pushValue(x1Hi | x2Hi)
			frame.pc++
		case wazeroir.OperationKindV128Xor:
			x2Hi, x2Lo := ce.popValue(), ce.popValue()
			x1Hi, x1Lo := ce.popValue(), ce.popValue()
			ce.pushValue(x1Lo ^ x2Lo)
			ce.pushValue(x1Hi ^ x2Hi)
			frame.pc++
		case wazeroir.OperationKindV128Bitselect:
			// https://github.com/WebAssembly/spec/blob/main/proposals/simd/SIMD.md#bitwise-select
			cHi, cLo := ce.popValue(), ce.popValue()
			x2Hi, x2Lo := ce.popValue(), ce.popValue()
			x1Hi, x1Lo := ce.popValue(), ce.popValue()
			// v128.or(v128.and(v1, c), v128.and(v2, v128.not(c)))
			ce.pushValue((x1Lo & cLo) | (x2Lo & (^cLo)))
			ce.pushValue((x1Hi & cHi) | (x2Hi & (^cHi)))
			frame.pc++
		case wazeroir.OperationKindV128AndNot:
			x2Hi, x2Lo := ce.popValue(), ce.popValue()
			x1Hi, x1Lo := ce.popValue(), ce.popValue()
			ce.pushValue(x1Lo & (^x2Lo))
			ce.pushValue(x1Hi & (^x2Hi))
			frame.pc++
		case wazeroir.OperationKindV128Shl:
			s := ce.popValue()
			hi, lo := ce.popValue(), ce.popValue()
			switch op.b1 {
			case wazeroir.ShapeI8x16:
				s = s % 8
				lo = uint64(uint8(lo<<s)) |
					uint64(uint8((lo>>8)<<s))<<8 |
					uint64(uint8((lo>>16)<<s))<<16 |
					uint64(uint8((lo>>24)<<s))<<24 |
					uint64(uint8((lo>>32)<<s))<<32 |
					uint64(uint8((lo>>40)<<s))<<40 |
					uint64(uint8((lo>>48)<<s))<<48 |
					uint64(uint8((lo>>56)<<s))<<56
				hi = uint64(uint8(hi<<s)) |
					uint64(uint8((hi>>8)<<s))<<8 |
					uint64(uint8((hi>>16)<<s))<<16 |
					uint64(uint8((hi>>24)<<s))<<24 |
					uint64(uint8((hi>>32)<<s))<<32 |
					uint64(uint8((hi>>40)<<s))<<40 |
					uint64(uint8((hi>>48)<<s))<<48 |
					uint64(uint8((hi>>56)<<s))<<56
			case wazeroir.ShapeI16x8:
				s = s % 16
				lo = uint64(uint16(lo<<s)) |
					uint64(uint16((lo>>16)<<s))<<16 |
					uint64(uint16((lo>>32)<<s))<<32 |
					uint64(uint16((lo>>48)<<s))<<48
				hi = uint64(uint16(hi<<s)) |
					uint64(uint16((hi>>16)<<s))<<16 |
					uint64(uint16((hi>>32)<<s))<<32 |
					uint64(uint16((hi>>48)<<s))<<48
			case wazeroir.ShapeI32x4:
				s = s % 32
				lo = uint64(uint32(lo<<s)) | uint64(uint32((lo>>32)<<s))<<32
				hi = uint64(uint32(hi<<s)) | uint64(uint32((hi>>32)<<s))<<32
			case wazeroir.ShapeI64x2:
				s = s % 64
				lo = lo << s
				hi = hi << s
			}
			ce.pushValue(lo)
			ce.pushValue(hi)
			frame.pc++
		case wazeroir.OperationKindV128Shr:
			s := ce.popValue()
			hi, lo := ce.popValue(), ce.popValue()
			switch op.b1 {
			case wazeroir.ShapeI8x16:
				s = s % 8
				if op.b3 { // signed
					lo = uint64(uint8(int8(lo)>>s)) |
						uint64(uint8(int8(lo>>8)>>s))<<8 |
						uint64(uint8(int8(lo>>16)>>s))<<16 |
						uint64(uint8(int8(lo>>24)>>s))<<24 |
						uint64(uint8(int8(lo>>32)>>s))<<32 |
						uint64(uint8(int8(lo>>40)>>s))<<40 |
						uint64(uint8(int8(lo>>48)>>s))<<48 |
						uint64(uint8(int8(lo>>56)>>s))<<56
					hi = uint64(uint8(int8(hi)>>s)) |
						uint64(uint8(int8(hi>>8)>>s))<<8 |
						uint64(uint8(int8(hi>>16)>>s))<<16 |
						uint64(uint8(int8(hi>>24)>>s))<<24 |
						uint64(uint8(int8(hi>>32)>>s))<<32 |
						uint64(uint8(int8(hi>>40)>>s))<<40 |
						uint64(uint8(int8(hi>>48)>>s))<<48 |
						uint64(uint8(int8(hi>>56)>>s))<<56
				} else {
					lo = uint64(uint8(lo)>>s) |
						uint64(uint8(lo>>8)>>s)<<8 |
						uint64(uint8(lo>>16)>>s)<<16 |
						uint64(uint8(lo>>24)>>s)<<24 |
						uint64(uint8(lo>>32)>>s)<<32 |
						uint64(uint8(lo>>40)>>s)<<40 |
						uint64(uint8(lo>>48)>>s)<<48 |
						uint64(uint8(lo>>56)>>s)<<56
					hi = uint64(uint8(hi)>>s) |
						uint64(uint8(hi>>8)>>s)<<8 |
						uint64(uint8(hi>>16)>>s)<<16 |
						uint64(uint8(hi>>24)>>s)<<24 |
						uint64(uint8(hi>>32)>>s)<<32 |
						uint64(uint8(hi>>40)>>s)<<40 |
						uint64(uint8(hi>>48)>>s)<<48 |
						uint64(uint8(hi>>56)>>s)<<56
				}
			case wazeroir.ShapeI16x8:
				s = s % 16
				if op.b3 { // signed
					lo = uint64(uint16(int16(lo)>>s)) |
						uint64(uint16(int16(lo>>16)>>s))<<16 |
						uint64(uint16(int16(lo>>32)>>s))<<32 |
						uint64(uint16(int16(lo>>48)>>s))<<48
					hi = uint64(uint16(int16(hi)>>s)) |
						uint64(uint16(int16(hi>>16)>>s))<<16 |
						uint64(uint16(int16(hi>>32)>>s))<<32 |
						uint64(uint16(int16(hi>>48)>>s))<<48
				} else {
					lo = uint64(uint16(lo)>>s) |
						uint64(uint16(lo>>16)>>s)<<16 |
						uint64(uint16(lo>>32)>>s)<<32 |
						uint64(uint16(lo>>48)>>s)<<48
					hi = uint64(uint16(hi)>>s) |
						uint64(uint16(hi>>16)>>s)<<16 |
						uint64(uint16(hi>>32)>>s)<<32 |
						uint64(uint16(hi>>48)>>s)<<48
				}
			case wazeroir.ShapeI32x4:
				s = s % 32
				if op.b3 {
					lo = uint64(uint32(int32(lo)>>s)) | uint64(uint32(int32(lo>>32)>>s))<<32
					hi = uint64(uint32(int32(hi)>>s)) | uint64(uint32(int32(hi>>32)>>s))<<32
				} else {
					lo = uint64(uint32(lo)>>s) | uint64(uint32(lo>>32)>>s)<<32
					hi = uint64(uint32(hi)>>s) | uint64(uint32(hi>>32)>>s)<<32
				}
			case wazeroir.ShapeI64x2:
				s = s % 64
				if op.b3 { // signed
					lo = uint64(int64(lo) >> s)
					hi = uint64(int64(hi) >> s)
				} else {
					lo = lo >> s
					hi = hi >> s
				}

			}
			ce.pushValue(lo)
			ce.pushValue(hi)
			frame.pc++
		case wazeroir.OperationKindV128Cmp:
			x2Hi, x2Lo := ce.popValue(), ce.popValue()
			x1Hi, x1Lo := ce.popValue(), ce.popValue()
			var result []bool
			switch op.b1 {
			case wazeroir.V128CmpTypeI8x16Eq:
				result = []bool{
					byte(x1Lo>>0) == byte(x2Lo>>0), byte(x1Lo>>8) == byte(x2Lo>>8),
					byte(x1Lo>>16) == byte(x2Lo>>16), byte(x1Lo>>24) == byte(x2Lo>>24),
					byte(x1Lo>>32) == byte(x2Lo>>32), byte(x1Lo>>40) == byte(x2Lo>>40),
					byte(x1Lo>>48) == byte(x2Lo>>48), byte(x1Lo>>56) == byte(x2Lo>>56),
					byte(x1Hi>>0) == byte(x2Hi>>0), byte(x1Hi>>8) == byte(x2Hi>>8),
					byte(x1Hi>>16) == byte(x2Hi>>16), byte(x1Hi>>24) == byte(x2Hi>>24),
					byte(x1Hi>>32) == byte(x2Hi>>32), byte(x1Hi>>40) == byte(x2Hi>>40),
					byte(x1Hi>>48) == byte(x2Hi>>48), byte(x1Hi>>56) == byte(x2Hi>>56),
				}
			case wazeroir.V128CmpTypeI8x16Ne:
				result = []bool{
					byte(x1Lo>>0) != byte(x2Lo>>0), byte(x1Lo>>8) != byte(x2Lo>>8),
					byte(x1Lo>>16) != byte(x2Lo>>16), byte(x1Lo>>24) != byte(x2Lo>>24),
					byte(x1Lo>>32) != byte(x2Lo>>32), byte(x1Lo>>40) != byte(x2Lo>>40),
					byte(x1Lo>>48) != byte(x2Lo>>48), byte(x1Lo>>56) != byte(x2Lo>>56),
					byte(x1Hi>>0) != byte(x2Hi>>0), byte(x1Hi>>8) != byte(x2Hi>>8),
					byte(x1Hi>>16) != byte(x2Hi>>16), byte(x1Hi>>24) != byte(x2Hi>>24),
					byte(x1Hi>>32) != byte(x2Hi>>32), byte(x1Hi>>40) != byte(x2Hi>>40),
					byte(x1Hi>>48) != byte(x2Hi>>48), byte(x1Hi>>56) != byte(x2Hi>>56),
				}
			case wazeroir.V128CmpTypeI8x16LtS:
				result = []bool{
					int8(x1Lo>>0) < int8(x2Lo>>0), int8(x1Lo>>8) < int8(x2Lo>>8),
					int8(x1Lo>>16) < int8(x2Lo>>16), int8(x1Lo>>24) < int8(x2Lo>>24),
					int8(x1Lo>>32) < int8(x2Lo>>32), int8(x1Lo>>40) < int8(x2Lo>>40),
					int8(x1Lo>>48) < int8(x2Lo>>48), int8(x1Lo>>56) < int8(x2Lo>>56),
					int8(x1Hi>>0) < int8(x2Hi>>0), int8(x1Hi>>8) < int8(x2Hi>>8),
					int8(x1Hi>>16) < int8(x2Hi>>16), int8(x1Hi>>24) < int8(x2Hi>>24),
					int8(x1Hi>>32) < int8(x2Hi>>32), int8(x1Hi>>40) < int8(x2Hi>>40),
					int8(x1Hi>>48) < int8(x2Hi>>48), int8(x1Hi>>56) < int8(x2Hi>>56),
				}
			case wazeroir.V128CmpTypeI8x16LtU:
				result = []bool{
					byte(x1Lo>>0) < byte(x2Lo>>0), byte(x1Lo>>8) < byte(x2Lo>>8),
					byte(x1Lo>>16) < byte(x2Lo>>16), byte(x1Lo>>24) < byte(x2Lo>>24),
					byte(x1Lo>>32) < byte(x2Lo>>32), byte(x1Lo>>40) < byte(x2Lo>>40),
					byte(x1Lo>>48) < byte(x2Lo>>48), byte(x1Lo>>56) < byte(x2Lo>>56),
					byte(x1Hi>>0) < byte(x2Hi>>0), byte(x1Hi>>8) < byte(x2Hi>>8),
					byte(x1Hi>>16) < byte(x2Hi>>16), byte(x1Hi>>24) < byte(x2Hi>>24),
					byte(x1Hi>>32) < byte(x2Hi>>32), byte(x1Hi>>40) < byte(x2Hi>>40),
					byte(x1Hi>>48) < byte(x2Hi>>48), byte(x1Hi>>56) < byte(x2Hi>>56),
				}
			case wazeroir.V128CmpTypeI8x16GtS:
				result = []bool{
					int8(x1Lo>>0) > int8(x2Lo>>0), int8(x1Lo>>8) > int8(x2Lo>>8),
					int8(x1Lo>>16) > int8(x2Lo>>16), int8(x1Lo>>24) > int8(x2Lo>>24),
					int8(x1Lo>>32) > int8(x2Lo>>32), int8(x1Lo>>40) > int8(x2Lo>>40),
					int8(x1Lo>>48) > int8(x2Lo>>48), int8(x1Lo>>56) > int8(x2Lo>>56),
					int8(x1Hi>>0) > int8(x2Hi>>0), int8(x1Hi>>8) > int8(x2Hi>>8),
					int8(x1Hi>>16) > int8(x2Hi>>16), int8(x1Hi>>24) > int8(x2Hi>>24),
					int8(x1Hi>>32) > int8(x2Hi>>32), int8(x1Hi>>40) > int8(x2Hi>>40),
					int8(x1Hi>>48) > int8(x2Hi>>48), int8(x1Hi>>56) > int8(x2Hi>>56),
				}
			case wazeroir.V128CmpTypeI8x16GtU:
				result = []bool{
					byte(x1Lo>>0) > byte(x2Lo>>0), byte(x1Lo>>8) > byte(x2Lo>>8),
					byte(x1Lo>>16) > byte(x2Lo>>16), byte(x1Lo>>24) > byte(x2Lo>>24),
					byte(x1Lo>>32) > byte(x2Lo>>32), byte(x1Lo>>40) > byte(x2Lo>>40),
					byte(x1Lo>>48) > byte(x2Lo>>48), byte(x1Lo>>56) > byte(x2Lo>>56),
					byte(x1Hi>>0) > byte(x2Hi>>0), byte(x1Hi>>8) > byte(x2Hi>>8),
					byte(x1Hi>>16) > byte(x2Hi>>16), byte(x1Hi>>24) > byte(x2Hi>>24),
					byte(x1Hi>>32) > byte(x2Hi>>32), byte(x1Hi>>40) > byte(x2Hi>>40),
					byte(x1Hi>>48) > byte(x2Hi>>48), byte(x1Hi>>56) > byte(x2Hi>>56),
				}
			case wazeroir.V128CmpTypeI8x16LeS:
				result = []bool{
					int8(x1Lo>>0) <= int8(x2Lo>>0), int8(x1Lo>>8) <= int8(x2Lo>>8),
					int8(x1Lo>>16) <= int8(x2Lo>>16), int8(x1Lo>>24) <= int8(x2Lo>>24),
					int8(x1Lo>>32) <= int8(x2Lo>>32), int8(x1Lo>>40) <= int8(x2Lo>>40),
					int8(x1Lo>>48) <= int8(x2Lo>>48), int8(x1Lo>>56) <= int8(x2Lo>>56),
					int8(x1Hi>>0) <= int8(x2Hi>>0), int8(x1Hi>>8) <= int8(x2Hi>>8),
					int8(x1Hi>>16) <= int8(x2Hi>>16), int8(x1Hi>>24) <= int8(x2Hi>>24),
					int8(x1Hi>>32) <= int8(x2Hi>>32), int8(x1Hi>>40) <= int8(x2Hi>>40),
					int8(x1Hi>>48) <= int8(x2Hi>>48), int8(x1Hi>>56) <= int8(x2Hi>>56),
				}
			case wazeroir.V128CmpTypeI8x16LeU:
				result = []bool{
					byte(x1Lo>>0) <= byte(x2Lo>>0), byte(x1Lo>>8) <= byte(x2Lo>>8),
					byte(x1Lo>>16) <= byte(x2Lo>>16), byte(x1Lo>>24) <= byte(x2Lo>>24),
					byte(x1Lo>>32) <= byte(x2Lo>>32), byte(x1Lo>>40) <= byte(x2Lo>>40),
					byte(x1Lo>>48) <= byte(x2Lo>>48), byte(x1Lo>>56) <= byte(x2Lo>>56),
					byte(x1Hi>>0) <= byte(x2Hi>>0), byte(x1Hi>>8) <= byte(x2Hi>>8),
					byte(x1Hi>>16) <= byte(x2Hi>>16), byte(x1Hi>>24) <= byte(x2Hi>>24),
					byte(x1Hi>>32) <= byte(x2Hi>>32), byte(x1Hi>>40) <= byte(x2Hi>>40),
					byte(x1Hi>>48) <= byte(x2Hi>>48), byte(x1Hi>>56) <= byte(x2Hi>>56),
				}
			case wazeroir.V128CmpTypeI8x16GeS:
				result = []bool{
					int8(x1Lo>>0) >= int8(x2Lo>>0), int8(x1Lo>>8) >= int8(x2Lo>>8),
					int8(x1Lo>>16) >= int8(x2Lo>>16), int8(x1Lo>>24) >= int8(x2Lo>>24),
					int8(x1Lo>>32) >= int8(x2Lo>>32), int8(x1Lo>>40) >= int8(x2Lo>>40),
					int8(x1Lo>>48) >= int8(x2Lo>>48), int8(x1Lo>>56) >= int8(x2Lo>>56),
					int8(x1Hi>>0) >= int8(x2Hi>>0), int8(x1Hi>>8) >= int8(x2Hi>>8),
					int8(x1Hi>>16) >= int8(x2Hi>>16), int8(x1Hi>>24) >= int8(x2Hi>>24),
					int8(x1Hi>>32) >= int8(x2Hi>>32), int8(x1Hi>>40) >= int8(x2Hi>>40),
					int8(x1Hi>>48) >= int8(x2Hi>>48), int8(x1Hi>>56) >= int8(x2Hi>>56),
				}
			case wazeroir.V128CmpTypeI8x16GeU:
				result = []bool{
					byte(x1Lo>>0) >= byte(x2Lo>>0), byte(x1Lo>>8) >= byte(x2Lo>>8),
					byte(x1Lo>>16) >= byte(x2Lo>>16), byte(x1Lo>>24) >= byte(x2Lo>>24),
					byte(x1Lo>>32) >= byte(x2Lo>>32), byte(x1Lo>>40) >= byte(x2Lo>>40),
					byte(x1Lo>>48) >= byte(x2Lo>>48), byte(x1Lo>>56) >= byte(x2Lo>>56),
					byte(x1Hi>>0) >= byte(x2Hi>>0), byte(x1Hi>>8) >= byte(x2Hi>>8),
					byte(x1Hi>>16) >= byte(x2Hi>>16), byte(x1Hi>>24) >= byte(x2Hi>>24),
					byte(x1Hi>>32) >= byte(x2Hi>>32), byte(x1Hi>>40) >= byte(x2Hi>>40),
					byte(x1Hi>>48) >= byte(x2Hi>>48), byte(x1Hi>>56) >= byte(x2Hi>>56),
				}
			case wazeroir.V128CmpTypeI16x8Eq:
				result = []bool{
					uint16(x1Lo>>0) == uint16(x2Lo>>0), uint16(x1Lo>>16) == uint16(x2Lo>>16),
					uint16(x1Lo>>32) == uint16(x2Lo>>32), uint16(x1Lo>>48) == uint16(x2Lo>>48),
					uint16(x1Hi>>0) == uint16(x2Hi>>0), uint16(x1Hi>>16) == uint16(x2Hi>>16),
					uint16(x1Hi>>32) == uint16(x2Hi>>32), uint16(x1Hi>>48) == uint16(x2Hi>>48),
				}
			case wazeroir.V128CmpTypeI16x8Ne:
				result = []bool{
					uint16(x1Lo>>0) != uint16(x2Lo>>0), uint16(x1Lo>>16) != uint16(x2Lo>>16),
					uint16(x1Lo>>32) != uint16(x2Lo>>32), uint16(x1Lo>>48) != uint16(x2Lo>>48),
					uint16(x1Hi>>0) != uint16(x2Hi>>0), uint16(x1Hi>>16) != uint16(x2Hi>>16),
					uint16(x1Hi>>32) != uint16(x2Hi>>32), uint16(x1Hi>>48) != uint16(x2Hi>>48),
				}
			case wazeroir.V128CmpTypeI16x8LtS:
				result = []bool{
					int16(x1Lo>>0) < int16(x2Lo>>0), int16(x1Lo>>16) < int16(x2Lo>>16),
					int16(x1Lo>>32) < int16(x2Lo>>32), int16(x1Lo>>48) < int16(x2Lo>>48),
					int16(x1Hi>>0) < int16(x2Hi>>0), int16(x1Hi>>16) < int16(x2Hi>>16),
					int16(x1Hi>>32) < int16(x2Hi>>32), int16(x1Hi>>48) < int16(x2Hi>>48),
				}
			case wazeroir.V128CmpTypeI16x8LtU:
				result = []bool{
					uint16(x1Lo>>0) < uint16(x2Lo>>0), uint16(x1Lo>>16) < uint16(x2Lo>>16),
					uint16(x1Lo>>32) < uint16(x2Lo>>32), uint16(x1Lo>>48) < uint16(x2Lo>>48),
					uint16(x1Hi>>0) < uint16(x2Hi>>0), uint16(x1Hi>>16) < uint16(x2Hi>>16),
					uint16(x1Hi>>32) < uint16(x2Hi>>32), uint16(x1Hi>>48) < uint16(x2Hi>>48),
				}
			case wazeroir.V128CmpTypeI16x8GtS:
				result = []bool{
					int16(x1Lo>>0) > int16(x2Lo>>0), int16(x1Lo>>16) > int16(x2Lo>>16),
					int16(x1Lo>>32) > int16(x2Lo>>32), int16(x1Lo>>48) > int16(x2Lo>>48),
					int16(x1Hi>>0) > int16(x2Hi>>0), int16(x1Hi>>16) > int16(x2Hi>>16),
					int16(x1Hi>>32) > int16(x2Hi>>32), int16(x1Hi>>48) > int16(x2Hi>>48),
				}
			case wazeroir.V128CmpTypeI16x8GtU:
				result = []bool{
					uint16(x1Lo>>0) > uint16(x2Lo>>0), uint16(x1Lo>>16) > uint16(x2Lo>>16),
					uint16(x1Lo>>32) > uint16(x2Lo>>32), uint16(x1Lo>>48) > uint16(x2Lo>>48),
					uint16(x1Hi>>0) > uint16(x2Hi>>0), uint16(x1Hi>>16) > uint16(x2Hi>>16),
					uint16(x1Hi>>32) > uint16(x2Hi>>32), uint16(x1Hi>>48) > uint16(x2Hi>>48),
				}
			case wazeroir.V128CmpTypeI16x8LeS:
				result = []bool{
					int16(x1Lo>>0) <= int16(x2Lo>>0), int16(x1Lo>>16) <= int16(x2Lo>>16),
					int16(x1Lo>>32) <= int16(x2Lo>>32), int16(x1Lo>>48) <= int16(x2Lo>>48),
					int16(x1Hi>>0) <= int16(x2Hi>>0), int16(x1Hi>>16) <= int16(x2Hi>>16),
					int16(x1Hi>>32) <= int16(x2Hi>>32), int16(x1Hi>>48) <= int16(x2Hi>>48),
				}
			case wazeroir.V128CmpTypeI16x8LeU:
				result = []bool{
					uint16(x1Lo>>0) <= uint16(x2Lo>>0), uint16(x1Lo>>16) <= uint16(x2Lo>>16),
					uint16(x1Lo>>32) <= uint16(x2Lo>>32), uint16(x1Lo>>48) <= uint16(x2Lo>>48),
					uint16(x1Hi>>0) <= uint16(x2Hi>>0), uint16(x1Hi>>16) <= uint16(x2Hi>>16),
					uint16(x1Hi>>32) <= uint16(x2Hi>>32), uint16(x1Hi>>48) <= uint16(x2Hi>>48),
				}
			case wazeroir.V128CmpTypeI16x8GeS:
				result = []bool{
					int16(x1Lo>>0) >= int16(x2Lo>>0), int16(x1Lo>>16) >= int16(x2Lo>>16),
					int16(x1Lo>>32) >= int16(x2Lo>>32), int16(x1Lo>>48) >= int16(x2Lo>>48),
					int16(x1Hi>>0) >= int16(x2Hi>>0), int16(x1Hi>>16) >= int16(x2Hi>>16),
					int16(x1Hi>>32) >= int16(x2Hi>>32), int16(x1Hi>>48) >= int16(x2Hi>>48),
				}
			case wazeroir.V128CmpTypeI16x8GeU:
				result = []bool{
					uint16(x1Lo>>0) >= uint16(x2Lo>>0), uint16(x1Lo>>16) >= uint16(x2Lo>>16),
					uint16(x1Lo>>32) >= uint16(x2Lo>>32), uint16(x1Lo>>48) >= uint16(x2Lo>>48),
					uint16(x1Hi>>0) >= uint16(x2Hi>>0), uint16(x1Hi>>16) >= uint16(x2Hi>>16),
					uint16(x1Hi>>32) >= uint16(x2Hi>>32), uint16(x1Hi>>48) >= uint16(x2Hi>>48),
				}
			case wazeroir.V128CmpTypeI32x4Eq:
				result = []bool{
					uint32(x1Lo>>0) == uint32(x2Lo>>0), uint32(x1Lo>>32) == uint32(x2Lo>>32),
					uint32(x1Hi>>0) == uint32(x2Hi>>0), uint32(x1Hi>>32) == uint32(x2Hi>>32),
				}
			case wazeroir.V128CmpTypeI32x4Ne:
				result = []bool{
					uint32(x1Lo>>0) != uint32(x2Lo>>0), uint32(x1Lo>>32) != uint32(x2Lo>>32),
					uint32(x1Hi>>0) != uint32(x2Hi>>0), uint32(x1Hi>>32) != uint32(x2Hi>>32),
				}
			case wazeroir.V128CmpTypeI32x4LtS:
				result = []bool{
					int32(x1Lo>>0) < int32(x2Lo>>0), int32(x1Lo>>32) < int32(x2Lo>>32),
					int32(x1Hi>>0) < int32(x2Hi>>0), int32(x1Hi>>32) < int32(x2Hi>>32),
				}
			case wazeroir.V128CmpTypeI32x4LtU:
				result = []bool{
					uint32(x1Lo>>0) < uint32(x2Lo>>0), uint32(x1Lo>>32) < uint32(x2Lo>>32),
					uint32(x1Hi>>0) < uint32(x2Hi>>0), uint32(x1Hi>>32) < uint32(x2Hi>>32),
				}
			case wazeroir.V128CmpTypeI32x4GtS:
				result = []bool{
					int32(x1Lo>>0) > int32(x2Lo>>0), int32(x1Lo>>32) > int32(x2Lo>>32),
					int32(x1Hi>>0) > int32(x2Hi>>0), int32(x1Hi>>32) > int32(x2Hi>>32),
				}
			case wazeroir.V128CmpTypeI32x4GtU:
				result = []bool{
					uint32(x1Lo>>0) > uint32(x2Lo>>0), uint32(x1Lo>>32) > uint32(x2Lo>>32),
					uint32(x1Hi>>0) > uint32(x2Hi>>0), uint32(x1Hi>>32) > uint32(x2Hi>>32),
				}
			case wazeroir.V128CmpTypeI32x4LeS:
				result = []bool{
					int32(x1Lo>>0) <= int32(x2Lo>>0), int32(x1Lo>>32) <= int32(x2Lo>>32),
					int32(x1Hi>>0) <= int32(x2Hi>>0), int32(x1Hi>>32) <= int32(x2Hi>>32),
				}
			case wazeroir.V128CmpTypeI32x4LeU:
				result = []bool{
					uint32(x1Lo>>0) <= uint32(x2Lo>>0), uint32(x1Lo>>32) <= uint32(x2Lo>>32),
					uint32(x1Hi>>0) <= uint32(x2Hi>>0), uint32(x1Hi>>32) <= uint32(x2Hi>>32),
				}
			case wazeroir.V128CmpTypeI32x4GeS:
				result = []bool{
					int32(x1Lo>>0) >= int32(x2Lo>>0), int32(x1Lo>>32) >= int32(x2Lo>>32),
					int32(x1Hi>>0) >= int32(x2Hi>>0), int32(x1Hi>>32) >= int32(x2Hi>>32),
				}
			case wazeroir.V128CmpTypeI32x4GeU:
				result = []bool{
					uint32(x1Lo>>0) >= uint32(x2Lo>>0), uint32(x1Lo>>32) >= uint32(x2Lo>>32),
					uint32(x1Hi>>0) >= uint32(x2Hi>>0), uint32(x1Hi>>32) >= uint32(x2Hi>>32),
				}
			case wazeroir.V128CmpTypeI64x2Eq:
				result = []bool{x1Lo == x2Lo, x1Hi == x2Hi}
			case wazeroir.V128CmpTypeI64x2Ne:
				result = []bool{x1Lo != x2Lo, x1Hi != x2Hi}
			case wazeroir.V128CmpTypeI64x2LtS:
				result = []bool{int64(x1Lo) < int64(x2Lo), int64(x1Hi) < int64(x2Hi)}
			case wazeroir.V128CmpTypeI64x2GtS:
				result = []bool{int64(x1Lo) > int64(x2Lo), int64(x1Hi) > int64(x2Hi)}
			case wazeroir.V128CmpTypeI64x2LeS:
				result = []bool{int64(x1Lo) <= int64(x2Lo), int64(x1Hi) <= int64(x2Hi)}
			case wazeroir.V128CmpTypeI64x2GeS:
				result = []bool{int64(x1Lo) >= int64(x2Lo), int64(x1Hi) >= int64(x2Hi)}
			case wazeroir.V128CmpTypeF32x4Eq:
				result = []bool{
					math.Float32frombits(uint32(x1Lo>>0)) == math.Float32frombits(uint32(x2Lo>>0)),
					math.Float32frombits(uint32(x1Lo>>32)) == math.Float32frombits(uint32(x2Lo>>32)),
					math.Float32frombits(uint32(x1Hi>>0)) == math.Float32frombits(uint32(x2Hi>>0)),
					math.Float32frombits(uint32(x1Hi>>32)) == math.Float32frombits(uint32(x2Hi>>32)),
				}
			case wazeroir.V128CmpTypeF32x4Ne:
				result = []bool{
					math.Float32frombits(uint32(x1Lo>>0)) != math.Float32frombits(uint32(x2Lo>>0)),
					math.Float32frombits(uint32(x1Lo>>32)) != math.Float32frombits(uint32(x2Lo>>32)),
					math.Float32frombits(uint32(x1Hi>>0)) != math.Float32frombits(uint32(x2Hi>>0)),
					math.Float32frombits(uint32(x1Hi>>32)) != math.Float32frombits(uint32(x2Hi>>32)),
				}
			case wazeroir.V128CmpTypeF32x4Lt:
				result = []bool{
					math.Float32frombits(uint32(x1Lo>>0)) < math.Float32frombits(uint32(x2Lo>>0)),
					math.Float32frombits(uint32(x1Lo>>32)) < math.Float32frombits(uint32(x2Lo>>32)),
					math.Float32frombits(uint32(x1Hi>>0)) < math.Float32frombits(uint32(x2Hi>>0)),
					math.Float32frombits(uint32(x1Hi>>32)) < math.Float32frombits(uint32(x2Hi>>32)),
				}
			case wazeroir.V128CmpTypeF32x4Gt:
				result = []bool{
					math.Float32frombits(uint32(x1Lo>>0)) > math.Float32frombits(uint32(x2Lo>>0)),
					math.Float32frombits(uint32(x1Lo>>32)) > math.Float32frombits(uint32(x2Lo>>32)),
					math.Float32frombits(uint32(x1Hi>>0)) > math.Float32frombits(uint32(x2Hi>>0)),
					math.Float32frombits(uint32(x1Hi>>32)) > math.Float32frombits(uint32(x2Hi>>32)),
				}
			case wazeroir.V128CmpTypeF32x4Le:
				result = []bool{
					math.Float32frombits(uint32(x1Lo>>0)) <= math.Float32frombits(uint32(x2Lo>>0)),
					math.Float32frombits(uint32(x1Lo>>32)) <= math.Float32frombits(uint32(x2Lo>>32)),
					math.Float32frombits(uint32(x1Hi>>0)) <= math.Float32frombits(uint32(x2Hi>>0)),
					math.Float32frombits(uint32(x1Hi>>32)) <= math.Float32frombits(uint32(x2Hi>>32)),
				}
			case wazeroir.V128CmpTypeF32x4Ge:
				result = []bool{
					math.Float32frombits(uint32(x1Lo>>0)) >= math.Float32frombits(uint32(x2Lo>>0)),
					math.Float32frombits(uint32(x1Lo>>32)) >= math.Float32frombits(uint32(x2Lo>>32)),
					math.Float32frombits(uint32(x1Hi>>0)) >= math.Float32frombits(uint32(x2Hi>>0)),
					math.Float32frombits(uint32(x1Hi>>32)) >= math.Float32frombits(uint32(x2Hi>>32)),
				}
			case wazeroir.V128CmpTypeF64x2Eq:
				result = []bool{
					math.Float64frombits(x1Lo) == math.Float64frombits(x2Lo),
					math.Float64frombits(x1Hi) == math.Float64frombits(x2Hi),
				}
			case wazeroir.V128CmpTypeF64x2Ne:
				result = []bool{
					math.Float64frombits(x1Lo) != math.Float64frombits(x2Lo),
					math.Float64frombits(x1Hi) != math.Float64frombits(x2Hi),
				}
			case wazeroir.V128CmpTypeF64x2Lt:
				result = []bool{
					math.Float64frombits(x1Lo) < math.Float64frombits(x2Lo),
					math.Float64frombits(x1Hi) < math.Float64frombits(x2Hi),
				}
			case wazeroir.V128CmpTypeF64x2Gt:
				result = []bool{
					math.Float64frombits(x1Lo) > math.Float64frombits(x2Lo),
					math.Float64frombits(x1Hi) > math.Float64frombits(x2Hi),
				}
			case wazeroir.V128CmpTypeF64x2Le:
				result = []bool{
					math.Float64frombits(x1Lo) <= math.Float64frombits(x2Lo),
					math.Float64frombits(x1Hi) <= math.Float64frombits(x2Hi),
				}
			case wazeroir.V128CmpTypeF64x2Ge:
				result = []bool{
					math.Float64frombits(x1Lo) >= math.Float64frombits(x2Lo),
					math.Float64frombits(x1Hi) >= math.Float64frombits(x2Hi),
				}
			}

			var retLo, retHi uint64
			laneNum := len(result)
			switch laneNum {
			case 16:
				for i, b := range result {
					if b {
						if i < 8 {
							retLo |= 0xff << (i * 8)
						} else {
							retHi |= 0xff << ((i - 8) * 8)
						}
					}
				}
			case 8:
				for i, b := range result {
					if b {
						if i < 4 {
							retLo |= 0xffff << (i * 16)
						} else {
							retHi |= 0xffff << ((i - 4) * 16)
						}
					}
				}
			case 4:
				for i, b := range result {
					if b {
						if i < 2 {
							retLo |= 0xffff_ffff << (i * 32)
						} else {
							retHi |= 0xffff_ffff << ((i - 2) * 32)
						}
					}
				}
			case 2:
				if result[0] {
					retLo = ^uint64(0)
				}
				if result[1] {
					retHi = ^uint64(0)
				}
			}

			ce.pushValue(retLo)
			ce.pushValue(retHi)
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
