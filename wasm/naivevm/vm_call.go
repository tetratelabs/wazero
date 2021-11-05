package naivevm

import (
	"fmt"
	"math"
	"reflect"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/buildoptions"
)

func call(vm *naiveVirtualMachine) {
	vm.activeFrame.pc++
	index := vm.FetchUint32()
	currentF := vm.activeFrame.f
	nextF := currentF.ModuleInstance.Functions[index]
	callIn(vm, nextF)
}

func callIndirect(vm *naiveVirtualMachine) {
	currentModuleInst := vm.activeFrame.f.ModuleInstance

	vm.activeFrame.pc++
	typeIndex := vm.FetchUint32()
	expType := currentModuleInst.Types[typeIndex]

	// note: mvp limits the size of table index space to 1
	const tableIndex = 0
	vm.activeFrame.pc++ // skip 0x00 (table index)

	tableInst := currentModuleInst.Tables[tableIndex]
	index := vm.operands.pop()
	tableElm := tableInst.Table[index]
	f := tableElm.Function
	if !wasm.HasSameSignature(f.Signature.InputTypes, expType.InputTypes) ||
		!wasm.HasSameSignature(f.Signature.ReturnTypes, expType.ReturnTypes) {
		panic(fmt.Sprintf("function signature mismatch (%#x, %#x) != (%#x, %#x)",
			f.Signature.InputTypes, f.Signature.ReturnTypes, expType.InputTypes, expType.ReturnTypes))
	}
	callIn(vm, f)
}

func callIn(vm *naiveVirtualMachine, nextF *wasm.FunctionInstance) {
	vm.activeFrame.pc++ // skip the current call instruction of the current frame.
	if nextF.HostFunction != nil {
		hostF := *nextF.HostFunction
		if buildoptions.IsDebugMode {
			fmt.Printf("call host function '%s'\n", nextF.Name)
		}
		tp := hostF.Type()
		in := make([]reflect.Value, tp.NumIn())
		for i := len(in) - 1; i >= 1; i-- {
			val := reflect.New(tp.In(i)).Elem()
			raw := vm.operands.pop()
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
		val.Set(reflect.ValueOf(
			&wasm.HostFunctionCallContext{Memory: vm.activeFrame.f.ModuleInstance.Memory},
		))

		in[0] = val
		vm.frames.push(&frame{f: nextF})
		for _, ret := range hostF.Call(in) {
			switch ret.Kind() {
			case reflect.Float64, reflect.Float32:
				vm.operands.push(math.Float64bits(ret.Float()))
			case reflect.Uint32, reflect.Uint64:
				vm.operands.push(ret.Uint())
			case reflect.Int32, reflect.Int64:
				vm.operands.push(uint64(ret.Int()))
			default:
				panic("invalid return type")
			}
		}
		vm.frames.pop()
	} else {
		al := len(nextF.Signature.InputTypes)
		locals := make([]uint64, nextF.NumLocals+uint32(al))
		for i := 0; i < al; i++ {
			locals[al-1-i] = vm.operands.pop()
		}
		frame := &frame{
			f:      nextF,
			locals: locals,
			labels: newLabelStack(),
		}
		frame.labels.push(&label{
			arity:          len(nextF.Signature.ReturnTypes),
			continuationPC: uint64(len(nextF.Body)) - 1,
			operandSP:      -1,
		})
		vm.frames.push(frame)
		vm.activeFrame = frame
	}
}
