package wasm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"

	"github.com/mathetake/gasm/wasm/leb128"
)

const PageSize uint64 = 65536

type (
	VirtualMachine struct {
		Store         *Store
		ActiveContext *NativeFunctionContext
		OperandStack  *VirtualMachineOperandStack
	}

	NativeFunctionContext struct {
		PC         uint64
		Function   *NativeFunction
		Locals     []uint64
		LabelStack *VirtualMachineLabelStack
		Returned   bool
	}
)

func NewVM() (*VirtualMachine, error) {
	return &VirtualMachine{
		Store:        NewStore(),
		OperandStack: NewVirtualMachineOperandStack(),
	}, nil
}

func (vm *VirtualMachine) InstantiateModule(module *Module, name string) (errRet error) {
	inst, err := vm.Store.Instantiate(module, name)
	if err != nil {
		return err
	}

	defer func() {
		if err := recover(); err != nil {
			errRet = fmt.Errorf("calling start function failed: %v", err)
		}
	}()

	if module.StartSection != nil {
		vm.Store.Functions[inst.FunctionAddrs[*module.StartSection]].Call(vm)
	}
	return
}

func (vm *VirtualMachine) ExecExportedFunction(moduleName, funcName string, args ...uint64) (returns []uint64, returnTypes []ValueType, err error) {
	m, ok := vm.Store.ModuleInstances[moduleName]
	if !ok {
		return nil, nil, fmt.Errorf("module '%s' not instantiated", moduleName)
	}

	exp, ok := m.Exports[funcName]
	if !ok {
		return nil, nil, fmt.Errorf("exported function '%s' not found in '%s'", funcName, moduleName)
	}

	if exp.Kind != ExportKindFunction {
		return nil, nil, fmt.Errorf("'%s' is not functype", funcName)
	}

	f := vm.Store.Functions[exp.Addr]
	if len(f.FunctionType().InputTypes) != len(args) {
		return nil, nil, fmt.Errorf("invalid number of arguments")
	}

	for _, arg := range args {
		vm.OperandStack.Push(arg)
	}

	f.Call(vm)

	ret := make([]uint64, len(f.FunctionType().ReturnTypes))
	for i := range ret {
		ret[len(ret)-1-i] = vm.OperandStack.Pop()
	}
	return ret, f.FunctionType().ReturnTypes, nil
}

func (vm *VirtualMachine) AddHostFunction(moduleName, funcName string, fn reflect.Value) error {
	return vm.Store.AddHostFunction(moduleName, funcName, fn)
}

func (vm *VirtualMachine) AddGlobal(moduleName, funcName string, value uint64, valueType ValueType, mutable bool) error {
	return vm.Store.AddGlobal(moduleName, funcName, value, valueType, mutable)
}

func (vm *VirtualMachine) AddTableInstance(moduleName, funcName string, min uint32, max *uint32) error {
	return vm.Store.AddTableInstance(moduleName, funcName, min, max)
}

func (vm *VirtualMachine) AddMemoryInstance(moduleName, funcName string, min uint32, max *uint32) error {
	return vm.Store.AddMemoryInstance(moduleName, funcName, min, max)
}

func (vm *VirtualMachine) FetchInt32() int32 {
	ret, num, err := leb128.DecodeInt32(bytes.NewBuffer(
		vm.ActiveContext.Function.Body[vm.ActiveContext.PC:]))
	if err != nil {
		panic(err)
	}
	vm.ActiveContext.PC += num - 1
	return ret
}

func (vm *VirtualMachine) FetchUint32() uint32 {
	ret, num, err := leb128.DecodeUint32(bytes.NewBuffer(
		vm.ActiveContext.Function.Body[vm.ActiveContext.PC:]))
	if err != nil {
		panic(err)
	}
	vm.ActiveContext.PC += num - 1
	return ret
}

func (vm *VirtualMachine) FetchInt64() int64 {
	ret, num, err := leb128.DecodeInt64(bytes.NewBuffer(
		vm.ActiveContext.Function.Body[vm.ActiveContext.PC:]))
	if err != nil {
		panic(err)
	}
	vm.ActiveContext.PC += num - 1
	return ret
}

func (vm *VirtualMachine) FetchFloat32() float32 {
	v := math.Float32frombits(binary.LittleEndian.Uint32(
		vm.ActiveContext.Function.Body[vm.ActiveContext.PC:]))
	vm.ActiveContext.PC += 3
	return v
}

