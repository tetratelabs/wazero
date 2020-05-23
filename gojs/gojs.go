// go port of https://github.com/golang/go/blob/master/misc/wasm/wasm_exec.js
package gojs

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
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

func (s *state) setInt64(vm *wasm.VirtualMachine, addr uint32, v uint64) {
	binary.LittleEndian.PutUint64(vm.Memory[addr:], v)
}

func (s *state) getInt64(vm *wasm.VirtualMachine, addr uint32) int64 {
	return int64(binary.LittleEndian.Uint64(vm.Memory[addr:]))
}

func (s *state) loadValue(vm *wasm.VirtualMachine, addr uint32) reflect.Value {
	f := math.Float64frombits(binary.LittleEndian.Uint64(vm.Memory[addr:]))
	if f == 0 {
		return undefinedValue
	} else if f != math.NaN() {
		return reflect.ValueOf(f)
	}

	id := binary.LittleEndian.Uint32(vm.Memory[addr:])
	return s.idToValue[id]
}

func (s *state) loadSlice(vm *wasm.VirtualMachine, addr uint32) []byte {
	arr := s.getInt64(vm, addr)
	l := s.getInt64(vm, addr+8)
	return vm.Memory[arr:l]
}

func (s *state) loadSliceOfValues(vm *wasm.VirtualMachine, addr uint32) []reflect.Value {
	arr := uint32(s.getInt64(vm, addr))
	l := uint32(s.getInt64(vm, addr+8))
	ret := make([]reflect.Value, l)
	for i := uint32(0); i < l; i++ {
		ret[i] = s.loadValue(vm, arr+i*8)
	}
	return ret
}

func (s *state) loadString(vm *wasm.VirtualMachine, addr uint32) string {
	start := s.getInt64(vm, addr)
	l := s.getInt64(vm, addr+8)
	return string(vm.Memory[start:l])
}

func (s *state) storeValue(vm *wasm.VirtualMachine, addr uint32, v reflect.Value) {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if float64(v.Int()) == math.NaN() {
			s.setNan(vm, addr)
			return
		} else if v.Int() != 0 {

			binary.LittleEndian.PutUint64(vm.Memory[addr:], uint64(v.Int()))
			return
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if float64(v.Int()) == math.NaN() {
			s.setNan(vm, addr)
			return
		} else if v.Uint() != 0 {
			binary.LittleEndian.PutUint64(vm.Memory[addr:], v.Uint())
			return
		}
	case reflect.Float32, reflect.Float64:
		if v.Float() == math.NaN() {
			s.setNan(vm, addr)
			return
		} else if v.Float() != 0 {
			binary.LittleEndian.PutUint64(vm.Memory[addr:], math.Float64bits(v.Float()))
			return
		}
	}

	if v.Type() == reflect.TypeOf(undefined{}) {
		binary.LittleEndian.PutUint64(vm.Memory[addr:], 0)
		return
	}

	id, ok := s.valueToID[v]
	if !ok {
		if len(s.idPool) == 0 {
			id = uint32(len(s.idToValue)) + 1
		}
		s.idToValue[id] = v
		s.goRefCounts[id] = 0
		s.valueToID[v] = id
	}

	s.goRefCounts[id]++
	var typeFlag uint32
	switch v.Kind() {
	case reflect.Array:
		typeFlag = 1
	case reflect.String:
		typeFlag = 2
	// fixme:
	// case Symbol: how should we deal with Symbol?
	case reflect.Func:
		typeFlag = 4
	}

	binary.LittleEndian.PutUint32(vm.Memory[addr:], nanHHead|typeFlag)
	binary.LittleEndian.PutUint32(vm.Memory[addr+4:], id)
}

func (s *state) setNan(vm *wasm.VirtualMachine, addr uint32) {
	binary.LittleEndian.PutUint32(vm.Memory[addr:], nanHHead)
	binary.LittleEndian.PutUint32(vm.Memory[addr+4:], 0)
}
