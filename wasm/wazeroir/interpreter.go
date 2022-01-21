package wazeroir

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"reflect"
	"runtime/debug"
	"strings"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/buildoptions"
)

var callStackCeiling = buildoptions.CallStackCeiling

// interpreter implements wasm.Engine interface.
// This is the direct interpreter of wazeroir operations.
type interpreter struct {
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
	return &interpreter{
		functions:                  map[wasm.FunctionAddress]*interpreterFunction{},
		onCompilationDoneCallbacks: map[wasm.FunctionAddress][]func(*interpreterFunction){},
	}
}

func (it *interpreter) push(v uint64) {
	it.stack = append(it.stack, v)
}

func (it *interpreter) pop() (v uint64) {
	// No need to check stack bound
	// as we can assume that all the operations
	// are valid thanks to validateFunction
	// at module validation phase
	// and wazeroir translation
	// before compilation.
	v = it.stack[len(it.stack)-1]
	it.stack = it.stack[:len(it.stack)-1]
	return
}

func (it *interpreter) drop(r *InclusiveRange) {
	// No need to check stack bound
	// as we can assume that all the operations
	// are valid thanks to validateFunction
	// at module validation phase
	// and wazeroir translation
	// before compilation.
	if r == nil {
		return
	} else if r.Start == 0 {
		it.stack = it.stack[:len(it.stack)-1-r.End]
	} else {
		newStack := it.stack[:len(it.stack)-1-r.End]
		newStack = append(newStack, it.stack[len(it.stack)-r.Start:]...)
		it.stack = newStack
	}
}

func (it *interpreter) pushFrame(frame *interpreterFrame) {
	if callStackCeiling <= len(it.frames) {
		panic(wasm.ErrRuntimeCallStackOverflow)
	}
	it.frames = append(it.frames, frame)
}

func (it *interpreter) popFrame() (frame *interpreterFrame) {
	// No need to check stack bound as we can assume that all the operations are valid thanks to validateFunction at
	// module validation phase and wazeroir translation before compilation.
	oneLess := len(it.frames) - 1
	frame = it.frames[oneLess]
	it.frames = it.frames[:oneLess]
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
	kind   OperationKind
	b1, b2 byte
	us     []uint64
	rs     []*InclusiveRange
	f      *interpreterFunction
}

// Compile Implements wasm.Engine for interpreter.
func (it *interpreter) Compile(f *wasm.FunctionInstance) error {
	funcaddr := f.Address

	if f.IsHostFunction() {
		ret := &interpreterFunction{
			hostFn: f.HostFunction, funcInstance: f,
		}
		it.functions[funcaddr] = ret
		return nil
	} else {
		ir, err := Compile(f)
		if err != nil {
			return fmt.Errorf("failed to compile Wasm to wazeroir: %w", err)
		}

		fn, err := it.lowerIROps(f, ir.Operations)
		if err != nil {
			return fmt.Errorf("failed to convert wazeroir operations to interpreter ones: %w", err)
		}
		it.functions[funcaddr] = fn
		for _, cb := range it.onCompilationDoneCallbacks[funcaddr] {
			cb(fn)
		}
		delete(it.onCompilationDoneCallbacks, funcaddr)
	}
	return nil
}

