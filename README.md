# gasm

[![CircleCI](https://circleci.com/gh/mathetake/gasm.svg?style=shield&circle-token=89ec47a30847c650d215699c0a99c8732a2d538d	)](https://circleci.com/gh/mathetake/gasm)
[![MIT License](http://img.shields.io/badge/license-MIT-blue.svg?style=flat)](LICENSE)

A minimal implementation of v1 WASM spec compatible virtual machine purely written in go.
The vm can be embedded in your go program without any dependency like cgo, and enables Gophers to 
write wasm host environments easily.

The vm should be used only for providing _sandbox_ environments embedded in your Go program since
 we have not implemented [validation](https://webassembly.github.io/spec/core/valid/index.html) of wasm binary.

The implementation is quite straightforward and I hope this code would be a
 good starting point for novices to learn WASM spec.

## examples

Full examples can be found at: https://github.com/mathetake/gasm/tree/master/examples

### call expoerted function from host

```golang
func Test_fibonacci(t *testing.T) {
	buf, _ := ioutil.ReadFile("wasm/fibonacci.wasm")
	mod, _ := wasm.DecodeModule(bytes.NewBuffer(buf))
	vm, _ := wasm.NewVM(mod, wasi.Modules)

	for _, c := range []struct {
		in, exp int32
	}{
		{in: 20, exp: 6765},
		{in: 10, exp: 55},
		{in: 5, exp: 5},
	} {
		ret, _, _ := vm.ExecExportedFunction("fib", uint64(c.in))
		require.Equal(t, c.exp, int32(ret[0]))
	}
}
```


### call host function from WASM module

```golang

func Test_hostFunc(t *testing.T) {
	buf, err := ioutil.ReadFile("wasm/host_func.wasm")
	require.NoError(t, err)

	mod, err := wasm.DecodeModule(bytes.NewBuffer(buf))
	require.NoError(t, err)

	var cnt uint64  // to be incremented as hostFunc is called

	// host functions must be defined in the form of `Virtual Machine closure` generator
	// in order to access VM state to get things done
	hostFunc := func(*wasm.VirtualMachine) reflect.Value {
		return reflect.ValueOf(func() {
			cnt++
		})
	}

	builder := hostfunc.NewModuleBuilderWith(wasi.Modules)
	builder.MustSetFunction("env", "host_func", hostFunc)
	vm, _ := wasm.NewVM(mod, builder.Done())

	for _, exp := range []uint64{5, 10, 15} {
		vm.ExecExportedFunction("call_host_func", exp)
		require.Equal(t, exp, cnt)
		cnt = 0
	}
}
```

## 🚧 WASI support 🚧

WebAssembly System Interface(WASI) will partly be supported in `wasi` package.
Currently only `fd_write` is implemented and you can see it works in `examples/panic` example
where the WASM module causes `panic` and the host prints the message through `fd_write` ABI. 

## references

https://webassembly.github.io/spec/core/index.html