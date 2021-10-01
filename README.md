# Gasm

A minimal implementation of Wasm Virtual machine purely written in Go.
The VM can be embedded in your Go program without any dependency like cgo, and enables Gophers to write Wasm host environments easily.

The implementation is quite straightforward and I hope this code would be a
 good starting point for novices to learn Wasm spec.

## Examples

Full examples can be found at: https://github.com/mathetake/gasm/tree/master/examples

### Call exported function from host

```golang
func Test_fibonacci(t *testing.T) {
	buf, _ := ioutil.ReadFile("wasm/fibonacci.wasm")
	mod, _ := wasm.DecodeModule(bytes.NewBuffer(buf))
	vm, _ := wasm.NewVM(mod, wasi.New().Modules())

	for _, c := range []struct {
		in, exp int32
	}{
		{in: 20, exp: 6765},
		{in: 10, exp: 55},
		{in: 5, exp: 5},
	} {
		ret, _, _ := vm.ExecExportedFunction("fibonacci", uint64(c.in))
		require.Equal(t, c.exp, int32(ret[0]))
	}
}
```


### Call host function from Wasm module

```golang

func Test_hostFunc(t *testing.T) {
	buf, _ := ioutil.ReadFile("wasm/host_func.wasm")
	mod, _ := wasm.DecodeModule(bytes.NewBuffer(buf))

	var cnt uint64  // to be incremented as hostFunc is called

	// host functions must be defined in the form of `Virtual Machine closure` generators
	// in order to access the VM state to get things done
	hostFunc := func(*wasm.VirtualMachine) reflect.Value {
		return reflect.ValueOf(func() {
			cnt++
		})
	}

	builder := hostfunc.NewModuleBuilderWith(wasi.New().Modules())
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

WebAssembly System Interface (WASI) is partly supported in `wasi` package.
Currently these methods are implemented:
- `fd_write`
- `fd_prestat_get`
- `fd_prestat_dir_name`
- `path_open`
- `fd_read`
- `fd_close`

By default, WASI uses the host process's Stdin, Stdout and Stderr and doesn't
preopen any directories, but that can be changed with functional options.

```go
vm, err := wasm.NewVM(mod, wasi.New(
	wasi.Stdin(myReader),
	wasi.Stdout(myWriter),
	wasi.Stderr(myErrWriter),
	wasi.Preopen(".", wasi.DirFS(".")),
).Modules())
if err != nil {
	panic(err)
}

if err := vm.ExecExportedFunction("_start"); err != nil {
	panic(err)
}
```

If you want to provide an in-memory file system to the wasm binary, you can
do so with `wasi.MemFS()`.

## References

- https://webassembly.github.io/spec/core/index.html
- https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md
