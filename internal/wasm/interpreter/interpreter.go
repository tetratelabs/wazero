package interpreter

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"reflect"
	"runtime/debug"
	"strings"

	"github.com/tetratelabs/wazero/internal/moremath"
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/buildoptions"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

var callStackCeiling = buildoptions.CallStackCeiling

// engine is an interpreter implementation of internalwasm.Engine
type engine struct {
	// Stores compiled functions.
	functions map[wasm.FunctionAddress]*interpreterFunction
	// stack contains the operands.
	// Note that all the values are represented as uint64.
	stack []uint64
	// Function call stack.
	frames []*interpreterFrame
	// onCompilationDoneCallbacks call back when a function instance is compiled.
	// See the comment where this is used below for detail.
	// Not used at runtime, and only in the compilation phase.
	onCompilationDoneCallbacks map[wasm.FunctionAddress][]func(*interpreterFunction)
}

func NewEngine() wasm.Engine {
	return &engine{
		functions:                  map[wasm.FunctionAddress]*interpreterFunction{},
		onCompilationDoneCallbacks: map[wasm.FunctionAddress][]func(*interpreterFunction){},
	}
}

func (e *engine) push(v uint64) {
	e.stack = append(e.stack, v)
}

func (e *engine) pop() (v uint64) {
	// No need to check stack bound
	// as we can assume that all the operations
	// are valid thanks to validateFunction
	// at module validation phase
	// and wazeroir translation
	// before compilation.
	v = e.stack[len(e.stack)-1]
	e.stack = e.stack[:len(e.stack)-1]
	return
}

func (e *engine) drop(r *wazeroir.InclusiveRange) {
	// No need to check stack bound
	// as we can assume that all the operations
	// are valid thanks to validateFunction
	// at module validation phase
	// and wazeroir translation
	// before compilation.
	if r == nil {
		return
	} else if r.Start == 0 {
		e.stack = e.stack[:len(e.stack)-1-r.End]
	} else {
		newStack := e.stack[:len(e.stack)-1-r.End]
		newStack = append(newStack, e.stack[len(e.stack)-r.Start:]...)
		e.stack = newStack
	}
}

func (e *engine) pushFrame(frame *interpreterFrame) {
	if callStackCeiling <= len(e.frames) {
		panic(wasm.ErrRuntimeCallStackOverflow)
	}
	e.frames = append(e.frames, frame)
}

func (e *engine) popFrame() (frame *interpreterFrame) {
	// No need to check stack bound as we can assume that all the operations are valid thanks to validateFunction at
	// module validation phase and wazeroir translation before compilation.
	oneLess := len(e.frames) - 1
	frame = e.frames[oneLess]
	e.frames = e.frames[:oneLess]
	return
}

type interpreterFrame struct {
	// Program counter representing the current postion
	// in the f.body.
	pc uint64
	// The compiled function used in this function frame.
	f *interpreterFunction
}

type interpreterFunction struct {
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
	f      *interpreterFunction
}

// Compile Implements wasm.Engine for engine.
func (e *engine) Compile(f *wasm.FunctionInstance) error {
	funcaddr := f.Address

	if f.FunctionKind == wasm.FunctionKindWasm {
		ir, err := wazeroir.Compile(f)
		if err != nil {
			return fmt.Errorf("failed to compile Wasm to wazeroir: %w", err)
		}

		fn, err := e.lowerIROps(f, ir.Operations)
		if err != nil {
			return fmt.Errorf("failed to convert wazeroir operations to engine ones: %w", err)
		}
		e.functions[funcaddr] = fn
		for _, cb := range e.onCompilationDoneCallbacks[funcaddr] {
			cb(fn)
		}
		delete(e.onCompilationDoneCallbacks, funcaddr)
	} else {
		ret := &interpreterFunction{
			hostFn: f.HostFunction, funcInstance: f,
		}
		e.functions[funcaddr] = ret
		return nil
	}
	return nil
}

