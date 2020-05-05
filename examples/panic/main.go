package main

import (
	"bytes"
	"io/ioutil"

	"github.com/mathetake/gasm/wasi"
	"github.com/mathetake/gasm/wasm"
)

func main() {
	buf, err := ioutil.ReadFile("./panic.wasm")
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

	_, _, err = vm.ExecExportedFunction("cause_panic")
	if err != nil {
		panic(err)
	}
}
