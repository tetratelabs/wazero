package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"reflect"

	"github.com/mathetake/gasm/hostmodule"
	"github.com/mathetake/gasm/wasi"
	"github.com/mathetake/gasm/wasm"
)

func main() {
	buf, err := ioutil.ReadFile("./host_func.wasm")
	if err != nil {
		panic(err)
	}

	mod, err := wasm.DecodeModule(bytes.NewBuffer(buf))
	if err != nil {
		panic(err)
	}

	var cnt uint64
	hostFunc := func(*wasm.VirtualMachine) reflect.Value {
		return reflect.ValueOf(func() {
			cnt++
		})
	}

	builder := hostmodule.NewBuilderWith(wasi.Modules)
	builder.MustAddFunction("env", "host_func", hostFunc)
	if err := mod.BuildIndexSpaces(builder.Done()); err != nil {
		panic(err)
	}

	vm, err := wasm.NewVM(mod)
	if err != nil {
		panic(vm)
	}

	for _, exp := range []uint64{5, 10, 15} {
		_, _, err = vm.ExecExportedFunction("call_host_func", exp)
		if err != nil {
			panic(err)
		}
		if cnt != exp {
			panic(fmt.Sprintf("want %d but got %d", exp, cnt))
		}
		cnt = 0
	}
}