// Lowers the wazeroir operations to engine friendly struct.
func (e *engine) lowerIROps(f *wasm.FunctionInstance,
	ops []wazeroir.Operation) (*interpreterFunction, error) {
	ret := &interpreterFunction{funcInstance: f}
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
			// We just ignore the lable operation
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
			target := f.ModuleInstance.Functions[o.FunctionIndex]
			compiledTarget, ok := e.functions[target.Address]
			if !ok {
				// If the target function instance is not compiled,
				// we set the callback so we can set the pointer to the target when the compilation done.
				e.onCompilationDoneCallbacks[target.Address] = append(e.onCompilationDoneCallbacks[target.Address],
					func(compiled *interpreterFunction) {
						op.f = compiled
					})
			} else {
				op.f = compiledTarget
			}
		case *wazeroir.OperationCallIndirect:
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.TableIndex)
			op.us[1] = uint64(f.ModuleInstance.Types[o.TypeIndex].TypeID)
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

// Call implements an interpreted wasm.Engine.
func (e *engine) Call(ctx *wasm.ModuleContext, f *wasm.FunctionInstance, params ...uint64) (results []uint64, err error) {
	paramSignature := f.FunctionType.Type.Params
	paramCount := len(params)
	if len(paramSignature) != paramCount {
		return nil, fmt.Errorf("expected %d params, but passed %d", len(paramSignature), paramCount)
	}

	prevFrameLen := len(e.frames)

	// shouldRecover is true when a panic at the origin of callstack should be recovered
	//
	// If this is the recursive call into Wasm (prevFrameLen != 0), we do not recover, and delegate the
	// recovery to the first engine.Call.
	//
	// For example, given the call stack:
	//	 "original host function" --(engine.Call)--> Wasm func A --> Host func --(engine.Call)--> Wasm function B,
	// if the top Wasm function panics, we go back to the "original host function".
	shouldRecover := prevFrameLen == 0
	defer func() {
		if shouldRecover {
			if v := recover(); v != nil {
				if buildoptions.IsDebugMode {
					debug.PrintStack()
				}
				traceNum := len(e.frames) - prevFrameLen
				traces := make([]string, 0, traceNum)
				for i := 0; i < traceNum; i++ {
					frame := e.popFrame()
					name := frame.f.funcInstance.Name
					// TODO: include the original instruction which corresponds
					// to frame.f.body[frame.pc].
					traces = append(traces, fmt.Sprintf("\t%d: %s", i, name))
				}

				e.frames = e.frames[:prevFrameLen]
				err2, ok := v.(error)
				if ok {
					if err2.Error() == "runtime error: integer divide by zero" {
						err2 = wasm.ErrRuntimeIntegerDivideByZero
					}
					err = fmt.Errorf("wasm runtime error: %w", err2)
				} else {
					err = fmt.Errorf("wasm runtime error: %v", v)
				}

				if len(traces) > 0 {
					err = fmt.Errorf("%w\nwasm backtrace:\n%s", err, strings.Join(traces, "\n"))
				}
			}
		}
	}()

	g, ok := e.functions[f.Address]
	if !ok {
		err = fmt.Errorf("function not compiled")
		return
	}

	for _, param := range params {
		e.push(param)
	}
	if g.hostFn != nil {
		e.callHostFunc(ctx, g)
	} else {
		e.callNativeFunc(ctx, g)
	}
	results = make([]uint64, len(f.FunctionType.Type.Results))
	for i := range results {
		results[len(results)-1-i] = e.pop()
	}
	return
}

