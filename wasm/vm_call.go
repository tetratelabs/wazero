package wasm

import (
	"fmt"
	"math"
	"reflect"

	"github.com/mathetake/gasm/wasm/buildoptions"
)

func call(vm *VirtualMachine) {
	vm.ActiveFrame.PC++
	index := vm.FetchUint32()
	currentF := vm.ActiveFrame.F
	addr := currentF.ModuleInstance.FunctionAddrs[index]
	nextF := vm.Store.Functions[addr]
	callIn(vm, nextF)
}

func callIndirect(vm *VirtualMachine) {
	currentModuleInst := vm.ActiveFrame.F.ModuleInstance

	vm.ActiveFrame.PC++
	typeIndex := vm.FetchUint32()
	expType := currentModuleInst.Types[typeIndex]

	// note: mvp limits the size of table index space to 1
	const tableIndex = 0
	vm.ActiveFrame.PC++ // skip 0x00 (table index)

	tableAddr := currentModuleInst.TableAddrs[tableIndex]
	tableInst := vm.Store.Tables[tableAddr]
	index := vm.Operands.Pop()
	functinAddr := tableInst.Table[index]
	f := vm.Store.Functions[*functinAddr]
	if !hasSameSignature(f.Signature.InputTypes, expType.InputTypes) ||
		!hasSameSignature(f.Signature.ReturnTypes, expType.ReturnTypes) {
		panic(fmt.Sprintf("function signature mismatch (%#x, %#x) != (%#x, %#x)",
			f.Signature.InputTypes, f.Signature.ReturnTypes, expType.InputTypes, expType.ReturnTypes))
	}
	callIn(vm, f)
}

func callIn(vm *VirtualMachine, nextF *FunctionInstance) {
	vm.ActiveFrame.PC++ // skip the current call instruction of the current frame.
	if nextF.HostFunction != nil {
		hostF := *nextF.HostFunction
		if buildoptions.IsDebugMode {
			fmt.Printf("call host function '%s'\n", nextF.Name)
		}
		tp := hostF.Type()
		in := make([]reflect.Value, tp.NumIn())
		for i := len(in) - 1; i >= 1; i-- {
			val := reflect.New(tp.In(i)).Elem()
			raw := vm.Operands.Pop()
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
		val := reflect.New(tp.In(0)).Elem()
		val.Set(reflect.ValueOf(vm))
		in[0] = val
		vm.Frames.Push(&VirtualMachineFrame{F: nextF})
		for _, ret := range hostF.Call(in) {
			switch ret.Kind() {
			case reflect.Float64, reflect.Float32:
				vm.Operands.Push(math.Float64bits(ret.Float()))
			case reflect.Uint32, reflect.Uint64:
				vm.Operands.Push(ret.Uint())
			case reflect.Int32, reflect.Int64:
				vm.Operands.Push(uint64(ret.Int()))
			default:
				panic("invalid return type")
			}
		}
		vm.Frames.Pop()
	} else {
		al := len(nextF.Signature.InputTypes)
		locals := make([]uint64, nextF.NumLocals+uint32(al))
		for i := 0; i < al; i++ {
			locals[al-1-i] = vm.Operands.Pop()
		}
		frame := &VirtualMachineFrame{
			F:      nextF,
			Locals: locals,
			Labels: NewVirtualMachineLabelStack(),
		}
		frame.Labels.Push(&Label{
			Arity:          len(nextF.Signature.ReturnTypes),
			ContinuationPC: uint64(len(nextF.Body)) - 1,
			OperandSP:      -1,
		})
		vm.Frames.Push(frame)
		vm.ActiveFrame = frame
	}
}
