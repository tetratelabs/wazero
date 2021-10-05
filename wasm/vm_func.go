package wasm

import (
	"fmt"
	"math"
	"reflect"
	"time"
)

type (
	HostFunction struct {
		Name             string
		ClosureGenerator func(vm *VirtualMachine) reflect.Value
		function         reflect.Value // should be set at the time of VM creation
		Signature        *FunctionType
	}
	NativeFunction struct {
		Signature      *FunctionType
		NumLocal       uint32
		Body           []byte
		Blocks         map[uint64]*NativeFunctionBlock
		ModuleInstance *ModuleInstance
	}
	NativeFunctionBlock struct {
		StartAt, ElseAt, EndAt uint64
		BlockType              *FunctionType
		BlockTypeBytes         uint64
	}
)

var (
	_ FuncInstance = &HostFunction{}
	_ FuncInstance = &NativeFunction{}
)

func (h *HostFunction) FunctionType() *FunctionType {
	return h.Signature
}

func (n *NativeFunction) FunctionType() *FunctionType {
	return n.Signature
}

func (h *HostFunction) Call(vm *VirtualMachine) {
	if isDebugMode {
		fmt.Printf("Call host function '%s'\n", h.Name)
	}
	tp := h.function.Type()
	in := make([]reflect.Value, tp.NumIn())
	for i := len(in) - 1; i >= 0; i-- {
		val := reflect.New(tp.In(i)).Elem()
		raw := vm.OperandStack.Pop()
		kind := tp.In(i).Kind()

		switch kind {
		case reflect.Float64, reflect.Float32:
			val.SetFloat(math.Float64frombits(raw))
		case reflect.Uint32, reflect.Uint64:
			val.SetUint(raw)
		case reflect.Int32, reflect.Int64:
			val.SetInt(int64(raw))
		default:
			panic("invalid input type")
		}
		in[i] = val
	}

	for _, ret := range h.function.Call(in) {
		switch ret.Kind() {
		case reflect.Float64, reflect.Float32:
			vm.OperandStack.Push(math.Float64bits(ret.Float()))
		case reflect.Uint32, reflect.Uint64:
			vm.OperandStack.Push(ret.Uint())
		case reflect.Int32, reflect.Int64:
			vm.OperandStack.Push(uint64(ret.Int()))
		default:
			panic("invalid return type")
		}
	}
}

func (n *NativeFunction) Call(vm *VirtualMachine) {
	al := len(n.Signature.InputTypes)
	locals := make([]uint64, n.NumLocal+uint32(al))
	for i := 0; i < al; i++ {
		locals[al-1-i] = vm.OperandStack.Pop()
	}

	prev := vm.ActiveContext
	labelStack := NewVirtualMachineLabelStack()
	labelStack.Push(&Label{
		Arity:          len(n.Signature.ReturnTypes),
		ContinuationPC: uint64(len(n.Body)),
		OperandSP:      -1,
	})
	vm.ActiveContext = &NativeFunctionContext{
		Function:   n,
		Locals:     locals,
		LabelStack: labelStack,
	}
	vm.execNativeFunction()
	vm.ActiveContext = prev
}

func (vm *VirtualMachine) execNativeFunction() {
	bl := len(vm.ActiveContext.Function.Body)
	for int(vm.ActiveContext.PC) < bl && !vm.ActiveContext.Returned {
		op := vm.ActiveContext.Function.Body[vm.ActiveContext.PC]
		if isDebugMode {
			fmt.Printf("0x%x: op=%s (Label SP=%d, Operand SP=%d) \n", vm.ActiveContext.PC, optcodeStrs[op], vm.ActiveContext.LabelStack.SP, vm.OperandStack.SP)
			time.Sleep(time.Millisecond)
		}
		virtualMachineInstructions[op](vm)
	}
}