func (e *engine) callHostFunc(ctx *wasm.ModuleContext, f *interpreterFunction) {
	tp := f.hostFn.Type()
	in := make([]reflect.Value, tp.NumIn())

	wasmParamOffset := 0
	if f.funcInstance.FunctionKind != wasm.FunctionKindGoNoContext {
		wasmParamOffset = 1
	}
	for i := len(in) - 1; i >= wasmParamOffset; i-- {
		val := reflect.New(tp.In(i)).Elem()
		raw := e.pop()
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
	if len(e.frames) > 0 {
		ctx = ctx.WithMemory(e.frames[len(e.frames)-1].f.funcInstance.ModuleInstance.MemoryInstance)
	}

	// Handle any special parameter zero
	if val := wasm.GetHostFunctionCallContextValue(f.funcInstance.FunctionKind, ctx); val != nil {
		in[0] = *val
	}

	frame := &interpreterFrame{f: f}
	e.pushFrame(frame)
	for _, ret := range f.hostFn.Call(in) {
		switch ret.Kind() {
		case reflect.Float32:
			e.push(uint64(math.Float32bits(float32(ret.Float()))))
		case reflect.Float64:
			e.push(math.Float64bits(ret.Float()))
		case reflect.Uint32, reflect.Uint64:
			e.push(ret.Uint())
		case reflect.Int32, reflect.Int64:
			e.push(uint64(ret.Int()))
		default:
			panic("invalid return type")
		}
	}
	e.popFrame()
}

func (e *engine) callNativeFunc(ctx *wasm.ModuleContext, f *interpreterFunction) {
	frame := &interpreterFrame{f: f}
	moduleInst := f.funcInstance.ModuleInstance
	memoryInst := moduleInst.MemoryInstance
	globals := moduleInst.Globals
	var table *wasm.TableInstance
	if len(moduleInst.Tables) > 0 {
		table = moduleInst.Tables[0] // WebAssembly 1.0 (MVP) defines at most one table
	}
	e.pushFrame(frame)
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
				if e.pop() > 0 {
					e.drop(op.rs[0])
					frame.pc = op.us[0]
				} else {
					e.drop(op.rs[1])
					frame.pc = op.us[1]
				}
			}
		case wazeroir.OperationKindBrTable:
			{
				if v := int(e.pop()); v < len(op.us)-1 {
					e.drop(op.rs[v+1])
					frame.pc = op.us[v+1]
				} else {
					// Default branch.
					e.drop(op.rs[0])
					frame.pc = op.us[0]
				}
			}
		case wazeroir.OperationKindCall:
			{
				if op.f.hostFn != nil {
					e.callHostFunc(ctx, op.f)
				} else {
					e.callNativeFunc(ctx, op.f)
				}
				frame.pc++
			}
		case wazeroir.OperationKindCallIndirect:
			{
				offset := e.pop()
				if offset >= uint64(len(table.Table)) {
					panic(wasm.ErrRuntimeInvalidTableAccess)
				}
				tableElement := table.Table[offset]
				// Type check.
				if uint64(tableElement.FunctionTypeID) != op.us[1] {
					if tableElement.FunctionTypeID == wasm.UninitializedTableElementTypeID {
						panic(wasm.ErrRuntimeInvalidTableAccess)
					}
					panic(wasm.ErrRuntimeIndirectCallTypeMismatch)
				}
				target := e.functions[table.Table[offset].FunctionAddress]
				// Call in.
				if target.hostFn != nil {
					e.callHostFunc(ctx, target)
				} else {
					e.callNativeFunc(ctx, target)
				}
				frame.pc++
			}
		case wazeroir.OperationKindDrop:
			{
				e.drop(op.rs[0])
				frame.pc++
			}
		case wazeroir.OperationKindSelect:
			{
				c := e.pop()
				v2 := e.pop()
				if c == 0 {
					_ = e.pop()
					e.push(v2)
				}
				frame.pc++
			}
		case wazeroir.OperationKindPick:
			{
				e.push(e.stack[len(e.stack)-1-int(op.us[0])])
				frame.pc++
			}
		case wazeroir.OperationKindSwap:
			{
				index := len(e.stack) - 1 - int(op.us[0])
				e.stack[len(e.stack)-1], e.stack[index] = e.stack[index], e.stack[len(e.stack)-1]
				frame.pc++
			}
		case wazeroir.OperationKindGlobalGet:
			{
				g := globals[op.us[0]]
				e.push(g.Val)
				frame.pc++
			}
		case wazeroir.OperationKindGlobalSet:
			{
				g := globals[op.us[0]]
				g.Val = e.pop()
				frame.pc++
			}
		case wazeroir.OperationKindLoad:
			{
				base := op.us[1] + e.pop()
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeF32:
					if uint64(len(memoryInst.Buffer)) < base+4 {
						panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
					}
					e.push(uint64(binary.LittleEndian.Uint32(memoryInst.Buffer[base:])))
				case wazeroir.UnsignedTypeI64, wazeroir.UnsignedTypeF64:
					if uint64(len(memoryInst.Buffer)) < base+8 {
						panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
					}
					e.push(binary.LittleEndian.Uint64(memoryInst.Buffer[base:]))
				}
				frame.pc++
			}
		case wazeroir.OperationKindLoad8:
			{
				base := op.us[1] + e.pop()
				if uint64(len(memoryInst.Buffer)) < base+1 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				switch wazeroir.SignedInt(op.b1) {
				case wazeroir.SignedInt32, wazeroir.SignedInt64:
					e.push(uint64(int8(memoryInst.Buffer[base])))
				case wazeroir.SignedUint32, wazeroir.SignedUint64:
					e.push(uint64(uint8(memoryInst.Buffer[base])))
				}
				frame.pc++
			}
		case wazeroir.OperationKindLoad16:
			{
				base := op.us[1] + e.pop()
				if uint64(len(memoryInst.Buffer)) < base+2 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				switch wazeroir.SignedInt(op.b1) {
				case wazeroir.SignedInt32, wazeroir.SignedInt64:
					e.push(uint64(int16(binary.LittleEndian.Uint16(memoryInst.Buffer[base:]))))
				case wazeroir.SignedUint32, wazeroir.SignedUint64:
					e.push(uint64(binary.LittleEndian.Uint16(memoryInst.Buffer[base:])))
				}
				frame.pc++
			}
		case wazeroir.OperationKindLoad32:
			{
				base := op.us[1] + e.pop()
				if uint64(len(memoryInst.Buffer)) < base+4 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				if op.b1 == 1 {
					e.push(uint64(int32(binary.LittleEndian.Uint32(memoryInst.Buffer[base:]))))
				} else {
					e.push(uint64(binary.LittleEndian.Uint32(memoryInst.Buffer[base:])))
				}
				frame.pc++
			}
		case wazeroir.OperationKindStore:
			{
				val := e.pop()
				base := op.us[1] + e.pop()
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
				val := byte(e.pop())
				base := op.us[1] + e.pop()
				if uint64(len(memoryInst.Buffer)) < base+1 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				memoryInst.Buffer[base] = val
				frame.pc++
			}
		case wazeroir.OperationKindStore16:
			{
				val := uint16(e.pop())
				base := op.us[1] + e.pop()
				if uint64(len(memoryInst.Buffer)) < base+2 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				binary.LittleEndian.PutUint16(memoryInst.Buffer[base:], val)
				frame.pc++
			}
		case wazeroir.OperationKindStore32:
			{
				val := uint32(e.pop())
				base := op.us[1] + e.pop()
				if uint64(len(memoryInst.Buffer)) < base+4 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				binary.LittleEndian.PutUint32(memoryInst.Buffer[base:], val)
				frame.pc++
			}
		case wazeroir.OperationKindMemorySize:
			{
				e.push(uint64(memoryInst.PageSize()))
				frame.pc++
			}
		case wazeroir.OperationKindMemoryGrow:
			{
				n := e.pop()
				res := memoryInst.Grow(uint32(n))
				e.push(uint64(res))
				frame.pc++
			}
		case wazeroir.OperationKindConstI32, wazeroir.OperationKindConstI64,
			wazeroir.OperationKindConstF32, wazeroir.OperationKindConstF64:
			{
				e.push(op.us[0])
				frame.pc++
			}
		case wazeroir.OperationKindEq:
			{
				var b bool
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeI64:
					v2, v1 := e.pop(), e.pop()
					b = v1 == v2
				case wazeroir.UnsignedTypeF32:
					v2, v1 := e.pop(), e.pop()
					b = math.Float32frombits(uint32(v2)) == math.Float32frombits(uint32(v1))
				case wazeroir.UnsignedTypeF64:
					v2, v1 := e.pop(), e.pop()
					b = math.Float64frombits(v2) == math.Float64frombits(v1)
				}
				if b {
					e.push(1)
				} else {
					e.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindNe:
			{
				var b bool
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeI64:
					v2, v1 := e.pop(), e.pop()
					b = v1 != v2
				case wazeroir.UnsignedTypeF32:
					v2, v1 := e.pop(), e.pop()
					b = math.Float32frombits(uint32(v2)) != math.Float32frombits(uint32(v1))
				case wazeroir.UnsignedTypeF64:
					v2, v1 := e.pop(), e.pop()
					b = math.Float64frombits(v2) != math.Float64frombits(v1)
				}
				if b {
					e.push(1)
				} else {
					e.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindEqz:
			{
				if e.pop() == 0 {
					e.push(1)
				} else {
					e.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindLt:
			{
				v2 := e.pop()
				v1 := e.pop()
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
					e.push(1)
				} else {
					e.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindGt:
			{
				v2 := e.pop()
				v1 := e.pop()
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
					e.push(1)
				} else {
					e.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindLe:
			{
				v2 := e.pop()
				v1 := e.pop()
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
					e.push(1)
				} else {
					e.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindGe:
			{
				v2 := e.pop()
				v1 := e.pop()
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
					e.push(1)
				} else {
					e.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindAdd:
			{
				v2 := e.pop()
				v1 := e.pop()
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32:
					v := uint32(v1) + uint32(v2)
					e.push(uint64(v))
				case wazeroir.UnsignedTypeI64:
					e.push(v1 + v2)
				case wazeroir.UnsignedTypeF32:
					v := math.Float32frombits(uint32(v1)) + math.Float32frombits(uint32(v2))
					e.push(uint64(math.Float32bits(v)))
				case wazeroir.UnsignedTypeF64:
					v := math.Float64frombits(v1) + math.Float64frombits(v2)
					e.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindSub:
			{
				v2 := e.pop()
				v1 := e.pop()
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32:
					e.push(uint64(uint32(v1) - uint32(v2)))
				case wazeroir.UnsignedTypeI64:
					e.push(v1 - v2)
				case wazeroir.UnsignedTypeF32:
					v := math.Float32frombits(uint32(v1)) - math.Float32frombits(uint32(v2))
					e.push(uint64(math.Float32bits(v)))
				case wazeroir.UnsignedTypeF64:
					v := math.Float64frombits(v1) - math.Float64frombits(v2)
					e.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindMul:
			{
				v2 := e.pop()
				v1 := e.pop()
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32:
					e.push(uint64(uint32(v1) * uint32(v2)))
				case wazeroir.UnsignedTypeI64:
					e.push(v1 * v2)
				case wazeroir.UnsignedTypeF32:
					v := math.Float32frombits(uint32(v2)) * math.Float32frombits(uint32(v1))
					e.push(uint64(math.Float32bits(v)))
				case wazeroir.UnsignedTypeF64:
					v := math.Float64frombits(v2) * math.Float64frombits(v1)
					e.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindClz:
			{
				v := e.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					e.push(uint64(bits.LeadingZeros32(uint32(v))))
				} else {
					// UnsignedInt64
					e.push(uint64(bits.LeadingZeros64(v)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindCtz:
			{
				v := e.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					e.push(uint64(bits.TrailingZeros32(uint32(v))))
				} else {
					// UnsignedInt64
					e.push(uint64(bits.TrailingZeros64(v)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindPopcnt:
			{
				v := e.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					e.push(uint64(bits.OnesCount32(uint32(v))))
				} else {
					// UnsignedInt64
					e.push(uint64(bits.OnesCount64(v)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindDiv:
			{
				switch wazeroir.SignedType(op.b1) {
				case wazeroir.SignedTypeInt32:
					v2 := int32(e.pop())
					v1 := int32(e.pop())
					if v1 == math.MinInt32 && v2 == -1 {
						panic(wasm.ErrRuntimeIntegerOverflow)
					}
					e.push(uint64(uint32(v1 / v2)))
				case wazeroir.SignedTypeInt64:
					v2 := int64(e.pop())
					v1 := int64(e.pop())
					if v1 == math.MinInt64 && v2 == -1 {
						panic(wasm.ErrRuntimeIntegerOverflow)
					}
					e.push(uint64(v1 / v2))
				case wazeroir.SignedTypeUint32:
					v2 := uint32(e.pop())
					v1 := uint32(e.pop())
					e.push(uint64(v1 / v2))
				case wazeroir.SignedTypeUint64:
					v2 := e.pop()
					v1 := e.pop()
					e.push(v1 / v2)
				case wazeroir.SignedTypeFloat32:
					v2 := e.pop()
					v1 := e.pop()
					v := math.Float32frombits(uint32(v1)) / math.Float32frombits(uint32(v2))
					e.push(uint64(math.Float32bits(v)))
				case wazeroir.SignedTypeFloat64:
					v2 := e.pop()
					v1 := e.pop()
					v := math.Float64frombits(v1) / math.Float64frombits(v2)
					e.push(uint64(math.Float64bits(v)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindRem:
			{
				switch wazeroir.SignedInt(op.b1) {
				case wazeroir.SignedInt32:
					v2 := int32(e.pop())
					v1 := int32(e.pop())
					e.push(uint64(uint32(v1 % v2)))
				case wazeroir.SignedInt64:
					v2 := int64(e.pop())
					v1 := int64(e.pop())
					e.push(uint64(v1 % v2))
				case wazeroir.SignedUint32:
					v2 := uint32(e.pop())
					v1 := uint32(e.pop())
					e.push(uint64(v1 % v2))
				case wazeroir.SignedUint64:
					v2 := e.pop()
					v1 := e.pop()
					e.push(v1 % v2)
				}
				frame.pc++
			}
		case wazeroir.OperationKindAnd:
			{
				v2 := e.pop()
				v1 := e.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					e.push(uint64(uint32(v2) & uint32(v1)))
				} else {
					// UnsignedInt64
					e.push(uint64(v2 & v1))
				}
				frame.pc++
			}
		case wazeroir.OperationKindOr:
			{
				v2 := e.pop()
				v1 := e.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					e.push(uint64(uint32(v2) | uint32(v1)))
				} else {
					// UnsignedInt64
					e.push(uint64(v2 | v1))
				}
				frame.pc++
			}
		case wazeroir.OperationKindXor:
			{
				v2 := e.pop()
				v1 := e.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					e.push(uint64(uint32(v2) ^ uint32(v1)))
				} else {
					// UnsignedInt64
					e.push(uint64(v2 ^ v1))
				}
				frame.pc++
			}
		case wazeroir.OperationKindShl:
			{
				v2 := e.pop()
				v1 := e.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					e.push(uint64(uint32(v1) << (uint32(v2) % 32)))
				} else {
					// UnsignedInt64
					e.push(v1 << (v2 % 64))
				}
				frame.pc++
			}
		case wazeroir.OperationKindShr:
			{
				v2 := e.pop()
				v1 := e.pop()
				switch wazeroir.SignedInt(op.b1) {
				case wazeroir.SignedInt32:
					e.push(uint64(int32(v1) >> (uint32(v2) % 32)))
				case wazeroir.SignedInt64:
					e.push(uint64(int64(v1) >> (v2 % 64)))
				case wazeroir.SignedUint32:
					e.push(uint64(uint32(v1) >> (uint32(v2) % 32)))
				case wazeroir.SignedUint64:
					e.push(v1 >> (v2 % 64))
				}
				frame.pc++
			}
		case wazeroir.OperationKindRotl:
			{
				v2 := e.pop()
				v1 := e.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					e.push(uint64(bits.RotateLeft32(uint32(v1), int(v2))))
				} else {
					// UnsignedInt64
					e.push(uint64(bits.RotateLeft64(v1, int(v2))))
				}
				frame.pc++
			}
		case wazeroir.OperationKindRotr:
			{
				v2 := e.pop()
				v1 := e.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					e.push(uint64(bits.RotateLeft32(uint32(v1), -int(v2))))
				} else {
					// UnsignedInt64
					e.push(uint64(bits.RotateLeft64(v1, -int(v2))))
				}
				frame.pc++
			}
		case wazeroir.OperationKindAbs:
			{
				if op.b1 == 0 {
					// Float32
					const mask uint32 = 1 << 31
					e.push(uint64(uint32(e.pop()) &^ mask))
				} else {
					// Float64
					const mask uint64 = 1 << 63
					e.push(uint64(e.pop() &^ mask))
				}
				frame.pc++
			}
		case wazeroir.OperationKindNeg:
			{
				if op.b1 == 0 {
					// Float32
					v := -math.Float32frombits(uint32(e.pop()))
					e.push(uint64(math.Float32bits(v)))
				} else {
					// Float64
					v := -math.Float64frombits(e.pop())
					e.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindCeil:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Ceil(float64(math.Float32frombits(uint32(e.pop()))))
					e.push(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Ceil(float64(math.Float64frombits(e.pop())))
					e.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindFloor:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Floor(float64(math.Float32frombits(uint32(e.pop()))))
					e.push(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Floor(float64(math.Float64frombits(e.pop())))
					e.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindTrunc:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Trunc(float64(math.Float32frombits(uint32(e.pop()))))
					e.push(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Trunc(float64(math.Float64frombits(e.pop())))
					e.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindNearest:
			{
				if op.b1 == 0 {
					// Float32
					f := math.Float32frombits(uint32(e.pop()))
					e.push(uint64(math.Float32bits(moremath.WasmCompatNearestF32(f))))
				} else {
					// Float64
					f := math.Float64frombits(e.pop())
					e.push(math.Float64bits(moremath.WasmCompatNearestF64(f)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindSqrt:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Sqrt(float64(math.Float32frombits(uint32(e.pop()))))
					e.push(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Sqrt(float64(math.Float64frombits(e.pop())))
					e.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindMin:
			{
				if op.b1 == 0 {
					// Float32
					v2 := math.Float32frombits(uint32(e.pop()))
					v1 := math.Float32frombits(uint32(e.pop()))
					e.push(uint64(math.Float32bits(float32(moremath.WasmCompatMin(float64(v1), float64(v2))))))
				} else {
					v2 := math.Float64frombits(e.pop())
					v1 := math.Float64frombits(e.pop())
					e.push(math.Float64bits(moremath.WasmCompatMin(v1, v2)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindMax:
			{

				if op.b1 == 0 {
					// Float32
					v2 := math.Float32frombits(uint32(e.pop()))
					v1 := math.Float32frombits(uint32(e.pop()))
					e.push(uint64(math.Float32bits(float32(moremath.WasmCompatMax(float64(v1), float64(v2))))))
				} else {
					// Float64
					v2 := math.Float64frombits(e.pop())
					v1 := math.Float64frombits(e.pop())
					e.push(math.Float64bits(moremath.WasmCompatMax(v1, v2)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindCopysign:
			{
				if op.b1 == 0 {
					// Float32
					v2 := uint32(e.pop())
					v1 := uint32(e.pop())
					const signbit = 1 << 31
					e.push(uint64(v1&^signbit | v2&signbit))
				} else {
					// Float64
					v2 := e.pop()
					v1 := e.pop()
					const signbit = 1 << 63
					e.push(v1&^signbit | v2&signbit)
				}
				frame.pc++
			}
		case wazeroir.OperationKindI32WrapFromI64:
			{
				e.push(uint64(uint32(e.pop())))
				frame.pc++
			}
		case wazeroir.OperationKindITruncFromF:
			{
				if op.b1 == 0 {
					// Float32
					switch wazeroir.SignedInt(op.b2) {
					case wazeroir.SignedInt32:
						v := math.Trunc(float64(math.Float32frombits(uint32(e.pop()))))
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < math.MinInt32 || v > math.MaxInt32 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						e.push(uint64(int32(v)))
					case wazeroir.SignedInt64:
						v := math.Trunc(float64(math.Float32frombits(uint32(e.pop()))))
						res := int64(v)
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < math.MinInt64 || v >= math.MaxInt64 {
							// Note: math.MaxInt64 is rounded up to math.MaxInt64+1 in 64-bit float representation,
							// and that's why we use '>=' not '>' to check overflow.
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						e.push(uint64(res))
					case wazeroir.SignedUint32:
						v := math.Trunc(float64(math.Float32frombits(uint32(e.pop()))))
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < 0 || v > math.MaxUint32 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						e.push(uint64(uint32(v)))
					case wazeroir.SignedUint64:
						v := math.Trunc(float64(math.Float32frombits(uint32(e.pop()))))
						res := uint64(v)
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < 0 || v >= math.MaxUint64 {
							// Note: math.MaxUint64 is rounded up to math.MaxUint64+1 in 64-bit float representation,
							// and that's why we use '>=' not '>' to check overflow.
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						e.push(res)
					}
				} else {
					// Float64
					switch wazeroir.SignedInt(op.b2) {
					case wazeroir.SignedInt32:
						v := math.Trunc(math.Float64frombits(e.pop()))
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < math.MinInt32 || v > math.MaxInt32 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						e.push(uint64(int32(v)))
					case wazeroir.SignedInt64:
						v := math.Trunc(math.Float64frombits(e.pop()))
						res := int64(v)
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < math.MinInt64 || v >= math.MaxInt64 {
							// Note: math.MaxInt64 is rounded up to math.MaxInt64+1 in 64-bit float representation,
							// and that's why we use '>=' not '>' to check overflow.
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						e.push(uint64(res))
					case wazeroir.SignedUint32:
						v := math.Trunc(math.Float64frombits(e.pop()))
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < 0 || v > math.MaxUint32 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						e.push(uint64(uint32(v)))
					case wazeroir.SignedUint64:
						v := math.Trunc(math.Float64frombits(e.pop()))
						res := uint64(v)
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < 0 || v >= math.MaxUint64 {
							// Note: math.MaxUint64 is rounded up to math.MaxUint64+1 in 64-bit float representation,
							// and that's why we use '>=' not '>' to check overflow.
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						e.push(res)
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
						v := float32(int32(e.pop()))
						e.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(int32(e.pop()))
						e.push(math.Float64bits(v))
					}
				case wazeroir.SignedInt64:
					if op.b2 == 0 {
						// Float32
						v := float32(int64(e.pop()))
						e.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(int64(e.pop()))
						e.push(math.Float64bits(v))
					}
				case wazeroir.SignedUint32:
					if op.b2 == 0 {
						// Float32
						v := float32(uint32(e.pop()))
						e.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(uint32(e.pop()))
						e.push(math.Float64bits(v))
					}
				case wazeroir.SignedUint64:
					if op.b2 == 0 {
						// Float32
						v := float32(e.pop())
						e.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(e.pop())
						e.push(math.Float64bits(v))
					}
				}
				frame.pc++
			}
		case wazeroir.OperationKindF32DemoteFromF64:
			{
				v := float32(math.Float64frombits(e.pop()))
				e.push(uint64(math.Float32bits(v)))
				frame.pc++
			}
		case wazeroir.OperationKindF64PromoteFromF32:
			{
				v := float64(math.Float32frombits(uint32(e.pop())))
				e.push(math.Float64bits(v))
				frame.pc++
			}
		case wazeroir.OperationKindExtend:
			{
				if op.b1 == 1 {
					// Signed.
					v := int64(int32(e.pop()))
					e.push(uint64(v))
				} else {
					v := uint64(uint32(e.pop()))
					e.push(v)
				}
				frame.pc++
			}
		}
	}
	e.popFrame()
}