// Lowers the wazeroir operations to interpreter friendly struct.
func (it *interpreter) lowerIROps(f *wasm.FunctionInstance,
	ops []Operation) (*interpreterFunction, error) {
	ret := &interpreterFunction{funcInstance: f}
	labelAddress := map[string]uint64{}
	onLabelAddressResolved := map[string][]func(addr uint64){}
	for _, original := range ops {
		op := &interpreterOp{kind: original.Kind()}
		switch o := original.(type) {
		case *OperationUnreachable:
		case *OperationLabel:
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
		case *OperationBr:
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
		case *OperationBrIf:
			op.rs = make([]*InclusiveRange, 2)
			op.us = make([]uint64, 2)
			for i, target := range []*BranchTargetDrop{o.Then, o.Else} {
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
		case *OperationBrTable:
			targets := append([]*BranchTargetDrop{o.Default}, o.Targets...)
			op.rs = make([]*InclusiveRange, len(targets))
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
		case *OperationCall:
			target := f.ModuleInstance.Functions[o.FunctionIndex]
			compiledTarget, ok := it.functions[target.Address]
			if !ok {
				// If the target function instance is not compiled,
				// we set the callback so we can set the pointer to the target when the compilation done.
				it.onCompilationDoneCallbacks[target.Address] = append(it.onCompilationDoneCallbacks[target.Address],
					func(compiled *interpreterFunction) {
						op.f = compiled
					})
			} else {
				op.f = compiledTarget
			}
		case *OperationCallIndirect:
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.TableIndex)
			op.us[1] = uint64(f.ModuleInstance.Types[o.TypeIndex].TypeID)
		case *OperationDrop:
			op.rs = make([]*InclusiveRange, 1)
			op.rs[0] = o.Range
		case *OperationSelect:
		case *OperationPick:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.Depth)
		case *OperationSwap:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.Depth)
		case *OperationGlobalGet:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.Index)
		case *OperationGlobalSet:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.Index)
		case *OperationLoad:
			op.b1 = byte(o.Type)
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offest)
		case *OperationLoad8:
			op.b1 = byte(o.Type)
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offest)
		case *OperationLoad16:
			op.b1 = byte(o.Type)
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offest)
		case *OperationLoad32:
			if o.Signed {
				op.b1 = 1
			}
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offest)
		case *OperationStore:
			op.b1 = byte(o.Type)
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offest)
		case *OperationStore8:
			op.b1 = byte(o.Type)
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offest)
		case *OperationStore16:
			op.b1 = byte(o.Type)
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offest)
		case *OperationStore32:
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.Arg.Alignment)
			op.us[1] = uint64(o.Arg.Offest)
		case *OperationMemorySize:
		case *OperationMemoryGrow:
		case *OperationConstI32:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(o.Value)
		case *OperationConstI64:
			op.us = make([]uint64, 1)
			op.us[0] = o.Value
		case *OperationConstF32:
			op.us = make([]uint64, 1)
			op.us[0] = uint64(math.Float32bits(o.Value))
		case *OperationConstF64:
			op.us = make([]uint64, 1)
			op.us[0] = math.Float64bits(o.Value)
		case *OperationEq:
			op.b1 = byte(o.Type)
		case *OperationNe:
			op.b1 = byte(o.Type)
		case *OperationEqz:
			op.b1 = byte(o.Type)
		case *OperationLt:
			op.b1 = byte(o.Type)
		case *OperationGt:
			op.b1 = byte(o.Type)
		case *OperationLe:
			op.b1 = byte(o.Type)
		case *OperationGe:
			op.b1 = byte(o.Type)
		case *OperationAdd:
			op.b1 = byte(o.Type)
		case *OperationSub:
			op.b1 = byte(o.Type)
		case *OperationMul:
			op.b1 = byte(o.Type)
		case *OperationClz:
			op.b1 = byte(o.Type)
		case *OperationCtz:
			op.b1 = byte(o.Type)
		case *OperationPopcnt:
			op.b1 = byte(o.Type)
		case *OperationDiv:
			op.b1 = byte(o.Type)
		case *OperationRem:
			op.b1 = byte(o.Type)
		case *OperationAnd:
			op.b1 = byte(o.Type)
		case *OperationOr:
			op.b1 = byte(o.Type)
		case *OperationXor:
			op.b1 = byte(o.Type)
		case *OperationShl:
			op.b1 = byte(o.Type)
		case *OperationShr:
			op.b1 = byte(o.Type)
		case *OperationRotl:
			op.b1 = byte(o.Type)
		case *OperationRotr:
			op.b1 = byte(o.Type)
		case *OperationAbs:
			op.b1 = byte(o.Type)
		case *OperationNeg:
			op.b1 = byte(o.Type)
		case *OperationCeil:
			op.b1 = byte(o.Type)
		case *OperationFloor:
			op.b1 = byte(o.Type)
		case *OperationTrunc:
			op.b1 = byte(o.Type)
		case *OperationNearest:
			op.b1 = byte(o.Type)
		case *OperationSqrt:
			op.b1 = byte(o.Type)
		case *OperationMin:
			op.b1 = byte(o.Type)
		case *OperationMax:
			op.b1 = byte(o.Type)
		case *OperationCopysign:
			op.b1 = byte(o.Type)
		case *OperationI32WrapFromI64:
		case *OperationITruncFromF:
			op.b1 = byte(o.InputType)
			op.b2 = byte(o.OutputType)
		case *OperationFConvertFromI:
			op.b1 = byte(o.InputType)
			op.b2 = byte(o.OutputType)
		case *OperationF32DemoteFromF64:
		case *OperationF64PromoteFromF32:
		case *OperationI32ReinterpretFromF32,
			*OperationI64ReinterpretFromF64,
			*OperationF32ReinterpretFromI32,
			*OperationF64ReinterpretFromI64:
			// Reinterpret ops are essentially nop for interpreter mode
			// because we treat all values as uint64, and the reinterpret is only used at module
			// validation phase where we check type soundness of all the operations.
			// So just eliminate the ops.
			continue
		case *OperationExtend:
			if o.Signed {
				op.b1 = 1
			}
		default:
			return nil, fmt.Errorf("unreachable: a bug in wazeroir interpreter")
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
func (it *interpreter) Call(f *wasm.FunctionInstance, params ...uint64) (results []uint64, err error) {
	prevFrameLen := len(it.frames)
	defer func() {
		if v := recover(); v != nil {
			if buildoptions.IsDebugMode {
				debug.PrintStack()
			}
			traceNum := len(it.frames) - prevFrameLen
			traces := make([]string, 0, traceNum)
			for i := 0; i < traceNum; i++ {
				frame := it.popFrame()
				name := frame.f.funcInstance.Name
				// TODO: include the original instruction which corresponds
				// to frame.f.body[frame.pc].
				traces = append(traces, fmt.Sprintf("\t%d: %s", i, name))
			}

			it.frames = it.frames[:prevFrameLen]
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
	}()

	g, ok := it.functions[f.Address]
	if !ok {
		err = fmt.Errorf("function not compiled")
		return
	}

	if g.hostFn != nil {
		it.callHostFunc(g, params...)
	} else {
		for _, param := range params {
			it.push(param)
		}
		it.callNativeFunc(g)
	}
	results = make([]uint64, len(f.FunctionType.Type.Results))
	for i := range results {
		results[len(results)-1-i] = it.pop()
	}
	return
}

func (it *interpreter) callHostFunc(f *interpreterFunction, _ ...uint64) {
	tp := f.hostFn.Type()
	in := make([]reflect.Value, tp.NumIn())
	for i := len(in) - 1; i >= 1; i-- {
		val := reflect.New(tp.In(i)).Elem()
		raw := it.pop()
		kind := tp.In(i).Kind()
		switch kind {
		case reflect.Float64, reflect.Float32:
			val.SetFloat(math.Float64frombits(raw))
		case reflect.Uint32, reflect.Uint64:
			val.SetUint(raw)
		case reflect.Int32, reflect.Int64:
			val.SetInt(int64(raw))
		}
		in[i] = val
	}

	val := reflect.New(tp.In(0)).Elem()
	var memory *wasm.MemoryInstance
	if len(it.frames) > 0 {
		memory = it.frames[len(it.frames)-1].f.funcInstance.ModuleInstance.Memory
	}
	val.Set(reflect.ValueOf(&wasm.HostFunctionCallContext{Memory: memory}))
	in[0] = val

	frame := &interpreterFrame{f: f}
	it.pushFrame(frame)
	for _, ret := range f.hostFn.Call(in) {
		switch ret.Kind() {
		case reflect.Float64, reflect.Float32:
			it.push(math.Float64bits(ret.Float()))
		case reflect.Uint32, reflect.Uint64:
			it.push(ret.Uint())
		case reflect.Int32, reflect.Int64:
			it.push(uint64(ret.Int()))
		default:
			panic("invalid return type")
		}
	}
	it.popFrame()
}

func (it *interpreter) callNativeFunc(f *interpreterFunction) {
	frame := &interpreterFrame{f: f}
	moduleInst := f.funcInstance.ModuleInstance
	memoryInst := moduleInst.Memory
	globals := moduleInst.Globals
	var table *wasm.TableInstance
	if len(moduleInst.Tables) > 0 {
		table = moduleInst.Tables[0] // WebAssembly 1.0 (MVP) defines at most one table
	}
	it.pushFrame(frame)
	bodyLen := uint64(len(frame.f.body))
	for frame.pc < bodyLen {
		op := frame.f.body[frame.pc]
		// TODO: add description of each operation/case
		// on, for example, how many args are used,
		// how the stack is modified, etc.
		switch op.kind {
		case OperationKindUnreachable:
			panic(wasm.ErrRuntimeUnreachable)
		case OperationKindBr:
			{
				frame.pc = op.us[0]
			}
		case OperationKindBrIf:
			{
				if it.pop() > 0 {
					it.drop(op.rs[0])
					frame.pc = op.us[0]
				} else {
					it.drop(op.rs[1])
					frame.pc = op.us[1]
				}
			}
		case OperationKindBrTable:
			{
				if v := int(it.pop()); v < len(op.us)-1 {
					it.drop(op.rs[v+1])
					frame.pc = op.us[v+1]
				} else {
					// Default branch.
					it.drop(op.rs[0])
					frame.pc = op.us[0]
				}
			}
		case OperationKindCall:
			{
				if op.f.hostFn != nil {
					it.callHostFunc(op.f, it.stack[len(it.stack)-len(op.f.funcInstance.FunctionType.Type.Params):]...)
				} else {
					it.callNativeFunc(op.f)
				}
				frame.pc++
			}
		case OperationKindCallIndirect:
			{
				offset := it.pop()
				if offset > uint64(len(table.Table)) {
					panic(wasm.ErrRuntimeOutOfBoundsTableAcces)
				}
				target, ok := it.functions[table.Table[offset].FunctionAddress]
				if !ok {
					panic(wasm.ErrRuntimeOutOfBoundsTableAcces)
				}
				// Type check.
				expType := target.funcInstance.FunctionType
				if uint64(expType.TypeID) != op.us[1] {
					panic(wasm.ErrRuntimeIndirectCallTypeMismatch)
				}
				// Call in.
				if target.hostFn != nil {
					it.callHostFunc(target, it.stack[len(it.stack)-len(expType.Type.Params):]...)
				} else {
					it.callNativeFunc(target)
				}
				frame.pc++
			}
		case OperationKindDrop:
			{
				it.drop(op.rs[0])
				frame.pc++
			}
		case OperationKindSelect:
			{
				c := it.pop()
				v2 := it.pop()
				if c == 0 {
					_ = it.pop()
					it.push(v2)
				}
				frame.pc++
			}
		case OperationKindPick:
			{
				it.push(it.stack[len(it.stack)-1-int(op.us[0])])
				frame.pc++
			}
		case OperationKindSwap:
			{
				index := len(it.stack) - 1 - int(op.us[0])
				it.stack[len(it.stack)-1], it.stack[index] = it.stack[index], it.stack[len(it.stack)-1]
				frame.pc++
			}
		case OperationKindGlobalGet:
			{
				g := globals[op.us[0]]
				it.push(g.Val)
				frame.pc++
			}
		case OperationKindGlobalSet:
			{
				g := globals[op.us[0]]
				g.Val = it.pop()
				frame.pc++
			}
		case OperationKindLoad:
			{
				base := op.us[1] + it.pop()
				switch UnsignedType(op.b1) {
				case UnsignedTypeI32, UnsignedTypeF32:
					it.push(uint64(binary.LittleEndian.Uint32(memoryInst.Buffer[base:])))
				case UnsignedTypeI64, UnsignedTypeF64:
					it.push(binary.LittleEndian.Uint64(memoryInst.Buffer[base:]))
				}
				frame.pc++
			}
		case OperationKindLoad8:
			{
				base := op.us[1] + it.pop()
				switch SignedInt(op.b1) {
				case SignedInt32, SignedInt64:
					it.push(uint64(int8(memoryInst.Buffer[base])))
				case SignedUint32, SignedUint64:
					it.push(uint64(uint8(memoryInst.Buffer[base])))
				}
				frame.pc++
			}
		case OperationKindLoad16:
			{
				base := op.us[1] + it.pop()
				switch SignedInt(op.b1) {
				case SignedInt32, SignedInt64:
					it.push(uint64(int16(binary.LittleEndian.Uint16(memoryInst.Buffer[base:]))))
				case SignedUint32, SignedUint64:
					it.push(uint64(binary.LittleEndian.Uint16(memoryInst.Buffer[base:])))
				}
				frame.pc++
			}
		case OperationKindLoad32:
			{
				base := op.us[1] + it.pop()
				if op.b1 == 1 {
					it.push(uint64(int32(binary.LittleEndian.Uint32(memoryInst.Buffer[base:]))))
				} else {
					it.push(uint64(binary.LittleEndian.Uint32(memoryInst.Buffer[base:])))
				}
				frame.pc++
			}
		case OperationKindStore:
			{
				val := it.pop()
				base := op.us[1] + it.pop()
				switch UnsignedType(op.b1) {
				case UnsignedTypeI32, UnsignedTypeF32:
					binary.LittleEndian.PutUint32(memoryInst.Buffer[base:], uint32(val))
				case UnsignedTypeI64, UnsignedTypeF64:
					binary.LittleEndian.PutUint64(memoryInst.Buffer[base:], val)
				}
				frame.pc++
			}
		case OperationKindStore8:
			{
				val := byte(it.pop())
				base := op.us[1] + it.pop()
				memoryInst.Buffer[base] = val
				frame.pc++
			}
		case OperationKindStore16:
			{
				val := uint16(it.pop())
				base := op.us[1] + it.pop()
				binary.LittleEndian.PutUint16(memoryInst.Buffer[base:], val)
				frame.pc++
			}
		case OperationKindStore32:
			{
				val := uint32(it.pop())
				base := op.us[1] + it.pop()
				binary.LittleEndian.PutUint32(memoryInst.Buffer[base:], val)
				frame.pc++
			}
		case OperationKindMemorySize:
			{
				v := uint64(len(memoryInst.Buffer)) / wasm.PageSize
				it.push(v)
				frame.pc++
			}
		case OperationKindMemoryGrow:
			{
				n := it.pop()
				max := uint64(math.MaxUint32)
				if memoryInst.Max != nil {
					max = uint64(*memoryInst.Max) * wasm.PageSize
				}
				if uint64(n*wasm.PageSize+uint64(len(memoryInst.Buffer))) > max {
					v := int32(-1)
					it.push(uint64(v))
				} else {
					it.push(uint64(len(memoryInst.Buffer)) / wasm.PageSize)
					memoryInst.Buffer = append(memoryInst.Buffer, make([]byte, n*wasm.PageSize)...)
				}
				frame.pc++
			}
		case OperationKindConstI32, OperationKindConstI64,
			OperationKindConstF32, OperationKindConstF64:
			{
				it.push(op.us[0])
				frame.pc++
			}
		case OperationKindEq:
			{
				var b bool
				switch UnsignedType(op.b1) {
				case UnsignedTypeI32, UnsignedTypeI64:
					v2, v1 := it.pop(), it.pop()
					b = v1 == v2
				case UnsignedTypeF32:
					v2, v1 := it.pop(), it.pop()
					b = math.Float32frombits(uint32(v2)) == math.Float32frombits(uint32(v1))
				case UnsignedTypeF64:
					v2, v1 := it.pop(), it.pop()
					b = math.Float64frombits(v2) == math.Float64frombits(v1)
				}
				if b {
					it.push(1)
				} else {
					it.push(0)
				}
				frame.pc++
			}
		case OperationKindNe:
			{
				var b bool
				switch UnsignedType(op.b1) {
				case UnsignedTypeI32, UnsignedTypeI64:
					v2, v1 := it.pop(), it.pop()
					b = v1 != v2
				case UnsignedTypeF32:
					v2, v1 := it.pop(), it.pop()
					b = math.Float32frombits(uint32(v2)) != math.Float32frombits(uint32(v1))
				case UnsignedTypeF64:
					v2, v1 := it.pop(), it.pop()
					b = math.Float64frombits(v2) != math.Float64frombits(v1)
				}
				if b {
					it.push(1)
				} else {
					it.push(0)
				}
				frame.pc++
			}
		case OperationKindEqz:
			{
				if it.pop() == 0 {
					it.push(1)
				} else {
					it.push(0)
				}
				frame.pc++
			}
		case OperationKindLt:
			{
				v2 := it.pop()
				v1 := it.pop()
				var b bool
				switch SignedType(op.b1) {
				case SignedTypeInt32:
					b = int32(v1) < int32(v2)
				case SignedTypeInt64:
					b = int64(v1) < int64(v2)
				case SignedTypeUint32, SignedTypeUint64:
					b = v1 < v2
				case SignedTypeFloat32:
					b = math.Float32frombits(uint32(v1)) < math.Float32frombits(uint32(v2))
				case SignedTypeFloat64:
					b = math.Float64frombits(v1) < math.Float64frombits(v2)
				}
				if b {
					it.push(1)
				} else {
					it.push(0)
				}
				frame.pc++
			}
		case OperationKindGt:
			{
				v2 := it.pop()
				v1 := it.pop()
				var b bool
				switch SignedType(op.b1) {
				case SignedTypeInt32:
					b = int32(v1) > int32(v2)
				case SignedTypeInt64:
					b = int64(v1) > int64(v2)
				case SignedTypeUint32, SignedTypeUint64:
					b = v1 > v2
				case SignedTypeFloat32:
					b = math.Float32frombits(uint32(v1)) > math.Float32frombits(uint32(v2))
				case SignedTypeFloat64:
					b = math.Float64frombits(v1) > math.Float64frombits(v2)
				}
				if b {
					it.push(1)
				} else {
					it.push(0)
				}
				frame.pc++
			}
		case OperationKindLe:
			{
				v2 := it.pop()
				v1 := it.pop()
				var b bool
				switch SignedType(op.b1) {
				case SignedTypeInt32:
					b = int32(v1) <= int32(v2)
				case SignedTypeInt64:
					b = int64(v1) <= int64(v2)
				case SignedTypeUint32, SignedTypeUint64:
					b = v1 <= v2
				case SignedTypeFloat32:
					b = math.Float32frombits(uint32(v1)) <= math.Float32frombits(uint32(v2))
				case SignedTypeFloat64:
					b = math.Float64frombits(v1) <= math.Float64frombits(v2)
				}
				if b {
					it.push(1)
				} else {
					it.push(0)
				}
				frame.pc++
			}
		case OperationKindGe:
			{
				v2 := it.pop()
				v1 := it.pop()
				var b bool
				switch SignedType(op.b1) {
				case SignedTypeInt32:
					b = int32(v1) >= int32(v2)
				case SignedTypeInt64:
					b = int64(v1) >= int64(v2)
				case SignedTypeUint32, SignedTypeUint64:
					b = v1 >= v2
				case SignedTypeFloat32:
					b = math.Float32frombits(uint32(v1)) >= math.Float32frombits(uint32(v2))
				case SignedTypeFloat64:
					b = math.Float64frombits(v1) >= math.Float64frombits(v2)
				}
				if b {
					it.push(1)
				} else {
					it.push(0)
				}
				frame.pc++
			}
		case OperationKindAdd:
			{
				v2 := it.pop()
				v1 := it.pop()
				switch UnsignedType(op.b1) {
				case UnsignedTypeI32:
					v := uint32(v1) + uint32(v2)
					it.push(uint64(v))
				case UnsignedTypeI64:
					it.push(v1 + v2)
				case UnsignedTypeF32:
					v := math.Float32frombits(uint32(v1)) + math.Float32frombits(uint32(v2))
					it.push(uint64(math.Float32bits(v)))
				case UnsignedTypeF64:
					v := math.Float64frombits(v1) + math.Float64frombits(v2)
					it.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case OperationKindSub:
			{
				v2 := it.pop()
				v1 := it.pop()
				switch UnsignedType(op.b1) {
				case UnsignedTypeI32:
					it.push(uint64(uint32(v1) - uint32(v2)))
				case UnsignedTypeI64:
					it.push(v1 - v2)
				case UnsignedTypeF32:
					v := math.Float32frombits(uint32(v1)) - math.Float32frombits(uint32(v2))
					it.push(uint64(math.Float32bits(v)))
				case UnsignedTypeF64:
					v := math.Float64frombits(v1) - math.Float64frombits(v2)
					it.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case OperationKindMul:
			{
				v2 := it.pop()
				v1 := it.pop()
				switch UnsignedType(op.b1) {
				case UnsignedTypeI32:
					it.push(uint64(uint32(v1) * uint32(v2)))
				case UnsignedTypeI64:
					it.push(v1 * v2)
				case UnsignedTypeF32:
					v := math.Float32frombits(uint32(v2)) * math.Float32frombits(uint32(v1))
					it.push(uint64(math.Float32bits(v)))
				case UnsignedTypeF64:
					v := math.Float64frombits(v2) * math.Float64frombits(v1)
					it.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case OperationKindClz:
			{
				v := it.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					it.push(uint64(bits.LeadingZeros32(uint32(v))))
				} else {
					// UnsignedInt64
					it.push(uint64(bits.LeadingZeros64(v)))
				}
				frame.pc++
			}
		case OperationKindCtz:
			{
				v := it.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					it.push(uint64(bits.TrailingZeros32(uint32(v))))
				} else {
					// UnsignedInt64
					it.push(uint64(bits.TrailingZeros64(v)))
				}
				frame.pc++
			}
		case OperationKindPopcnt:
			{
				v := it.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					it.push(uint64(bits.OnesCount32(uint32(v))))
				} else {
					// UnsignedInt64
					it.push(uint64(bits.OnesCount64(v)))
				}
				frame.pc++
			}
		case OperationKindDiv:
			{
				switch SignedType(op.b1) {
				case SignedTypeInt32:
					v2 := int32(it.pop())
					v1 := int32(it.pop())
					if v1 == math.MinInt32 && v2 == -1 {
						panic(wasm.ErrRuntimeIntegerOverflow)
					}
					it.push(uint64(uint32(v1 / v2)))
				case SignedTypeInt64:
					v2 := int64(it.pop())
					v1 := int64(it.pop())
					if v1 == math.MinInt64 && v2 == -1 {
						panic(wasm.ErrRuntimeIntegerOverflow)
					}
					it.push(uint64(v1 / v2))
				case SignedTypeUint32:
					v2 := uint32(it.pop())
					v1 := uint32(it.pop())
					it.push(uint64(v1 / v2))
				case SignedTypeUint64:
					v2 := it.pop()
					v1 := it.pop()
					it.push(v1 / v2)
				case SignedTypeFloat32:
					v2 := it.pop()
					v1 := it.pop()
					v := math.Float32frombits(uint32(v1)) / math.Float32frombits(uint32(v2))
					it.push(uint64(math.Float32bits(v)))
				case SignedTypeFloat64:
					v2 := it.pop()
					v1 := it.pop()
					v := math.Float64frombits(v1) / math.Float64frombits(v2)
					it.push(uint64(math.Float64bits(v)))
				}
				frame.pc++
			}
		case OperationKindRem:
			{
				switch SignedInt(op.b1) {
				case SignedInt32:
					v2 := int32(it.pop())
					v1 := int32(it.pop())
					it.push(uint64(uint32(v1 % v2)))
				case SignedInt64:
					v2 := int64(it.pop())
					v1 := int64(it.pop())
					it.push(uint64(v1 % v2))
				case SignedUint32:
					v2 := uint32(it.pop())
					v1 := uint32(it.pop())
					it.push(uint64(v1 % v2))
				case SignedUint64:
					v2 := it.pop()
					v1 := it.pop()
					it.push(v1 % v2)
				}
				frame.pc++
			}
		case OperationKindAnd:
			{
				v2 := it.pop()
				v1 := it.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					it.push(uint64(uint32(v2) & uint32(v1)))
				} else {
					// UnsignedInt64
					it.push(uint64(v2 & v1))
				}
				frame.pc++
			}
		case OperationKindOr:
			{
				v2 := it.pop()
				v1 := it.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					it.push(uint64(uint32(v2) | uint32(v1)))
				} else {
					// UnsignedInt64
					it.push(uint64(v2 | v1))
				}
				frame.pc++
			}
		case OperationKindXor:
			{
				v2 := it.pop()
				v1 := it.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					it.push(uint64(uint32(v2) ^ uint32(v1)))
				} else {
					// UnsignedInt64
					it.push(uint64(v2 ^ v1))
				}
				frame.pc++
			}
		case OperationKindShl:
			{
				v2 := it.pop()
				v1 := it.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					it.push(uint64(uint32(v1) << (uint32(v2) % 32)))
				} else {
					// UnsignedInt64
					it.push(v1 << (v2 % 64))
				}
				frame.pc++
			}
		case OperationKindShr:
			{
				v2 := it.pop()
				v1 := it.pop()
				switch SignedInt(op.b1) {
				case SignedInt32:
					it.push(uint64(int32(v1) >> (uint32(v2) % 32)))
				case SignedInt64:
					it.push(uint64(int64(v1) >> (v2 % 64)))
				case SignedUint32:
					it.push(uint64(uint32(v1) >> (uint32(v2) % 32)))
				case SignedUint64:
					it.push(v1 >> (v2 % 64))
				}
				frame.pc++
			}
		case OperationKindRotl:
			{
				v2 := it.pop()
				v1 := it.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					it.push(uint64(bits.RotateLeft32(uint32(v1), int(v2))))
				} else {
					// UnsignedInt64
					it.push(uint64(bits.RotateLeft64(v1, int(v2))))
				}
				frame.pc++
			}
		case OperationKindRotr:
			{
				v2 := it.pop()
				v1 := it.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					it.push(uint64(bits.RotateLeft32(uint32(v1), -int(v2))))
				} else {
					// UnsignedInt64
					it.push(uint64(bits.RotateLeft64(v1, -int(v2))))
				}
				frame.pc++
			}
		case OperationKindAbs:
			{
				if op.b1 == 0 {
					// Float32
					const mask uint32 = 1 << 31
					it.push(uint64(uint32(it.pop()) &^ mask))
				} else {
					// Float64
					const mask uint64 = 1 << 63
					it.push(uint64(it.pop() &^ mask))
				}
				frame.pc++
			}
		case OperationKindNeg:
			{
				if op.b1 == 0 {
					// Float32
					v := -math.Float32frombits(uint32(it.pop()))
					it.push(uint64(math.Float32bits(v)))
				} else {
					// Float64
					v := -math.Float64frombits(it.pop())
					it.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case OperationKindCeil:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Ceil(float64(math.Float32frombits(uint32(it.pop()))))
					it.push(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Ceil(float64(math.Float64frombits(it.pop())))
					it.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case OperationKindFloor:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Floor(float64(math.Float32frombits(uint32(it.pop()))))
					it.push(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Floor(float64(math.Float64frombits(it.pop())))
					it.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case OperationKindTrunc:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Trunc(float64(math.Float32frombits(uint32(it.pop()))))
					it.push(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Trunc(float64(math.Float64frombits(it.pop())))
					it.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case OperationKindNearest:
			{
				// TODO: look at https://github.com/bytecodealliance/wasmtime/pull/2171 and reconsider this algorithm
				if op.b1 == 0 {
					// Float32
					f := math.Float32frombits(uint32(it.pop()))
					if f != -0 && f != 0 {
						ceil := float32(math.Ceil(float64(f)))
						floor := float32(math.Floor(float64(f)))
						distToCeil := math.Abs(float64(f - ceil))
						distToFloor := math.Abs(float64(f - floor))
						h := ceil / 2.0
						if distToCeil < distToFloor {
							f = ceil
						} else if distToCeil == distToFloor && float32(math.Floor(float64(h))) == h {
							f = ceil
						} else {
							f = floor
						}
					}
					it.push(uint64(math.Float32bits(f)))
				} else {
					// Float64
					f := math.Float64frombits(it.pop())
					if f != -0 && f != 0 {
						ceil := math.Ceil(f)
						floor := math.Floor(f)
						distToCeil := math.Abs(f - ceil)
						distToFloor := math.Abs(f - floor)
						h := ceil / 2.0
						if distToCeil < distToFloor {
							f = ceil
						} else if distToCeil == distToFloor && math.Floor(float64(h)) == h {
							f = ceil
						} else {
							f = floor
						}
					}
					it.push(math.Float64bits(f))
				}
				frame.pc++
			}
		case OperationKindSqrt:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Sqrt(float64(math.Float32frombits(uint32(it.pop()))))
					it.push(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Sqrt(float64(math.Float64frombits(it.pop())))
					it.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case OperationKindMin:
			{
				if op.b1 == 0 {
					// Float32
					v2 := math.Float32frombits(uint32(it.pop()))
					v1 := math.Float32frombits(uint32(it.pop()))
					it.push(uint64(math.Float32bits(float32(Min(float64(v1), float64(v2))))))
				} else {
					v2 := math.Float64frombits(it.pop())
					v1 := math.Float64frombits(it.pop())
					it.push(math.Float64bits(Min(v1, v2)))
				}
				frame.pc++
			}
		case OperationKindMax:
			{

				if op.b1 == 0 {
					// Float32
					v2 := math.Float32frombits(uint32(it.pop()))
					v1 := math.Float32frombits(uint32(it.pop()))
					it.push(uint64(math.Float32bits(float32(Max(float64(v1), float64(v2))))))
				} else {
					// Float64
					v2 := math.Float64frombits(it.pop())
					v1 := math.Float64frombits(it.pop())
					it.push(math.Float64bits(Max(v1, v2)))
				}
				frame.pc++
			}
		case OperationKindCopysign:
			{
				if op.b1 == 0 {
					// Float32
					v2 := math.Float32frombits(uint32(it.pop()))
					v1 := math.Float32frombits(uint32(it.pop()))
					it.push(uint64(math.Float32bits(float32(math.Copysign(float64(v1), float64(v2))))))
				} else {
					// Float64
					v2 := math.Float64frombits(it.pop())
					v1 := math.Float64frombits(it.pop())
					it.push(uint64(math.Float64bits(math.Copysign(v1, v2))))
				}
				frame.pc++
			}
		case OperationKindI32WrapFromI64:
			{
				it.push(uint64(uint32(it.pop())))
				frame.pc++
			}
		case OperationKindITruncFromF:
			{
				if op.b1 == 0 {
					// Float32
					switch SignedInt(op.b2) {
					case SignedInt32:
						v := math.Trunc(float64(math.Float32frombits(uint32(it.pop()))))
						if math.IsNaN(v) {
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < math.MinInt32 || v > math.MaxInt32 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						it.push(uint64(int32(v)))
					case SignedInt64:
						v := math.Trunc(float64(math.Float32frombits(uint32(it.pop()))))
						res := int64(v)
						if math.IsNaN(v) {
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < math.MinInt64 || v > 0 && res < 0 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						it.push(uint64(res))
					case SignedUint32:
						v := math.Trunc(float64(math.Float32frombits(uint32(it.pop()))))
						if math.IsNaN(v) {
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < 0 || v > math.MaxUint32 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						it.push(uint64(uint32(v)))
					case SignedUint64:
						v := math.Trunc(float64(math.Float32frombits(uint32(it.pop()))))
						res := uint64(v)
						if math.IsNaN(v) {
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < 0 || v > float64(res) {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						it.push(res)
					}
				} else {
					// Float64
					switch SignedInt(op.b2) {
					case SignedInt32:
						v := math.Trunc(math.Float64frombits(it.pop()))
						if math.IsNaN(v) {
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < math.MinInt32 || v > math.MaxInt32 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						it.push(uint64(int32(v)))
					case SignedInt64:
						v := math.Trunc(math.Float64frombits(it.pop()))
						res := int64(v)
						if math.IsNaN(v) {
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < math.MinInt64 || v > 0 && res < 0 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						it.push(uint64(res))
					case SignedUint32:
						v := math.Trunc(math.Float64frombits(it.pop()))
						if math.IsNaN(v) {
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < 0 || v > math.MaxUint32 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						it.push(uint64(uint32(v)))
					case SignedUint64:
						v := math.Trunc(math.Float64frombits(it.pop()))
						res := uint64(v)
						if math.IsNaN(v) {
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < 0 || v > float64(res) {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						it.push(res)
					}
				}
				frame.pc++
			}
		case OperationKindFConvertFromI:
			{
				switch SignedInt(op.b1) {
				case SignedInt32:
					if op.b2 == 0 {
						// Float32
						v := float32(int32(it.pop()))
						it.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(int32(it.pop()))
						it.push(math.Float64bits(v))
					}
				case SignedInt64:
					if op.b2 == 0 {
						// Float32
						v := float32(int64(it.pop()))
						it.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(int64(it.pop()))
						it.push(math.Float64bits(v))
					}
				case SignedUint32:
					if op.b2 == 0 {
						// Float32
						v := float32(uint32(it.pop()))
						it.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(uint32(it.pop()))
						it.push(math.Float64bits(v))
					}
				case SignedUint64:
					if op.b2 == 0 {
						// Float32
						v := float32(it.pop())
						it.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(it.pop())
						it.push(math.Float64bits(v))
					}
				}
				frame.pc++
			}
		case OperationKindF32DemoteFromF64:
			{
				v := float32(math.Float64frombits(it.pop()))
				it.push(uint64(math.Float32bits(v)))
				frame.pc++
			}
		case OperationKindF64PromoteFromF32:
			{
				v := float64(math.Float32frombits(uint32(it.pop())))
				it.push(math.Float64bits(v))
				frame.pc++
			}
		case OperationKindExtend:
			{
				if op.b1 == 1 {
					// Signed.
					v := int64(int32(it.pop()))
					it.push(uint64(v))
				} else {
					v := uint64(uint32(it.pop()))
					it.push(v)
				}
				frame.pc++
			}
		}
	}
	it.popFrame()
}

// math.Min doen't comply with the Wasm spec, so we borrow from the original
// with a change that either one of NaN results in NaN even if another is -Inf.
// https://github.com/golang/go/blob/1d20a362d0ca4898d77865e314ef6f73582daef0/src/math/dim.go#L74-L91
func Min(x, y float64) float64 {
	switch {
	case math.IsNaN(x) || math.IsNaN(y):
		return math.NaN()
	case math.IsInf(x, -1) || math.IsInf(y, -1):
		return math.Inf(-1)
	case x == 0 && x == y:
		if math.Signbit(x) {
			return x
		}
		return y
	}
	if x < y {
		return x
	}
	return y
}

// math.Max doen't comply with the Wasm spec, so we borrow from the original
// with a change that either one of NaN results in NaN even if another is Inf.
// https://github.com/golang/go/blob/1d20a362d0ca4898d77865e314ef6f73582daef0/src/math/dim.go#L42-L59
func Max(x, y float64) float64 {
	switch {
	case math.IsNaN(x) || math.IsNaN(y):
		return math.NaN()
	case math.IsInf(x, 1) || math.IsInf(y, 1):
		return math.Inf(1)

	case x == 0 && x == y:
		if math.Signbit(x) {
			return y
		}
		return x
	}
	if x > y {
		return x
	}
	return y
}