func (vm *VirtualMachine) FetchFloat64() float64 {
	v := math.Float64frombits(binary.LittleEndian.Uint64(
		vm.ActiveContext.Function.Body[vm.ActiveContext.PC:]))
	vm.ActiveContext.PC += 7
	return v
}

var virtualMachineInstructions = [256]func(vm *VirtualMachine){
	OptCodeUnreachable:       func(vm *VirtualMachine) { panic("unreachable") },
	OptCodeNop:               func(vm *VirtualMachine) { vm.ActiveContext.PC++ },
	OptCodeBlock:             block,
	OptCodeLoop:              loop,
	OptCodeIf:                ifOp,
	OptCodeElse:              elseOp,
	OptCodeEnd:               end,
	OptCodeBr:                br,
	OptCodeBrIf:              brIf,
	OptCodeBrTable:           brTable,
	OptCodeReturn:            func(vm *VirtualMachine) { vm.ActiveContext.PC++; vm.ActiveContext.Returned = true },
	OptCodeCall:              call,
	OptCodeCallIndirect:      callIndirect,
	OptCodeDrop:              drop,
	OptCodeSelect:            selectOp,
	OptCodeLocalGet:          getLocal,
	OptCodeLocalSet:          setLocal,
	OptCodeLocalTee:          teeLocal,
	OptCodeGlobalGet:         getGlobal,
	OptCodeGlobalSet:         setGlobal,
	OptCodeI32Load:           i32Load,
	OptCodeI64Load:           i64Load,
	OptCodeF32Load:           f32Load,
	OptCodeF64Load:           f64Load,
	OptCodeI32Load8s:         i32Load8s,
	OptCodeI32Load8u:         i32Load8u,
	OptCodeI32Load16s:        i32Load16s,
	OptCodeI32Load16u:        i32Load16u,
	OptCodeI64Load8s:         i64Load8s,
	OptCodeI64Load8u:         i64Load8u,
	OptCodeI64Load16s:        i64Load16s,
	OptCodeI64Load16u:        i64Load16u,
	OptCodeI64Load32s:        i64Load32s,
	OptCodeI64Load32u:        i64Load32u,
	OptCodeI32Store:          i32Store,
	OptCodeI64Store:          i64Store,
	OptCodeF32Store:          f32Store,
	OptCodeF64Store:          f64Store,
	OptCodeI32Store8:         i32Store8,
	OptCodeI32Store16:        i32Store16,
	OptCodeI64Store8:         i64Store8,
	OptCodeI64Store16:        i64Store16,
	OptCodeI64Store32:        i64Store32,
	OptCodeMemorySize:        memorySize,
	OptCodeMemoryGrow:        memoryGrow,
	OptCodeI32Const:          i32Const,
	OptCodeI64Const:          i64Const,
	OptCodeF32Const:          f32Const,
	OptCodeF64Const:          f64Const,
	OptCodeI32eqz:            i32eqz,
	OptCodeI32eq:             i32eq,
	OptCodeI32ne:             i32ne,
	OptCodeI32lts:            i32lts,
	OptCodeI32ltu:            i32ltu,
	OptCodeI32gts:            i32gts,
	OptCodeI32gtu:            i32gtu,
	OptCodeI32les:            i32les,
	OptCodeI32leu:            i32leu,
	OptCodeI32ges:            i32ges,
	OptCodeI32geu:            i32geu,
	OptCodeI64eqz:            i64eqz,
	OptCodeI64eq:             i64eq,
	OptCodeI64ne:             i64ne,
	OptCodeI64lts:            i64lts,
	OptCodeI64ltu:            i64ltu,
	OptCodeI64gts:            i64gts,
	OptCodeI64gtu:            i64gtu,
	OptCodeI64les:            i64les,
	OptCodeI64leu:            i64leu,
	OptCodeI64ges:            i64ges,
	OptCodeI64geu:            i64geu,
	OptCodeF32eq:             f32eq,
	OptCodeF32ne:             f32ne,
	OptCodeF32lt:             f32lt,
	OptCodeF32gt:             f32gt,
	OptCodeF32le:             f32le,
	OptCodeF32ge:             f32ge,
	OptCodeF64eq:             f64eq,
	OptCodeF64ne:             f64ne,
	OptCodeF64lt:             f64lt,
	OptCodeF64gt:             f64gt,
	OptCodeF64le:             f64le,
	OptCodeF64ge:             f64ge,
	OptCodeI32clz:            i32clz,
	OptCodeI32ctz:            i32ctz,
	OptCodeI32popcnt:         i32popcnt,
	OptCodeI32add:            i32add,
	OptCodeI32sub:            i32sub,
	OptCodeI32mul:            i32mul,
	OptCodeI32divs:           i32divs,
	OptCodeI32divu:           i32divu,
	OptCodeI32rems:           i32rems,
	OptCodeI32remu:           i32remu,
	OptCodeI32and:            i32and,
	OptCodeI32or:             i32or,
	OptCodeI32xor:            i32xor,
	OptCodeI32shl:            i32shl,
	OptCodeI32shrs:           i32shrs,
	OptCodeI32shru:           i32shru,
	OptCodeI32rotl:           i32rotl,
	OptCodeI32rotr:           i32rotr,
	OptCodeI64clz:            i64clz,
	OptCodeI64ctz:            i64ctz,
	OptCodeI64popcnt:         i64popcnt,
	OptCodeI64add:            i64add,
	OptCodeI64sub:            i64sub,
	OptCodeI64mul:            i64mul,
	OptCodeI64divs:           i64divs,
	OptCodeI64divu:           i64divu,
	OptCodeI64rems:           i64rems,
	OptCodeI64remu:           i64remu,
	OptCodeI64and:            i64and,
	OptCodeI64or:             i64or,
	OptCodeI64xor:            i64xor,
	OptCodeI64shl:            i64shl,
	OptCodeI64shrs:           i64shrs,
	OptCodeI64shru:           i64shru,
	OptCodeI64rotl:           i64rotl,
	OptCodeI64rotr:           i64rotr,
	OptCodeF32abs:            f32abs,
	OptCodeF32neg:            f32neg,
	OptCodeF32ceil:           f32ceil,
	OptCodeF32floor:          f32floor,
	OptCodeF32trunc:          f32trunc,
	OptCodeF32nearest:        f32nearest,
	OptCodeF32sqrt:           f32sqrt,
	OptCodeF32add:            f32add,
	OptCodeF32sub:            f32sub,
	OptCodeF32mul:            f32mul,
	OptCodeF32div:            f32div,
	OptCodeF32min:            f32min,
	OptCodeF32max:            f32max,
	OptCodeF32copysign:       f32copysign,
	OptCodeF64abs:            f64abs,
	OptCodeF64neg:            f64neg,
	OptCodeF64ceil:           f64ceil,
	OptCodeF64floor:          f64floor,
	OptCodeF64trunc:          f64trunc,
	OptCodeF64nearest:        f64nearest,
	OptCodeF64sqrt:           f64sqrt,
	OptCodeF64add:            f64add,
	OptCodeF64sub:            f64sub,
	OptCodeF64mul:            f64mul,
	OptCodeF64div:            f64div,
	OptCodeF64min:            f64min,
	OptCodeF64max:            f64max,
	OptCodeF64copysign:       f64copysign,
	OptCodeI32wrapI64:        i32wrapi64,
	OptCodeI32truncf32s:      i32truncf32s,
	OptCodeI32truncf32u:      i32truncf32u,
	OptCodeI32truncf64s:      i32truncf64s,
	OptCodeI32truncf64u:      i32truncf64u,
	OptCodeI64Extendi32s:     i64extendi32s,
	OptCodeI64Extendi32u:     i64extendi32u,
	OptCodeI64TruncF32s:      i64truncf32s,
	OptCodeI64TruncF32u:      i64truncf32u,
	OptCodeI64Truncf64s:      i64truncf64s,
	OptCodeI64Truncf64u:      i64truncf64u,
	OptCodeF32Converti32s:    f32converti32s,
	OptCodeF32Converti32u:    f32converti32u,
	OptCodeF32Converti64s:    f32converti64s,
	OptCodeF32Converti64u:    f32converti64u,
	OptCodeF32Demotef64:      f32demotef64,
	OptCodeF64Converti32s:    f64converti32s,
	OptCodeF64Converti32u:    f64converti32u,
	OptCodeF64Converti64s:    f64converti64s,
	OptCodeF64Converti64u:    f64converti64u,
	OptCodeF64Promotef32:     f64promotef32,
	OptCodeI32reinterpretf32: func(vm *VirtualMachine) { vm.ActiveContext.PC++ },
	OptCodeI64reinterpretf64: func(vm *VirtualMachine) { vm.ActiveContext.PC++ },
	OptCodeF32reinterpreti32: func(vm *VirtualMachine) { vm.ActiveContext.PC++ },
	OptCodeF64reinterpreti64: func(vm *VirtualMachine) { vm.ActiveContext.PC++ },
}
