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

	"github.com/tetratelabs/wazero/internal/moremath"
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/buildoptions"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

var callStackCeiling = buildoptions.CallStackCeiling

// engine is an interpreter implementation of internalwasm.Engine
type engine struct {
	// functions stores compiled functions where the index means wasm.FunctionAddress.
	functions []*interpreterFunction

	// mux is used for read/write access to functions slice.
	// This is necessary as each compiled function will access the slice at runtime
	// when they make function calls while engine might be modifying the underlying slice when
	// adding a new compiled function. We take read lock when creating new virtualMachine
	// for each function invocation while take write lock in engine.addCompiledFunction.
	mux sync.RWMutex
}

const initialCompiledFunctionsSliceSize = 128

func NewEngine() wasm.Engine {
	return &engine{
		functions: make([]*interpreterFunction, initialCompiledFunctionsSliceSize),
	}
}

type virtualMachine struct {
	// stack contains the operands.
	// Note that all the values are represented as uint64.
	stack []uint64
	// Function call stack.
	frames []*interpreterFrame

	// functions is engine.functions at the time when this virtual machine was created.
	// engine.functions can be modified whenever engine compiles a new function, so
	// we take a pointer to the current functions array so that it is safe even if engine.functions changes.
	functions []*interpreterFunction
}

func (e *engine) newVirtualMachine() *virtualMachine {
	e.mux.RLock()
	defer e.mux.RUnlock()
	return &virtualMachine{functions: e.functions}
}

func (vm *virtualMachine) push(v uint64) {
	vm.stack = append(vm.stack, v)
}

func (vm *virtualMachine) pop() (v uint64) {
	// No need to check stack bound
	// as we can assume that all the operations
	// are valid thanks to validateFunction
	// at module validation phase
	// and wazeroir translation
	// before compilation.
	v = vm.stack[len(vm.stack)-1]
	vm.stack = vm.stack[:len(vm.stack)-1]
	return
}

func (vm *virtualMachine) drop(r *wazeroir.InclusiveRange) {
	// No need to check stack bound
	// as we can assume that all the operations
	// are valid thanks to validateFunction
	// at module validation phase
	// and wazeroir translation
	// before compilation.
	if r == nil {
		return
	} else if r.Start == 0 {
		vm.stack = vm.stack[:len(vm.stack)-1-r.End]
	} else {
		newStack := vm.stack[:len(vm.stack)-1-r.End]
		newStack = append(newStack, vm.stack[len(vm.stack)-r.Start:]...)
		vm.stack = newStack
	}
}

func (vm *virtualMachine) pushFrame(frame *interpreterFrame) {
	if callStackCeiling <= len(vm.frames) {
		panic(wasm.ErrRuntimeCallStackOverflow)
	}
	vm.frames = append(vm.frames, frame)
}

func (vm *virtualMachine) popFrame() (frame *interpreterFrame) {
	// No need to check stack bound as we can assume that all the operations are valid thanks to validateFunction at
	// module validation phase and wazeroir translation before compilation.
	oneLess := len(vm.frames) - 1
	frame = vm.frames[oneLess]
	vm.frames = vm.frames[:oneLess]
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
}

