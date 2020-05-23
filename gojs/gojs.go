// go port of https://github.com/golang/go/blob/master/misc/wasm/wasm_exec.js
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

const (
	goModuleName = "go"
	nanHHead     = 0x7FF80000
)

var undefinedValue = reflect.ValueOf(undefined{})

type undefined struct{}

func New() map[string]*wasm.Module {

	s := new(state)
	b := hostfunc.NewModuleBuilder()
	b.MustSetFunction(goModuleName, "debug", s.debug)
	b.MustSetFunction(goModuleName, "runtime.resetMemoryDataView", s.runtimeResetMemoryDataView)
	b.MustSetFunction(goModuleName, "runtime.wasmExit", s.runtimeWasmExit)
	b.MustSetFunction(goModuleName, "runtime.nanotime1", s.runtimeNanotime1)
	b.MustSetFunction(goModuleName, "runtime.walltime1", s.runtimeWallime1)
	b.MustSetFunction(goModuleName, "runtime.scheduleTimeoutEvent", s.runtimeScheduleTimeoutEvent)
	b.MustSetFunction(goModuleName, "runtime.clearTimeoutEvent", s.runtimeClearTimeoutEvent)
	b.MustSetFunction(goModuleName, "runtime.getRandomData", s.runtimeGetRandomData)
	b.MustSetFunction(goModuleName, "runtime.wasmWrite", s.runtimeWasmWrite)
	b.MustSetFunction(goModuleName, "syscall/js.finalizeRef", s.finalizeRef)
	b.MustSetFunction(goModuleName, "syscall/js.stringVal", s.stringVal)
	b.MustSetFunction(goModuleName, "syscall/js.valueGet", s.valueGet)

	return b.Done()
}

type state struct {
	idToValue   map[uint32]reflect.Value
	idPool      []uint32
	valueToID   map[reflect.Value]uint32
	goRefCounts map[uint32]int
}

func (s *state) debug(*wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		fmt.Println("debug: ", v)
	})
}

func (s *state) runtimeResetMemoryDataView(*wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {})
}

func (s *state) runtimeWasmExit(vm *wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		code := binary.LittleEndian.Uint32(vm.Memory[v+8:])
		fmt.Println("exit: ", code)
		vm.Exited = true
	})
}

func (s *state) runtimeNanotime1(vm *wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		binary.LittleEndian.PutUint64(vm.Memory[v+8:], uint64(time.Now().UnixNano()))
	})
}
func (s *state) runtimeWallime1(vm *wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		now := time.Now()
		binary.LittleEndian.PutUint64(vm.Memory[v+8:], uint64(now.Unix()))
		binary.LittleEndian.PutUint64(vm.Memory[v+16:], uint64(now.UnixNano()))
	})
}

func (s *state) runtimeScheduleTimeoutEvent(vm *wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		// note: intentionally leave this empty since I am not sure what to implement
		// fixme: does anyone have an idea?
		// https://github.com/golang/go/blob/f9640b88c7e5f4df3350643f3ec6c30c30e8678d/src/runtime/lock_js.go#L219-L221
	})
}

func (s *state) runtimeClearTimeoutEvent(vm *wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		// note: intentionally leave this empty since I am not sure what to implement
		// fixme: does anyone have an idea?
		// https://github.com/golang/go/blob/f9640b88c7e5f4df3350643f3ec6c30c30e8678d/src/runtime/lock_js.go#L223-L224
	})
}

func (s *state) runtimeGetRandomData(vm *wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		token := make([]byte, 4)
		_, err := rand.Read(token)
		if err != nil {
			panic(err)
		}
		copy(vm.Memory[v+8:v+12], token)
	})
}

func (s *state) runtimeWasmWrite(vm *wasm.VirtualMachine) reflect.Value {
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
