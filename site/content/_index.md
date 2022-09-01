+++
title = "wazero: the zero dependency WebAssembly runtime for Go developers"
layout = "single"
+++

WebAssembly is a way to safely run code compiled in other languages. Runtimes
execute WebAssembly Modules (Wasm), which are most often binaries with a
`.wasm` extension.

wazero is the only zero dependency WebAssembly runtime written in Go.

## Example

The best way to learn wazero is by trying one of our [examples][1]. The
most [basic example][2] extends a Go application with an addition function
defined in WebAssembly.

## Why zero?

By avoiding CGO, wazero avoids prerequisites such as shared libraries or libc,
and lets you keep features like cross compilation. Being pure Go, wazero adds
only a small amount of size to your binary. Meanwhile, wazero’s API gives
features you expect in Go, such as safe concurrency and context propagation.

### When can I use this?

wazero is an early project, so APIs are subject to change until version 1.0.
To use wazero meanwhile, you need to add its main branch to your project like
this:

```bash
go get github.com/tetratelabs/wazero@main
```

wazero will release its first beta at the end of August 2022, and finalize
1.0 once Go 1.20 is released in Feb 2023. Meanwhile, please practice the
current APIs to ensure they work for you, and give us a [star][3] if you are
enjoying it so far!

[1]: https://github.com/tetratelabs/wazero/blob/main/examples
[2]: https://github.com/tetratelabs/wazero/blob/main/examples/basic
[3]: https://github.com/tetratelabs/wazero/stargazers