// Compile Implements wasm.Engine for engine.
func (e *engine) Compile(f *wasm.FunctionInstance) error {
	funcaddr := f.Address
	var compiled *interpreterFunction
	if f.FunctionKind == wasm.FunctionKindWasm {
		ir, err := wazeroir.Compile(f)
		if err != nil {
			return fmt.Errorf("failed to compile Wasm to wazeroir: %w", err)
		}

		compiled, err = e.lowerIROps(f, ir.Operations)
		if err != nil {
			return fmt.Errorf("failed to convert wazeroir operations to engine ones: %w", err)
		}
	} else {
		compiled = &interpreterFunction{
			hostFn: f.HostFunction, funcInstance: f,
		}
	}

	if l := len(e.functions); l <= int(funcaddr) {
		e.mux.Lock() // Write lock.
		defer e.mux.Unlock()
		// Double the size of compiled functions.
		e.functions = append(e.functions, make([]*interpreterFunction, l)...)
	}
	e.functions[funcaddr] = compiled
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
			op.us = make([]uint64, 1)
			op.us[0] = uint64(target.Address)
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

	vm := e.newVirtualMachine()
	defer func() {
		if v := recover(); v != nil {
			if buildoptions.IsDebugMode {
				debug.PrintStack()
			}
			traces := make([]string, 0, len(vm.frames))
			for i := 0; i < len(vm.frames); i++ {
				frame := vm.popFrame()
				name := frame.f.funcInstance.Name
				// TODO: include DWARF symbols. See #58
				traces = append(traces, fmt.Sprintf("\t%d: %s", i, name))
			}

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

	compiled := e.functions[f.Address]
	if compiled == nil {
		err = fmt.Errorf("function not compiled")
		return
	}

	for _, param := range params {
		vm.push(param)
	}
	if f.FunctionKind == wasm.FunctionKindWasm {
		vm.callNativeFunc(ctx, compiled)
	} else {
		vm.callHostFunc(ctx, compiled)
	}
	results = make([]uint64, len(f.FunctionType.Type.Results))
	for i := range results {
		results[len(results)-1-i] = vm.pop()
	}
	return
}

func (vm *virtualMachine) callHostFunc(ctx *wasm.ModuleContext, f *interpreterFunction) {
	tp := f.hostFn.Type()
	in := make([]reflect.Value, tp.NumIn())

	wasmParamOffset := 0
	if f.funcInstance.FunctionKind != wasm.FunctionKindGoNoContext {
		wasmParamOffset = 1
	}
	for i := len(in) - 1; i >= wasmParamOffset; i-- {
		val := reflect.New(tp.In(i)).Elem()
		raw := vm.pop()
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
	if len(vm.frames) > 0 {
		ctx = ctx.WithMemory(vm.frames[len(vm.frames)-1].f.funcInstance.ModuleInstance.MemoryInstance)
	}

	// Handle any special parameter zero
	if val := wasm.GetHostFunctionCallContextValue(f.funcInstance.FunctionKind, ctx); val != nil {
		in[0] = *val
	}

	frame := &interpreterFrame{f: f}
	vm.pushFrame(frame)
	for _, ret := range f.hostFn.Call(in) {
		switch ret.Kind() {
		case reflect.Float32:
			vm.push(uint64(math.Float32bits(float32(ret.Float()))))
		case reflect.Float64:
			vm.push(math.Float64bits(ret.Float()))
		case reflect.Uint32, reflect.Uint64:
			vm.push(ret.Uint())
		case reflect.Int32, reflect.Int64:
			vm.push(uint64(ret.Int()))
		default:
			panic("invalid return type")
		}
	}
	vm.popFrame()
}

func (vm *virtualMachine) callNativeFunc(ctx *wasm.ModuleContext, f *interpreterFunction) {
	frame := &interpreterFrame{f: f}
	moduleInst := f.funcInstance.ModuleInstance
	memoryInst := moduleInst.MemoryInstance
	globals := moduleInst.Globals
	var table *wasm.TableInstance
	if len(moduleInst.Tables) > 0 {
		table = moduleInst.Tables[0] // WebAssembly 1.0 (MVP) defines at most one table
	}
	vm.pushFrame(frame)
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
				if vm.pop() > 0 {
					vm.drop(op.rs[0])
					frame.pc = op.us[0]
				} else {
					vm.drop(op.rs[1])
					frame.pc = op.us[1]
				}
			}
		case wazeroir.OperationKindBrTable:
			{
				if v := int(vm.pop()); v < len(op.us)-1 {
					vm.drop(op.rs[v+1])
					frame.pc = op.us[v+1]
				} else {
					// Default branch.
					vm.drop(op.rs[0])
					frame.pc = op.us[0]
				}
			}
		case wazeroir.OperationKindCall:
			{
				f := vm.functions[op.us[0]]
				if f.hostFn != nil {
					vm.callHostFunc(ctx, f)
				} else {
					vm.callNativeFunc(ctx, f)
				}
				frame.pc++
			}
		case wazeroir.OperationKindCallIndirect:
			{
				offset := vm.pop()
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
				target := vm.functions[table.Table[offset].FunctionAddress]
				// Call in.
				if target.hostFn != nil {
					vm.callHostFunc(ctx, target)
				} else {
					vm.callNativeFunc(ctx, target)
				}
				frame.pc++
			}
		case wazeroir.OperationKindDrop:
			{
				vm.drop(op.rs[0])
				frame.pc++
			}
		case wazeroir.OperationKindSelect:
			{
				c := vm.pop()
				v2 := vm.pop()
				if c == 0 {
					_ = vm.pop()
					vm.push(v2)
				}
				frame.pc++
			}
		case wazeroir.OperationKindPick:
			{
				vm.push(vm.stack[len(vm.stack)-1-int(op.us[0])])
				frame.pc++
			}
		case wazeroir.OperationKindSwap:
			{
				index := len(vm.stack) - 1 - int(op.us[0])
				vm.stack[len(vm.stack)-1], vm.stack[index] = vm.stack[index], vm.stack[len(vm.stack)-1]
				frame.pc++
			}
		case wazeroir.OperationKindGlobalGet:
			{
				g := globals[op.us[0]]
				vm.push(g.Val)
				frame.pc++
			}
		case wazeroir.OperationKindGlobalSet:
			{
				g := globals[op.us[0]]
				g.Val = vm.pop()
				frame.pc++
			}
		case wazeroir.OperationKindLoad:
			{
				base := op.us[1] + vm.pop()
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeF32:
					if uint64(len(memoryInst.Buffer)) < base+4 {
						panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
					}
					vm.push(uint64(binary.LittleEndian.Uint32(memoryInst.Buffer[base:])))
				case wazeroir.UnsignedTypeI64, wazeroir.UnsignedTypeF64:
					if uint64(len(memoryInst.Buffer)) < base+8 {
						panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
					}
					vm.push(binary.LittleEndian.Uint64(memoryInst.Buffer[base:]))
				}
				frame.pc++
			}
		case wazeroir.OperationKindLoad8:
			{
				base := op.us[1] + vm.pop()
				if uint64(len(memoryInst.Buffer)) < base+1 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				switch wazeroir.SignedInt(op.b1) {
				case wazeroir.SignedInt32, wazeroir.SignedInt64:
					vm.push(uint64(int8(memoryInst.Buffer[base])))
				case wazeroir.SignedUint32, wazeroir.SignedUint64:
					vm.push(uint64(uint8(memoryInst.Buffer[base])))
				}
				frame.pc++
			}
		case wazeroir.OperationKindLoad16:
			{
				base := op.us[1] + vm.pop()
				if uint64(len(memoryInst.Buffer)) < base+2 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				switch wazeroir.SignedInt(op.b1) {
				case wazeroir.SignedInt32, wazeroir.SignedInt64:
					vm.push(uint64(int16(binary.LittleEndian.Uint16(memoryInst.Buffer[base:]))))
				case wazeroir.SignedUint32, wazeroir.SignedUint64:
					vm.push(uint64(binary.LittleEndian.Uint16(memoryInst.Buffer[base:])))
				}
				frame.pc++
			}
		case wazeroir.OperationKindLoad32:
			{
				base := op.us[1] + vm.pop()
				if uint64(len(memoryInst.Buffer)) < base+4 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				if op.b1 == 1 {
					vm.push(uint64(int32(binary.LittleEndian.Uint32(memoryInst.Buffer[base:]))))
				} else {
					vm.push(uint64(binary.LittleEndian.Uint32(memoryInst.Buffer[base:])))
				}
				frame.pc++
			}
		case wazeroir.OperationKindStore:
			{
				val := vm.pop()
				base := op.us[1] + vm.pop()
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
				val := byte(vm.pop())
				base := op.us[1] + vm.pop()
				if uint64(len(memoryInst.Buffer)) < base+1 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				memoryInst.Buffer[base] = val
				frame.pc++
			}
		case wazeroir.OperationKindStore16:
			{
				val := uint16(vm.pop())
				base := op.us[1] + vm.pop()
				if uint64(len(memoryInst.Buffer)) < base+2 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				binary.LittleEndian.PutUint16(memoryInst.Buffer[base:], val)
				frame.pc++
			}
		case wazeroir.OperationKindStore32:
			{
				val := uint32(vm.pop())
				base := op.us[1] + vm.pop()
				if uint64(len(memoryInst.Buffer)) < base+4 {
					panic(wasm.ErrRuntimeOutOfBoundsMemoryAccess)
				}
				binary.LittleEndian.PutUint32(memoryInst.Buffer[base:], val)
				frame.pc++
			}
		case wazeroir.OperationKindMemorySize:
			{
				vm.push(uint64(memoryInst.PageSize()))
				frame.pc++
			}
		case wazeroir.OperationKindMemoryGrow:
			{
				n := vm.pop()
				res := memoryInst.Grow(uint32(n))
				vm.push(uint64(res))
				frame.pc++
			}
		case wazeroir.OperationKindConstI32, wazeroir.OperationKindConstI64,
			wazeroir.OperationKindConstF32, wazeroir.OperationKindConstF64:
			{
				vm.push(op.us[0])
				frame.pc++
			}
		case wazeroir.OperationKindEq:
			{
				var b bool
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeI64:
					v2, v1 := vm.pop(), vm.pop()
					b = v1 == v2
				case wazeroir.UnsignedTypeF32:
					v2, v1 := vm.pop(), vm.pop()
					b = math.Float32frombits(uint32(v2)) == math.Float32frombits(uint32(v1))
				case wazeroir.UnsignedTypeF64:
					v2, v1 := vm.pop(), vm.pop()
					b = math.Float64frombits(v2) == math.Float64frombits(v1)
				}
				if b {
					vm.push(1)
				} else {
					vm.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindNe:
			{
				var b bool
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeI64:
					v2, v1 := vm.pop(), vm.pop()
					b = v1 != v2
				case wazeroir.UnsignedTypeF32:
					v2, v1 := vm.pop(), vm.pop()
					b = math.Float32frombits(uint32(v2)) != math.Float32frombits(uint32(v1))
				case wazeroir.UnsignedTypeF64:
					v2, v1 := vm.pop(), vm.pop()
					b = math.Float64frombits(v2) != math.Float64frombits(v1)
				}
				if b {
					vm.push(1)
				} else {
					vm.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindEqz:
			{
				if vm.pop() == 0 {
					vm.push(1)
				} else {
					vm.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindLt:
			{
				v2 := vm.pop()
				v1 := vm.pop()
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
					vm.push(1)
				} else {
					vm.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindGt:
			{
				v2 := vm.pop()
				v1 := vm.pop()
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
					vm.push(1)
				} else {
					vm.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindLe:
			{
				v2 := vm.pop()
				v1 := vm.pop()
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
					vm.push(1)
				} else {
					vm.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindGe:
			{
				v2 := vm.pop()
				v1 := vm.pop()
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
					vm.push(1)
				} else {
					vm.push(0)
				}
				frame.pc++
			}
		case wazeroir.OperationKindAdd:
			{
				v2 := vm.pop()
				v1 := vm.pop()
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32:
					v := uint32(v1) + uint32(v2)
					vm.push(uint64(v))
				case wazeroir.UnsignedTypeI64:
					vm.push(v1 + v2)
				case wazeroir.UnsignedTypeF32:
					v := math.Float32frombits(uint32(v1)) + math.Float32frombits(uint32(v2))
					vm.push(uint64(math.Float32bits(v)))
				case wazeroir.UnsignedTypeF64:
					v := math.Float64frombits(v1) + math.Float64frombits(v2)
					vm.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindSub:
			{
				v2 := vm.pop()
				v1 := vm.pop()
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32:
					vm.push(uint64(uint32(v1) - uint32(v2)))
				case wazeroir.UnsignedTypeI64:
					vm.push(v1 - v2)
				case wazeroir.UnsignedTypeF32:
					v := math.Float32frombits(uint32(v1)) - math.Float32frombits(uint32(v2))
					vm.push(uint64(math.Float32bits(v)))
				case wazeroir.UnsignedTypeF64:
					v := math.Float64frombits(v1) - math.Float64frombits(v2)
					vm.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindMul:
			{
				v2 := vm.pop()
				v1 := vm.pop()
				switch wazeroir.UnsignedType(op.b1) {
				case wazeroir.UnsignedTypeI32:
					vm.push(uint64(uint32(v1) * uint32(v2)))
				case wazeroir.UnsignedTypeI64:
					vm.push(v1 * v2)
				case wazeroir.UnsignedTypeF32:
					v := math.Float32frombits(uint32(v2)) * math.Float32frombits(uint32(v1))
					vm.push(uint64(math.Float32bits(v)))
				case wazeroir.UnsignedTypeF64:
					v := math.Float64frombits(v2) * math.Float64frombits(v1)
					vm.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindClz:
			{
				v := vm.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					vm.push(uint64(bits.LeadingZeros32(uint32(v))))
				} else {
					// UnsignedInt64
					vm.push(uint64(bits.LeadingZeros64(v)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindCtz:
			{
				v := vm.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					vm.push(uint64(bits.TrailingZeros32(uint32(v))))
				} else {
					// UnsignedInt64
					vm.push(uint64(bits.TrailingZeros64(v)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindPopcnt:
			{
				v := vm.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					vm.push(uint64(bits.OnesCount32(uint32(v))))
				} else {
					// UnsignedInt64
					vm.push(uint64(bits.OnesCount64(v)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindDiv:
			{
				switch wazeroir.SignedType(op.b1) {
				case wazeroir.SignedTypeInt32:
					v2 := int32(vm.pop())
					v1 := int32(vm.pop())
					if v1 == math.MinInt32 && v2 == -1 {
						panic(wasm.ErrRuntimeIntegerOverflow)
					}
					vm.push(uint64(uint32(v1 / v2)))
				case wazeroir.SignedTypeInt64:
					v2 := int64(vm.pop())
					v1 := int64(vm.pop())
					if v1 == math.MinInt64 && v2 == -1 {
						panic(wasm.ErrRuntimeIntegerOverflow)
					}
					vm.push(uint64(v1 / v2))
				case wazeroir.SignedTypeUint32:
					v2 := uint32(vm.pop())
					v1 := uint32(vm.pop())
					vm.push(uint64(v1 / v2))
				case wazeroir.SignedTypeUint64:
					v2 := vm.pop()
					v1 := vm.pop()
					vm.push(v1 / v2)
				case wazeroir.SignedTypeFloat32:
					v2 := vm.pop()
					v1 := vm.pop()
					v := math.Float32frombits(uint32(v1)) / math.Float32frombits(uint32(v2))
					vm.push(uint64(math.Float32bits(v)))
				case wazeroir.SignedTypeFloat64:
					v2 := vm.pop()
					v1 := vm.pop()
					v := math.Float64frombits(v1) / math.Float64frombits(v2)
					vm.push(uint64(math.Float64bits(v)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindRem:
			{
				switch wazeroir.SignedInt(op.b1) {
				case wazeroir.SignedInt32:
					v2 := int32(vm.pop())
					v1 := int32(vm.pop())
					vm.push(uint64(uint32(v1 % v2)))
				case wazeroir.SignedInt64:
					v2 := int64(vm.pop())
					v1 := int64(vm.pop())
					vm.push(uint64(v1 % v2))
				case wazeroir.SignedUint32:
					v2 := uint32(vm.pop())
					v1 := uint32(vm.pop())
					vm.push(uint64(v1 % v2))
				case wazeroir.SignedUint64:
					v2 := vm.pop()
					v1 := vm.pop()
					vm.push(v1 % v2)
				}
				frame.pc++
			}
		case wazeroir.OperationKindAnd:
			{
				v2 := vm.pop()
				v1 := vm.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					vm.push(uint64(uint32(v2) & uint32(v1)))
				} else {
					// UnsignedInt64
					vm.push(uint64(v2 & v1))
				}
				frame.pc++
			}
		case wazeroir.OperationKindOr:
			{
				v2 := vm.pop()
				v1 := vm.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					vm.push(uint64(uint32(v2) | uint32(v1)))
				} else {
					// UnsignedInt64
					vm.push(uint64(v2 | v1))
				}
				frame.pc++
			}
		case wazeroir.OperationKindXor:
			{
				v2 := vm.pop()
				v1 := vm.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					vm.push(uint64(uint32(v2) ^ uint32(v1)))
				} else {
					// UnsignedInt64
					vm.push(uint64(v2 ^ v1))
				}
				frame.pc++
			}
		case wazeroir.OperationKindShl:
			{
				v2 := vm.pop()
				v1 := vm.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					vm.push(uint64(uint32(v1) << (uint32(v2) % 32)))
				} else {
					// UnsignedInt64
					vm.push(v1 << (v2 % 64))
				}
				frame.pc++
			}
		case wazeroir.OperationKindShr:
			{
				v2 := vm.pop()
				v1 := vm.pop()
				switch wazeroir.SignedInt(op.b1) {
				case wazeroir.SignedInt32:
					vm.push(uint64(int32(v1) >> (uint32(v2) % 32)))
				case wazeroir.SignedInt64:
					vm.push(uint64(int64(v1) >> (v2 % 64)))
				case wazeroir.SignedUint32:
					vm.push(uint64(uint32(v1) >> (uint32(v2) % 32)))
				case wazeroir.SignedUint64:
					vm.push(v1 >> (v2 % 64))
				}
				frame.pc++
			}
		case wazeroir.OperationKindRotl:
			{
				v2 := vm.pop()
				v1 := vm.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					vm.push(uint64(bits.RotateLeft32(uint32(v1), int(v2))))
				} else {
					// UnsignedInt64
					vm.push(uint64(bits.RotateLeft64(v1, int(v2))))
				}
				frame.pc++
			}
		case wazeroir.OperationKindRotr:
			{
				v2 := vm.pop()
				v1 := vm.pop()
				if op.b1 == 0 {
					// UnsignedInt32
					vm.push(uint64(bits.RotateLeft32(uint32(v1), -int(v2))))
				} else {
					// UnsignedInt64
					vm.push(uint64(bits.RotateLeft64(v1, -int(v2))))
				}
				frame.pc++
			}
		case wazeroir.OperationKindAbs:
			{
				if op.b1 == 0 {
					// Float32
					const mask uint32 = 1 << 31
					vm.push(uint64(uint32(vm.pop()) &^ mask))
				} else {
					// Float64
					const mask uint64 = 1 << 63
					vm.push(uint64(vm.pop() &^ mask))
				}
				frame.pc++
			}
		case wazeroir.OperationKindNeg:
			{
				if op.b1 == 0 {
					// Float32
					v := -math.Float32frombits(uint32(vm.pop()))
					vm.push(uint64(math.Float32bits(v)))
				} else {
					// Float64
					v := -math.Float64frombits(vm.pop())
					vm.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindCeil:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Ceil(float64(math.Float32frombits(uint32(vm.pop()))))
					vm.push(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Ceil(float64(math.Float64frombits(vm.pop())))
					vm.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindFloor:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Floor(float64(math.Float32frombits(uint32(vm.pop()))))
					vm.push(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Floor(float64(math.Float64frombits(vm.pop())))
					vm.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindTrunc:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Trunc(float64(math.Float32frombits(uint32(vm.pop()))))
					vm.push(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Trunc(float64(math.Float64frombits(vm.pop())))
					vm.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindNearest:
			{
				if op.b1 == 0 {
					// Float32
					f := math.Float32frombits(uint32(vm.pop()))
					vm.push(uint64(math.Float32bits(moremath.WasmCompatNearestF32(f))))
				} else {
					// Float64
					f := math.Float64frombits(vm.pop())
					vm.push(math.Float64bits(moremath.WasmCompatNearestF64(f)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindSqrt:
			{
				if op.b1 == 0 {
					// Float32
					v := math.Sqrt(float64(math.Float32frombits(uint32(vm.pop()))))
					vm.push(uint64(math.Float32bits(float32(v))))
				} else {
					// Float64
					v := math.Sqrt(float64(math.Float64frombits(vm.pop())))
					vm.push(math.Float64bits(v))
				}
				frame.pc++
			}
		case wazeroir.OperationKindMin:
			{
				if op.b1 == 0 {
					// Float32
					v2 := math.Float32frombits(uint32(vm.pop()))
					v1 := math.Float32frombits(uint32(vm.pop()))
					vm.push(uint64(math.Float32bits(float32(moremath.WasmCompatMin(float64(v1), float64(v2))))))
				} else {
					v2 := math.Float64frombits(vm.pop())
					v1 := math.Float64frombits(vm.pop())
					vm.push(math.Float64bits(moremath.WasmCompatMin(v1, v2)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindMax:
			{

				if op.b1 == 0 {
					// Float32
					v2 := math.Float32frombits(uint32(vm.pop()))
					v1 := math.Float32frombits(uint32(vm.pop()))
					vm.push(uint64(math.Float32bits(float32(moremath.WasmCompatMax(float64(v1), float64(v2))))))
				} else {
					// Float64
					v2 := math.Float64frombits(vm.pop())
					v1 := math.Float64frombits(vm.pop())
					vm.push(math.Float64bits(moremath.WasmCompatMax(v1, v2)))
				}
				frame.pc++
			}
		case wazeroir.OperationKindCopysign:
			{
				if op.b1 == 0 {
					// Float32
					v2 := uint32(vm.pop())
					v1 := uint32(vm.pop())
					const signbit = 1 << 31
					vm.push(uint64(v1&^signbit | v2&signbit))
				} else {
					// Float64
					v2 := vm.pop()
					v1 := vm.pop()
					const signbit = 1 << 63
					vm.push(v1&^signbit | v2&signbit)
				}
				frame.pc++
			}
		case wazeroir.OperationKindI32WrapFromI64:
			{
				vm.push(uint64(uint32(vm.pop())))
				frame.pc++
			}
		case wazeroir.OperationKindITruncFromF:
			{
				if op.b1 == 0 {
					// Float32
					switch wazeroir.SignedInt(op.b2) {
					case wazeroir.SignedInt32:
						v := math.Trunc(float64(math.Float32frombits(uint32(vm.pop()))))
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < math.MinInt32 || v > math.MaxInt32 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						vm.push(uint64(int32(v)))
					case wazeroir.SignedInt64:
						v := math.Trunc(float64(math.Float32frombits(uint32(vm.pop()))))
						res := int64(v)
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < math.MinInt64 || v >= math.MaxInt64 {
							// Note: math.MaxInt64 is rounded up to math.MaxInt64+1 in 64-bit float representation,
							// and that's why we use '>=' not '>' to check overflow.
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						vm.push(uint64(res))
					case wazeroir.SignedUint32:
						v := math.Trunc(float64(math.Float32frombits(uint32(vm.pop()))))
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < 0 || v > math.MaxUint32 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						vm.push(uint64(uint32(v)))
					case wazeroir.SignedUint64:
						v := math.Trunc(float64(math.Float32frombits(uint32(vm.pop()))))
						res := uint64(v)
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < 0 || v >= math.MaxUint64 {
							// Note: math.MaxUint64 is rounded up to math.MaxUint64+1 in 64-bit float representation,
							// and that's why we use '>=' not '>' to check overflow.
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						vm.push(res)
					}
				} else {
					// Float64
					switch wazeroir.SignedInt(op.b2) {
					case wazeroir.SignedInt32:
						v := math.Trunc(math.Float64frombits(vm.pop()))
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < math.MinInt32 || v > math.MaxInt32 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						vm.push(uint64(int32(v)))
					case wazeroir.SignedInt64:
						v := math.Trunc(math.Float64frombits(vm.pop()))
						res := int64(v)
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < math.MinInt64 || v >= math.MaxInt64 {
							// Note: math.MaxInt64 is rounded up to math.MaxInt64+1 in 64-bit float representation,
							// and that's why we use '>=' not '>' to check overflow.
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						vm.push(uint64(res))
					case wazeroir.SignedUint32:
						v := math.Trunc(math.Float64frombits(vm.pop()))
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < 0 || v > math.MaxUint32 {
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						vm.push(uint64(uint32(v)))
					case wazeroir.SignedUint64:
						v := math.Trunc(math.Float64frombits(vm.pop()))
						res := uint64(v)
						if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
							panic(wasm.ErrRuntimeInvalidConversionToInteger)
						} else if v < 0 || v >= math.MaxUint64 {
							// Note: math.MaxUint64 is rounded up to math.MaxUint64+1 in 64-bit float representation,
							// and that's why we use '>=' not '>' to check overflow.
							panic(wasm.ErrRuntimeIntegerOverflow)
						}
						vm.push(res)
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
						v := float32(int32(vm.pop()))
						vm.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(int32(vm.pop()))
						vm.push(math.Float64bits(v))
					}
				case wazeroir.SignedInt64:
					if op.b2 == 0 {
						// Float32
						v := float32(int64(vm.pop()))
						vm.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(int64(vm.pop()))
						vm.push(math.Float64bits(v))
					}
				case wazeroir.SignedUint32:
					if op.b2 == 0 {
						// Float32
						v := float32(uint32(vm.pop()))
						vm.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(uint32(vm.pop()))
						vm.push(math.Float64bits(v))
					}
				case wazeroir.SignedUint64:
					if op.b2 == 0 {
						// Float32
						v := float32(vm.pop())
						vm.push(uint64(math.Float32bits(v)))
					} else {
						// Float64
						v := float64(vm.pop())
						vm.push(math.Float64bits(v))
					}
				}
				frame.pc++
			}
		case wazeroir.OperationKindF32DemoteFromF64:
			{
				v := float32(math.Float64frombits(vm.pop()))
				vm.push(uint64(math.Float32bits(v)))
				frame.pc++
			}
		case wazeroir.OperationKindF64PromoteFromF32:
			{
				v := float64(math.Float32frombits(uint32(vm.pop())))
				vm.push(math.Float64bits(v))
				frame.pc++
			}
		case wazeroir.OperationKindExtend:
			{
				if op.b1 == 1 {
					// Signed.
					v := int64(int32(vm.pop()))
					vm.push(uint64(v))
				} else {
					v := uint64(uint32(vm.pop()))
					vm.push(v)
				}
				frame.pc++
			}
		}
	}
	vm.popFrame()
}
