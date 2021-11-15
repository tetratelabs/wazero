package wazeroir

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"reflect"
	"runtime/debug"
	"strings"
	"unsafe"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/buildoptions"
)

// interpreter implements wasm.Engine interface.
// This is the direct interpreter of wazeroir operations.
type interpreter struct {
	// Stores compiled functions.
	functions map[*wasm.FunctionInstance]*interpreterFunction
	// Cached function type IDs assigned to
	// function signatures.
	// Type IDs are used to check the signature of
	// functions on call_indirect operation at runtime
	functionTypeIDs map[string]uint64
	// stack contains the operands.
	// Note that all the values are represented as uint64.
	stack []uint64
	// Function call stack.
	frames []*interpreterFrame
	// The callbacks when an function instance is compiled.
	// See the comment where this is used below for detail.
	// Not used at runtime, and only in the compilation phase.
	onCompilationDoneCallbacks map[*wasm.FunctionInstance][]func(*interpreterFunction)
}

func (it *interpreter) push(v uint64) {
	it.stack = append(it.stack, v)
}

func (it *interpreter) pop() (v uint64) {
	// No need to check stack bound
	// as we can assume that all the operations
	// are valid thanks to analyzeFunction
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
	// are valid thanks to analyzeFunction
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
	if buildoptions.CheckCallStackOverflow {
		if buildoptions.CallStackHeightLimit <= len(it.frames)-1 {
			panic(wasm.ErrCallStackOverflow)
		}
	}
	it.frames = append(it.frames, frame)
}

func (it *interpreter) popFrame() (frame *interpreterFrame) {
	// No need to check stack bound
	// as we can assume that all the operations
	// are valid thanks to analyzeFunction
	// at module validation phase
	// and wazeroir translation
	// before compilation.
	frame = it.frames[len(it.frames)-1]
	it.frames = it.frames[:len(it.frames)-1]
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
	moduleInstance *wasm.ModuleInstance
	body           []*interpreterOp
	hostFn         *reflect.Value
	signature      *wasm.FunctionType
	typeID         uint64
}

// Non-interface union of all the wazeroir operations.
type interpreterOp struct {
	kind   OperationKind
	b1, b2 byte
	us     []uint64
	rs     []*InclusiveRange
	f      *interpreterFunction
}

// Implements wasm.Engine for interpreter.
func (it *interpreter) Compile(f *wasm.FunctionInstance) error {
	if _, ok := it.functions[f]; ok {
		return nil
	} else if f.HostFunction != nil {
		ret := &interpreterFunction{
			hostFn: f.HostFunction, moduleInstance: f.ModuleInstance,
			signature: f.Signature,
		}
		if id, ok := it.functionTypeIDs[funcTypeString(f.Signature)]; !ok {
			id = uint64(len(it.functionTypeIDs))
			it.functionTypeIDs[funcTypeString(f.Signature)] = id
			ret.typeID = id
		} else {
			ret.typeID = id
		}
		it.functions[f] = ret
		return nil
	}

	irOps, err := Compile(f)
	if err != nil {
		return fmt.Errorf("failed to lower Wasm to wazeroir: %w", err)
	}

	fn, err := it.lowerIROps(f, irOps)
	if err != nil {
		return fmt.Errorf("failed to lower wazeroir operations to interpreter operations: %w", err)
	}

	it.functions[f] = fn
	for _, cb := range it.onComilationDoneCallbacks[f] {
		cb(fn)
	}
	delete(it.onComilationDoneCallbacks, f)
	return nil
}

