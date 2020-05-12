package gojs

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/mathetake/gasm/hostfunc"
	"github.com/mathetake/gasm/wasm"
)

const goModuleName = "go"

var Modules map[string]*wasm.Module

func init() {
	b := hostfunc.NewModuleBuilder()
	b.MustSetFunction(goModuleName, "debug", debug)
	b.MustSetFunction(goModuleName, "runtime.resetMemoryDataView", runtimeResetMemoryDataView)
	b.MustSetFunction(goModuleName, "runtime.wasmExit", runtimeWasmExit)
	b.MustSetFunction(goModuleName, "runtime.nanotime1", runtimeNanotime1)
	b.MustSetFunction(goModuleName, "runtime.walltime1", runtimeWallime1)
	b.MustSetFunction(goModuleName, "runtime.scheduleTimeoutEvent", runtimeScheduleTimeoutEvent)
	b.MustSetFunction(goModuleName, "runtime.clearTimeoutEvent", runtimeClearTimeoutEvent)
	b.MustSetFunction(goModuleName, "runtime.getRandomData", runtimeGetRandomData)
	b.MustSetFunction(goModuleName, "runtime.wasmWrite", runtimeWasmWrite)
	Modules = b.Done()
}

func debug(*wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		fmt.Println("debug: ", v)
	})
}

func runtimeResetMemoryDataView(*wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {})
}

func runtimeWasmExit(vm *wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		code := binary.LittleEndian.Uint32(vm.Memory[v+8:])
		fmt.Println("exit: ", code)
		vm.Exited = true
	})
}

func runtimeNanotime1(vm *wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		binary.LittleEndian.PutUint64(vm.Memory[v+8:], uint64(time.Now().UnixNano()))
	})
}
func runtimeWallime1(vm *wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		now := time.Now()
		binary.LittleEndian.PutUint64(vm.Memory[v+8:], uint64(now.Unix()))
		binary.LittleEndian.PutUint64(vm.Memory[v+16:], uint64(now.UnixNano()))
	})
}

func runtimeScheduleTimeoutEvent(vm *wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		// note: intentionally leave this unimplemented since
		// I am not sure what to implement
		// https://github.com/golang/go/blob/f9640b88c7e5f4df3350643f3ec6c30c30e8678d/src/runtime/lock_js.go#L219-L221
	})
}

func runtimeClearTimeoutEvent(vm *wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		// note: intentionally leave this unimplemented since
		// I am not sure what to implement
		// https://github.com/golang/go/blob/f9640b88c7e5f4df3350643f3ec6c30c30e8678d/src/runtime/lock_js.go#L223-L224
	})
}

func runtimeGetRandomData(vm *wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		token := make([]byte, 4)
		_, err := rand.Read(token)
		if err != nil {
			panic(err)
		}
		copy(vm.Memory[v+8:v+12], token)
	})
}

func runtimeWasmWrite(vm *wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		fd := binary.LittleEndian.Uint32(vm.Memory[v+8:])
		if fd != 1 {
			// fixme: support any fd
			panic(fmt.Errorf("invalid file descriptor: %d", fd))
		}
		offset := binary.LittleEndian.Uint32(vm.Memory[v+16:])
		l := binary.LittleEndian.Uint32(vm.Memory[v+24:])
		_, err := os.Stdout.Write(vm.Memory[offset : offset+l])
		if err != nil {
			panic(err)
		}
	})
}
