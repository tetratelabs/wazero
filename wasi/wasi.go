package wasi

import (
	"encoding/binary"
	"fmt"
	"os"
	"reflect"

	"github.com/mathetake/gasm/hostfunc"
	"github.com/mathetake/gasm/wasm"
)

const wasiUnstableName = "wasi_unstable"

var Modules map[string]*wasm.Module

func init() {
	b := hostfunc.NewModuleBuilder()
	b.MustSetFunction(wasiUnstableName, "fd_write", fd_write)
	Modules = b.Done()
}

func fd_write(vm *wasm.VirtualMachine) reflect.Value {
	body := func(fd int32, iovsPtr int32, iovsLen int32, nwrittenPtr int32) (err int32) {
		if fd != 1 {
			panic(fmt.Errorf("invalid file descriptor: %d", fd))
		}

		var nwritten uint32
		for i := int32(0); i < iovsLen; i++ {
			iovPtr := iovsPtr + i*8
			offset := binary.LittleEndian.Uint32(vm.Memory[iovPtr:])
			l := binary.LittleEndian.Uint32(vm.Memory[iovPtr+4:])
			n, err := os.Stdout.Write(vm.Memory[offset : offset+l])
			if err != nil {
				panic(err)
			}
			nwritten += uint32(n)
		}
		binary.LittleEndian.PutUint32(vm.Memory[nwrittenPtr:], nwritten)
		return 0
	}
	return reflect.ValueOf(body)
}