// Lowers the wazeroir operations to interpreter friendly struct.
func (it *interpreter) lowerIROps(f *wasm.FunctionInstance,
	ops []Operation) (*interpreterFunction, error) {
	ret := &interpreterFunction{moduleInstance: f.ModuleInstance, signature: f.Signature}
	if id, ok := it.functionTypeIDs[funcTypeString(f.Signature)]; !ok {
		id = uint64(len(it.functionTypeIDs))
		it.functionTypeIDs[funcTypeString(f.Signature)] = id
		ret.typeID = id
	} else {
		ret.typeID = id
	}
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
			targetInst := f.ModuleInstance.Functions[o.FunctionIndex]
			target, ok := it.functions[targetInst]
			if !ok {
				// If the target function instance is not compiled,
				// we set the callback so we can set the pointer to the target when the compilation done.
				it.onComilationDoneCallbacks[targetInst] = append(it.onComilationDoneCallbacks[targetInst],
					func(compiled *interpreterFunction) {
						op.f = compiled
					})
			} else {
				op.f = target
			}
		case *OperationCallIndirect:
			op.us = make([]uint64, 2)
			op.us[0] = uint64(o.TableIndex)
			key := funcTypeString(f.ModuleInstance.Types[o.TypeIndex])
			typeid, ok := it.functionTypeIDs[key]
			if !ok {
				it.functionTypeIDs[key] = uint64(len(it.functionTypeIDs))
			}
			op.us[1] = typeid
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
			// Reinterpret ops are essentially nop for interpreter mode.
			// so just eliminate the ops.
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

// Implements wasm.Engine for interpreter.
func (it *interpreter) Call(f *wasm.FunctionInstance, args ...uint64) (returns []uint64, err error) {
	prevFrameLen := len(it.frames)
	defer func() {
		if v := recover(); v != nil {
			// TODO: include stack trace in the error message.
			if buildoptions.IsDebugMode {
				debug.PrintStack()
			}
			it.frames = it.frames[:prevFrameLen]
			err2, ok := v.(error)
			if ok {
				err = fmt.Errorf("runtime error: %w", err2)
			} else {
				err = fmt.Errorf("runtime error: %v", v)
			}
		}
	}()

	g, ok := it.functions[f]
	if !ok {
		err = fmt.Errorf("function not compiled")
		return
	}

	if g.hostFn != nil {
		it.callHostFunc(g, args...)
	} else {
		for _, arg := range args {
			it.push(arg)
		}
		it.callNativeFunc(g)
	}
	returns = make([]uint64, len(f.Signature.ReturnTypes))
	for i := range returns {
		returns[len(returns)-1-i] = it.pop()
	}
	return
}

func (it *interpreter) callHostFunc(f *interpreterFunction, args ...uint64) {
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
		memory = it.frames[len(it.frames)-1].f.moduleInstance.Memory
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
	memoryInst := frame.f.moduleInstance.Memory
	globals := frame.f.moduleInstance.Globals
	tables := f.moduleInstance.Tables
	it.pushFrame(frame)
	bodyLen := uint64(len(frame.f.body))
	for frame.pc < bodyLen {
		op := frame.f.body[frame.pc]
		switch op.kind {
		case OperationKindUnreachable:
			panic("unreachable")
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
					it.callHostFunc(op.f, it.stack[len(it.stack)-len(op.f.signature.InputTypes):]...)
				} else {
					it.callNativeFunc(op.f)
				}
				frame.pc++
			}
		case OperationKindCallIndirect:
			{
				index := it.pop()
				target := it.functions[tables[op.us[0]].Table[index].Function]
				// Type check.
				if target.typeID != op.us[1] {
					panic("function signature mismatch on call_indirect")
				}
				// Call in.
				if target.hostFn != nil {
					it.callHostFunc(target, it.stack[len(it.stack)-len(target.signature.InputTypes):]...)
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
				switch SignLessType(op.b1) {
				case SignLessTypeI32, SignLessTypeF32:
					it.push(uint64(binary.LittleEndian.Uint32(memoryInst.Buffer[base:])))
				case SignLessTypeI64, SignLessTypeF64:
					it.push(binary.LittleEndian.Uint64(memoryInst.Buffer[base:]))
				}
				frame.pc++
			}
		case OperationKindLoad8:
			{
				base := op.us[1] + it.pop()
				switch SignFulInt(op.b1) {
				case SignFulInt32, SignFulInt64:
					it.push(uint64(int8(memoryInst.Buffer[base])))
				case SignFulUint32, SignFulUint64:
					it.push(uint64(uint8(memoryInst.Buffer[base])))
				}
				frame.pc++
			}
		case OperationKindLoad16:
			{
				base := op.us[1] + it.pop()
				switch SignFulInt(op.b1) {
				case SignFulInt32, SignFulInt64:
					it.push(uint64(int16(binary.LittleEndian.Uint16(memoryInst.Buffer[base:]))))
				case SignFulUint32, SignFulUint64:
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
				switch SignLessType(op.b1) {
				case SignLessTypeI32, SignLessTypeF32:
					binary.LittleEndian.PutUint32(memoryInst.Buffer[base:], uint32(val))
				case SignLessTypeI64, SignLessTypeF64:
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
				v := uint64(len(frame.f.moduleInstance.Memory.Buffer)) / wasm.PageSize
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
				switch SignLessType(op.b1) {
				case SignLessTypeI32, SignLessTypeI64:
					v2, v1 := it.pop(), it.pop()
					b = v1 == v2
				case SignLessTypeF32:
					v2, v1 := it.pop(), it.pop()
					b = math.Float32frombits(uint32(v2)) == math.Float32frombits(uint32(v1))
				case SignLessTypeF64:
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
				switch SignLessType(op.b1) {
				case SignLessTypeI32, SignLessTypeI64:
					v2, v1 := it.pop(), it.pop()
					b = v1 != v2
				case SignLessTypeF32:
					v2, v1 := it.pop(), it.pop()
					b = math.Float32frombits(uint32(v2)) != math.Float32frombits(uint32(v1))
				case SignLessTypeF64:
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
				switch SignFulType(op.b1) {
				case SignFulTypeInt32:
					b = int32(v1) < int32(v2)
				case SignFulTypeInt64:
					b = int64(v1) < int64(v2)
				case SignFulTypeUint32, SignFulTypeUint64:
					b = v1 < v2
				case SignFulTypeFloat32:
					b = math.Float32frombits(uint32(v1)) < math.Float32frombits(uint32(v2))
				case SignFulTypeFloat64:
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
				switch SignFulType(op.b1) {
				case SignFulTypeInt32:
					b = int32(v1) > int32(v2)
				case SignFulTypeInt64:
					b = int64(v1) > int64(v2)
				case SignFulTypeUint32, SignFulTypeUint64:
					b = v1 > v2
				case SignFulTypeFloat32:
					b = math.Float32frombits(uint32(v1)) > math.Float32frombits(uint32(v2))
				case SignFulTypeFloat64:
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
				switch SignFulType(op.b1) {
				case SignFulTypeInt32:
					b = int32(v1) <= int32(v2)
				case SignFulTypeInt64:
					b = int64(v1) <= int64(v2)
				case SignFulTypeUint32, SignFulTypeUint64:
					b = v1 <= v2
				case SignFulTypeFloat32:
					b = math.Float32frombits(uint32(v1)) <= math.Float32frombits(uint32(v2))
				case SignFulTypeFloat64:
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
				switch SignFulType(op.b1) {
				case SignFulTypeInt32:
					b = int32(v1) >= int32(v2)
				case SignFulTypeInt64:
					b = int64(v1) >= int64(v2)
				case SignFulTypeUint32, SignFulTypeUint64:
					b = v1 >= v2
				case SignFulTypeFloat32:
					b = math.Float32frombits(uint32(v1)) >= math.Float32frombits(uint32(v2))
				case SignFulTypeFloat64:
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
				switch SignLessType(op.b1) {
				case SignLessTypeI32:
					v := uint32(v1) + uint32(v2)
					it.push(uint64(v))
				case SignLessTypeI64:
					it.push(v1 + v2)
				case SignLessTypeF32:
					v := math.Float32frombits(uint32(v1)) + math.Float32frombits(uint32(v2))
					it.push(uint64(math.Float32bits(v)))
				case SignLessTypeF64:
					v := math.Float64frombits(v1) + math.Float64frombits(v2)
					it.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case OperationKindSub:
			{
				v2 := it.pop()
				v1 := it.pop()
				switch SignLessType(op.b1) {
				case SignLessTypeI32:
					it.push(uint64(uint32(v1) - uint32(v2)))
				case SignLessTypeI64:
					it.push(v1 - v2)
				case SignLessTypeF32:
					v := math.Float32frombits(uint32(v1)) - math.Float32frombits(uint32(v2))
					it.push(uint64(math.Float32bits(v)))
				case SignLessTypeF64:
					v := math.Float64frombits(v1) - math.Float64frombits(v2)
					it.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case OperationKindMul:
			{
				v2 := it.pop()
				v1 := it.pop()
				switch SignLessType(op.b1) {
				case SignLessTypeI32:
					it.push(uint64(uint32(v1) * uint32(v2)))
				case SignLessTypeI64:
					it.push(v1 * v2)
				case SignLessTypeF32:
					v := math.Float32frombits(uint32(v2)) * math.Float32frombits(uint32(v1))
					it.push(uint64(math.Float32bits(v)))
				case SignLessTypeF64:
					v := math.Float64frombits(v2) * math.Float64frombits(v1)
					it.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case OperationKindClz:
			{
				v := it.pop()
				if op.b1 == 0 {
					// SignLessInt32
					it.push(uint64(bits.LeadingZeros32(uint32(v))))
				} else {
					// SignLessInt64
					it.push(uint64(bits.LeadingZeros64(v)))
				}
				frame.pc++
			}
		case OperationKindCtz:
			{
				v := it.pop()
				if op.b1 == 0 {
					// SignLessInt32
					it.push(uint64(bits.TrailingZeros32(uint32(v))))
				} else {
					// SignLessInt64
					it.push(uint64(bits.TrailingZeros64(v)))
				}
				frame.pc++
			}
		case OperationKindPopcnt:
			{
				v := it.pop()
				if op.b1 == 0 {
					// SignLessInt32
					it.push(uint64(bits.OnesCount32(uint32(v))))
				} else {
					// SignLessInt64
					it.push(uint64(bits.OnesCount64(v)))
				}
				frame.pc++
			}
		case OperationKindDiv:
			{
				switch SignFulType(op.b1) {
				case SignFulTypeInt32:
					v2 := int32(it.pop())
					v1 := int32(it.pop())
					if v2 == 0 || (v1 == math.MinInt32 && v2 == -1) {
						panic("undefined")
					}
					it.push(uint64(uint32(v1 / v2)))
				case SignFulTypeInt64:
					v2 := int64(it.pop())
					v1 := int64(it.pop())
					if v2 == 0 || (v1 == math.MinInt64 && v2 == -1) {
						panic("undefined")
					}
					it.push(uint64(v1 / v2))
				case SignFulTypeUint32:
					v2 := uint32(it.pop())
					v1 := uint32(it.pop())
					it.push(uint64(v1 / v2))
				case SignFulTypeUint64:
					v2 := it.pop()
					v1 := it.pop()
					it.push(v1 / v2)
				case SignFulTypeFloat32:
					v2 := it.pop()
					v1 := it.pop()
					v := math.Float32frombits(uint32(v1)) / math.Float32frombits(uint32(v2))
					it.push(uint64(math.Float32bits(v)))
				case SignFulTypeFloat64:
					v2 := it.pop()
					v1 := it.pop()
					v := math.Float64frombits(v1) / math.Float64frombits(v2)
					it.push(uint64(math.Float64bits(v)))
				}
				frame.pc++
			}
		case OperationKindRem:
			{
				switch SignFulInt(op.b1) {
				case SignFulInt32:
					v2 := int32(it.pop())
					v1 := int32(it.pop())
					it.push(uint64(uint32(v1 % v2)))
				case SignFulInt64:
					v2 := int64(it.pop())
					v1 := int64(it.pop())
					it.push(uint64(v1 % v2))
				case SignFulUint32:
					v2 := uint32(it.pop())
					v1 := uint32(it.pop())
					it.push(uint64(v1 % v2))
				case SignFulUint64:
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
					// SignLessInt32
					it.push(uint64(uint32(v2) & uint32(v1)))
				} else {
					// SignLessInt64
					it.push(uint64(v2 & v1))
				}
				frame.pc++
			}
		case OperationKindOr:
			{
				v2 := it.pop()
				v1 := it.pop()
				if op.b1 == 0 {
					// SignLessInt32
					it.push(uint64(uint32(v2) | uint32(v1)))
				} else {
					// SignLessInt64
					it.push(uint64(v2 | v1))
				}
				frame.pc++
			}
		case OperationKindXor:
			{
				v2 := it.pop()
				v1 := it.pop()
				if op.b1 == 0 {
					// SignLessInt32
					it.push(uint64(uint32(v2) ^ uint32(v1)))
				} else {
					// SignLessInt64
					it.push(uint64(v2 ^ v1))
				}
				frame.pc++
			}
		case OperationKindShl:
			{
				v2 := it.pop()
				v1 := it.pop()
				if op.b1 == 0 {
					// SignLessInt32
					it.push(uint64(uint32(v1) << (uint32(v2) % 32)))
				} else {
					// SignLessInt64
					it.push(v1 << (v2 % 64))
				}
				frame.pc++
			}
		case OperationKindShr:
			{
				v2 := it.pop()
				v1 := it.pop()
				switch SignFulInt(op.b1) {
				case SignFulInt32:
					it.push(uint64(int32(v1) >> (uint32(v2) % 32)))
				case SignFulInt64:
					it.push(uint64(int64(v1) >> (v2 % 64)))
				case SignFulUint32:
					it.push(uint64(uint32(v1) >> (uint32(v2) % 32)))
				case SignFulUint64:
					it.push(v1 >> (v2 % 64))
				}
				frame.pc++
			}
		case OperationKindRotl:
			{
				v2 := it.pop()
				v1 := it.pop()
				if op.b1 == 0 {
					// SignLessInt32
					it.push(uint64(bits.RotateLeft32(uint32(v1), int(v2))))
				} else {
					// SignLessInt64
					it.push(uint64(bits.RotateLeft64(v1, int(v2))))
				}
				frame.pc++
			}
		case OperationKindRotr:
			{
				v2 := it.pop()
				v1 := it.pop()
				if op.b1 == 0 {
					// SignLessInt32
					it.push(uint64(bits.RotateLeft32(uint32(v1), -int(v2))))
				} else {
					// SignLessInt64
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
				// Borrowed from https://github.com/wasmerio/wasmer/blob/703bb4ee2ffb17b2929a194fc045a7e351b696e2/lib/vm/src/libcalls.rs#L77
				if op.b1 == 0 {
					// Float32
					f := math.Float32frombits(uint32(it.pop()))
					f64 := float64(f)
					if f != -0 && f != 0 {
						u := float32(math.Ceil(f64))
						d := float32(math.Floor(f64))
						um := math.Abs(float64(f - u))
						dm := math.Abs(float64(f - d))
						h := u / 2.0
						if um < dm || float32(math.Floor(float64(h))) == h {
							f = u
						} else {
							f = d
						}
					}
					it.push(uint64(math.Float32bits(f)))
				} else {
					// Float64
					f := math.Float64frombits(it.pop())
					f64 := float64(f)
					if f != -0 && f != 0 {
						u := math.Ceil(f64)
						d := math.Floor(f64)
						um := math.Abs(f - u)
						dm := math.Abs(f - d)
						h := u / 2.0
						if um < dm || math.Floor(float64(h)) == h {
							f = u
						} else {
							f = d
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
					it.push(uint64(math.Float32bits(float32(min(float64(v1), float64(v2))))))
				} else {
					v2 := math.Float64frombits(it.pop())
					v1 := math.Float64frombits(it.pop())
					it.push(math.Float64bits(min(v1, v2)))
				}
				frame.pc++
			}
		case OperationKindMax:
			{

				if op.b1 == 0 {
					// Float32
					v2 := math.Float32frombits(uint32(it.pop()))
					v1 := math.Float32frombits(uint32(it.pop()))
					it.push(uint64(math.Float32bits(float32(max(float64(v1), float64(v2))))))
				} else {
					// Float64
					v2 := math.Float64frombits(it.pop())
					v1 := math.Float64frombits(it.pop())
					it.push(math.Float64bits(max(v1, v2)))
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
					switch SignFulInt(op.b2) {
					case SignFulInt32:
						v := math.Trunc(float64(math.Float32frombits(uint32(it.pop()))))
						if math.IsNaN(v) {
							panic("invalid conversion")
						} else if v < math.MinInt32 || v > math.MaxInt32 {
							panic("integer overflow")
						}
						it.push(uint64(int32(v)))
					case SignFulInt64:
						v := math.Trunc(float64(math.Float32frombits(uint32(it.pop()))))
						res := int64(v)
						if math.IsNaN(v) {
							panic("invalid conversion")
						} else if v < math.MinInt64 || v > 0 && res < 0 {
							panic("integer overflow")
						}
						it.push(uint64(res))
					case SignFulUint32:
						v := math.Trunc(float64(math.Float32frombits(uint32(it.pop()))))
						if math.IsNaN(v) {
							panic("invalid conversion")
						} else if v < 0 || v > math.MaxUint32 {
							panic("integer overflow")
						}
						it.push(uint64(uint32(v)))
					case SignFulUint64:
						v := math.Trunc(float64(math.Float32frombits(uint32(it.pop()))))
						res := uint64(v)
						if math.IsNaN(v) {
							panic("invalid conversion")
						} else if v < 0 || v > float64(res) {
							panic("integer overflow")
						}
						it.push(res)
					}
				} else {
					// Float64
					switch SignFulInt(op.b2) {
					case SignFulInt32:
						v := math.Trunc(math.Float64frombits(it.pop()))
						if math.IsNaN(v) {
							panic("invalid conversion")
						} else if v < math.MinInt32 || v > math.MaxInt32 {
							panic("integer overflow")
						}
						it.push(uint64(int32(v)))
					case SignFulInt64:
						v := math.Trunc(math.Float64frombits(it.pop()))
						res := int64(v)
						if math.IsNaN(v) {
							panic("invalid conversion")
						} else if v < math.MinInt64 || v > 0 && res < 0 {
							panic("integer overflow")
						}
						it.push(uint64(res))
					case SignFulUint32:
						v := math.Trunc(math.Float64frombits(it.pop()))
						if math.IsNaN(v) {
							panic("invalid conversion")
						} else if v < 0 || v > math.MaxUint32 {
							panic("integer overflow")
						}
						it.push(uint64(uint32(v)))
					case SignFulUint64:
						v := math.Trunc(math.Float64frombits(it.pop()))
						res := uint64(v)
						if math.IsNaN(v) {
							panic("invalid conversion")
						} else if v < 0 || v > float64(res) {
							panic("integer overflow")
						}
						it.push(res)
					}
				}
				frame.pc++
			}
		case OperationKindFConvertFromI:
			{
				switch SignFulInt(op.b1) {
				case SignFulInt32:
					if op.b2 == 0 {
						// Float32
						v := float32(int32(it.pop()))
						it.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(int32(it.pop()))
						it.push(math.Float64bits(v))
					}
				case SignFulInt64:
					if op.b2 == 0 {
						// Float32
						v := float32(int64(it.pop()))
						it.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(int64(it.pop()))
						it.push(math.Float64bits(v))
					}
				case SignFulUint32:
					if op.b2 == 0 {
						// Float32
						v := float32(uint32(it.pop()))
						it.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(uint32(it.pop()))
						it.push(math.Float64bits(v))
					}
				case SignFulUint64:
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

func funcTypeString(t *wasm.FunctionType) string {
	return fmt.Sprintf("%s-%s",
		// Fast stringification of byte slice.
		// This is safe anyway as the results are copied
		// into the return value string.
		*(*string)(unsafe.Pointer(&t.InputTypes)),
		*(*string)(unsafe.Pointer(&t.ReturnTypes)),
	)
}

// math.Min doen't comply with the Wasm spec, so we borrow from the original
// with a change that either one of NaN results in NaN even if another is -Inf.
// https://github.com/golang/go/blob/1d20a362d0ca4898d77865e314ef6f73582daef0/src/math/dim.go#L74-L91
func min(x, y float64) float64 {
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
func max(x, y float64) float64 {
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
