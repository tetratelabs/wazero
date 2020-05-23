package gojs

import (
	"encoding/binary"
	"math"
	"reflect"

	"github.com/mathetake/gasm/wasm"
)

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
			id = uint32(len(s.idToValue))
		}
		s.idToValue[id] = v
		s.goRefCounts[id] = 0
		s.valueToID[v] = id
	}

	s.goRefCounts[id]++
	var typeFlag uint32
	switch v.Kind() {
	case reflect.Struct:
		typeFlag = 1 //
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

func (s *state) finalizeRef(vm *wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		id := binary.LittleEndian.Uint32(vm.Memory[v+8:])
		s.goRefCounts[id]--
		if s.goRefCounts[id] == 0 {
			v := s.idToValue[id]
			delete(s.idToValue, id)
			delete(s.valueToID, v)
			s.idPool = append(s.idPool, id)
		}
	})
}

func (s *state) stringVal(vm *wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		s.storeValue(vm, v+24, reflect.ValueOf(s.loadString(vm, v+8)))
	})
}

func (s *state) valueGet(vm *wasm.VirtualMachine) reflect.Value {
	return reflect.ValueOf(func(v uint32) {
		val := s.loadValue(vm, v+8)
		result := val.FieldByName(s.loadString(vm, v+16))
		s.storeValue(vm, uint32(vm.OperandStack.SP+32), result)
	})
}
