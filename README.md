# Gasm

A minimal Wasm runtime purely written in Go. The runtime passes all the [Wasm Spec test suites](https://github.com/WebAssembly/spec/tree/wg-1.0/test/core) and is fully compatible with the WebAssembly v1.0 Specification.

This library can be embedded in your Go program without any dependency like cgo, and enables Gophers to write Wasm host environments easily.

## Example

```golang
func Test_fibonacci(t *testing.T) {
	binary, _ := os.ReadFile("wasm/fibonacci.wasm")
	mod, _ := wasm.DecodeModule(binary)
	store := wasm.NewStore(naivevm.NewEngine())
	store.Instantiate(mod, "test")

	for _, c := range []struct {
		in, exp int32
	}{
		{in: 20, exp: 6765},
		{in: 10, exp: 55},
		{in: 5, exp: 5},
	} {
		ret, retTypes, err := store.CallFunction("test", "fibonacci", uint64(c.in))
		require.NoError(t, err)
		require.Len(t, ret, len(retTypes))
		require.Equal(t, wasm.ValueTypeI32, retTypes[0])
		require.Equal(t, c.exp, int32(ret[0]))
	}
}

```

## Performance

Any performance optimization hasn't been done yet, and the runtime is just a simple interpreter of Wasm binary.
We are planning to add a single path compilation and lowering of pure Wasm binary into more efficient and powerful format in the near future.

## References

- https://webassembly.github.io/spec/core/index.html
