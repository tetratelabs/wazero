## Zig allocation example

This example shows how to pass strings in and out of a Wasm function defined in
Zig, built with `zig build`.

```bash
$ go run greet.go wazero
wasm >> Hello, wazero!
go >> Hello, wazero!
```

[greet.zig](testdata/greet.zig) does a few things of interest:
* Uses `@ptrToInt` to change a Zig pointer to a numeric type
* Uses `[*]u8` as an argument to take a pointer and slices it to build back a
  string

The Zig code exports "malloc" and "free", which we use for that purpose.

### Notes

This example uses `@panic()` rather than `unreachable` to handle errors
since `unreachable` emits a call to panic only in `Debug` and `ReleaseSafe`
mode. In `ReleaseFast` and `ReleaseSmall` mode, it would lead into undefined
behavior.
