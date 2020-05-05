package main

import (
	"bytes"
	"fmt"
	"io/ioutil"

	"github.com/mathetake/gasm/wasi"
	"github.com/mathetake/gasm/wasm"
)

func main() {
	buf, err := ioutil.ReadFile("./fib.wasm")
	if err != nil {
		panic(err)
	}

	mod, err := wasm.DecodeModule(bytes.NewBuffer(buf))
	if err != nil {
		panic(err)
	}

	if err := mod.BuildIndexSpaces(wasi.Modules); err != nil {
		panic(err)
	}

	vm, err := wasm.NewVM(mod)
	if err != nil {
		panic(vm)
	}

	for _, c := range []struct {
		in, exp int32
	}{
		{in: 20, exp: 6765},
		{in: 10, exp: 55},
		{in: 5, exp: 5},
	} {
		ret, retTypes, err := vm.ExecExportedFunction("fib", uint64(c.in))
		if err != nil {
			panic(err)
		}

		if len(ret) != 1 || len(retTypes) != 1 {
			panic("invalid return")
		}

		if retTypes[0] != wasm.ValueTypeI32 {
			panic("return type must be i32")
		}

		if int32(ret[0]) != c.exp {
			panic(fmt.Sprintf("result must be %d but got %d\n", c.exp, ret[0]))
		}
	}
}
