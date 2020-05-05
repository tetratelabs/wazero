# gasm

A minimal implementation of v1 WASM spec compatible virtual machine purely written in go.
The vm can be embedded in your go program without any dependency like cgo, and enables Gophers to 
write wasm host environments easily.

The vm should be used only for providing _sandbox_ environments embedded in your Go program since
 we have not implemented [validation](https://webassembly.github.io/spec/core/valid/index.html) of wasm binary.

The implementation is quite straightforward and I hope this code would be a
 good starting point for novices to learn WASM spec.

## examples

### call expoerted function from host

```golang

func main() {
	buf, _ := ioutil.ReadFile("./fib.wasm")
	mod, _ := wasm.DecodeModule(bytes.NewBuffer(buf))
	mod.BuildIndexSpaces(wasi.Modules) // fd_write must be injected
	vm, _ := wasm.NewVM(mod)

	for _, c := range []struct {
		in, exp int32
	}{
		{in: 20, exp: 6765},
		{in: 10, exp: 55},
		{in: 5, exp: 5},
	} {
		ret, _, _ := vm.ExecExportedFunction("fib", uint64(c.in))
		if int32(ret[0]) != c.exp {
			panic(fmt.Sprintf("result must be %d but got %d\n", c.exp, ret[0]))
		}
	}
}
```


### call host function from WASM module

```golang
func main() {
	buf, _ := ioutil.ReadFile("./host_func.wasm")
	mod, _ := wasm.DecodeModule(bytes.NewBuffer(buf))

	var cnt uint64
	hostFunc := func(*wasm.VirtualMachine) reflect.Value {
		return reflect.ValueOf(func() {
			cnt++
		})
	}

	builder := hostmodule.NewBuilderWith(wasi.Modules)
	builder.MustAddFunction("env", "host_func", hostFunc)
	mod.BuildIndexSpaces(builder.Done())
	vm, _ := wasm.NewVM(mod)

	for _, exp := range []uint64{5, 10, 15} {
		vm.ExecExportedFunction("call_host_func", exp)
		if cnt != exp {
			panic(fmt.Sprintf("want %d but got %d", exp, cnt))
		}
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