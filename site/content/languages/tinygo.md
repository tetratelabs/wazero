+++
title = "TinyGo"
+++

## Introduction

[TinyGo][1] is an alternative compiler for Go source code. It can generate
`%.wasm` files instead of architecture-specific binaries through two targets:

* `wasm`: for browser (JavaScript) use.
* `wasi`: for use outside the browser.

This document is maintained by wazero, which is a WebAssembly runtime that
embeds in Golang applications. Hence, all notes below will be about TinyGo's
`wasi` target.

When TinyGo compiles a `%.go` file with its `wasi` target, the output `%.wasm`
depends on a subset of features in the [WebAssembly 2.0 Core specification][2],
as well [WASI][3] host imports.

Unlike some compilers, TinyGo also supports importing custom host functions and
exporting functions back to the host.

## Example

Here's a basic example of source in TinyGo:

```go
package main

//export add
func add(x, y uint32) uint32 {
	return x + y
}
```

The following flags will result in the most compact (smallest) wasm file.
```bash
tinygo build -o main.wasm -target=wasi main.go
```

The resulting wasm exports the `add` function so that the embedding host can
call it, regardless of if the host is written in Go or not.

## Disclaimer

This document includes notes contributed by the wazero community. While wazero
includes TinyGo examples, and maintainers often contribute to TinyGo, this
isn't a TinyGo official document. For more help, consider the [TinyGo Using
WebAssembly Guide][4] or joining the [#TinyGo channel on the Gophers Slack][5].

Meanwhile, please help us [maintain][6] this document and [star our GitHub
repository][7], if it is helpful. Together, we can make WebAssembly easier on
the next person.

## Constraints

Like other compilers that can target wasm, there are constraints using TinyGo.
These constraints affect the library design and dependency choices in your Go
source.

### Partial Reflection Support
The first constraint people notice is that `encoding/json` usage compiles, but
panics at runtime.
```go
package main

import "encoding/json"

type response struct {
	Ok bool `json:"ok"`
}

func main() {
	var res response
	if err := json.Unmarshal([]byte(`{"ok": true}`), &res); err != nil {
		println(err)
	}
}
```
This is due to limited support for reflection, and effects other [serialization
tools][18] also. See [Frequently Asked Questions](#frequently-asked-questions)
for some workarounds.

### Unimplemented System Calls
You may also notice some other features not yet work. For example, the below
will compile, but print "readdir unimplemented : errno 54" at runtime.

```go
package main

import "os"

func main() {
	if _, err := os.ReadDir("."); err != nil {
		println(err)
	}
}
```

The underlying error is often, but not always `syscall.ENOSYS` which is the
standard way to stub a syscall until it is implemented. If you are interested
in more, see [System Calls](#system-calls).

### Mitigating Constraints
Realities like this are not unique to TinyGo as they will happen compiling any
language not written specifically with WebAssembly in mind. Knowing the same
code compiled to wasm may return errors or worse panic, the main mitigation
approach is testing.

Unit test the critical paths of your code, including errors, on your target
WebAssembly runtime, such as wazero. This not only gives higher confidence, but
is also a much more efficient means to communicate bugs vs ad-hoc reports.

## Memory

When TinyGo compiles go into wasm, it configures the WebAssembly linear memory
to an initial size of 2 pages (16KB), and marks a position in that memory as
the heap base. All memory beyond that is used for the Go heap, which can
[grow][20] until `memory.grow` on the host returns -1.

Allocations within Go (compiled to `%.wasm`) are managed as one would expect.
Sometimes a host function needs to allocate memory directly. For example, to
write JSON of a given length before invoking an exported function to parse it.

### Host Allocations

The below snippet is a realistic example of a function exported to the host,
who needs to allocate memory first.
```go
//export configure
func configure(ptr uintptr, size uint32) {
	json := ptrToString(ptr, size)
}
```
Note: WebAssembly uses 32-bit memory addressing, so a `uintptr` is 32-bits.

The general flow is that the host allocates memory by calling an allocation
function with the size needed. Then, it writes data, in this case JSON, to the
memory offset (`ptr`). At that point, it can call a host function, ex
`configure`, passing the `ptr` and `size` allocated. The guest wasm (compiled
from Go) will be able to read the data. To ensure no memory leaks, the host
calls a free function, with the same `ptr`, afterwards and unconditionally.

Note: wazero includes an [example project][8] that shows this.

There are two ways to implement this pattern, and they affect how to implement
the `ptrToString` function above:
* Built-in `malloc` and `free` functions
* Custom `malloc` and `free` functions

While both patterns are used in practice, TinyGo maintainers only support the
custom approach. See the following issues for clarifications:
* [WebAssembly exports for allocation][9]
* [Memory ownership of TinyGo allocated pointers][10]

#### Built-in `malloc` and `free` functions

The least code way to allow the host to allocate memory is to call the built-in
`malloc` and `free` functions exported by TinyGo:
```webassembly
(func (export "malloc") (param $size i32) (result (;$ptr;) i32))
(func (export "free") (param $ptr i32))
```

Go code (compiled to %.wasm) can read this memory directly by first coercing it
to a `reflect.SliceHeader`.
```go
func ptrToString(ptr uintptr, size uint32) string {
	return *(*string)(unsafe.Pointer(&reflect.SliceHeader{
		Data: ptr,
		Len:  uintptr(size),
		Cap:  uintptr(size),
	}))
}
```

The reason TinyGo maintainers do not recommend this approach is there's a risk
of garbage collection interference, albeit unlikely in practice.

#### Custom `malloc` and `free` functions

The safest way to allow the host to allocate memory is to define your own
`malloc` and `free` functions with names that don't collide with TinyGo's:
```webassembly
(func (export "my_malloc") (param $size i32) (result (;$ptr;) i32))
(func (export "my_free") (param $ptr i32))
```

The below implements the custom approach, in Go using a map of byte slices.
```go
func ptrToString(ptr uintptr, size uint32) string {
	// size is ignored as the underlying map is pre-allocated.
	return string(alivePointers[ptr])
}

var alivePointers = map[uintptr][]byte{}

//export my_malloc
func my_malloc(size uint32) uintptr {
	buf := make([]byte, size)
	ptr := &buf[0]
	unsafePtr := uintptr(unsafe.Pointer(ptr))
	alivePointers[unsafePtr] = buf
	return unsafePtr
}

//export my_free
func my_free(ptr uintptr) {
	delete(alivePointers, ptr)
}
```

Note: Even if you define your own functions, you should still keep the same
signatures as the built-in. For example, a `size` parameter on `ptrToString`,
even if you don't use it. This gives you more flexibility to change the
approach later.

## System Calls

WebAssembly is a stack-based virtual machine specification, so operates at a
lower level than an operating system. For functionality the operating system
would otherwise provide, TinyGo imports host functions, specifically ones
defined in [WASI][3], described in [Specifications]({{< ref "/specs" >}}).

Notably, if you compile and run below program with the target `wasi`, you'll
see that the effective `GOARCH=wasm` and `GOOS=linux`.

```go
package main

import (
	"fmt"
	"runtime"
)

func main() {
	fmt.Println(runtime.GOARCH, runtime.GOOS)
}
```

### WASI Internals

While developing WASI in TinyGo is outside the scope of this document, the
below pointers will help you understand the underlying architecture of the
`wasi` target. Ideally, these notes can help you frame support or feature
requests with the TinyGo team.

A close look at the [wasi target][11] reveals how things work. Underneath,
TinyGo leverages the `wasm32-unknown-wasi` LLVM target for the system call
layer (libc), which is eventually implemented by the [wasi-libc][12] library.

Similar to normal code, TinyGo decides which abstraction to use with GOOS and
GOARCH specific suffixes and build flags.

For example, `os.Args` is implemented directly using WebAssembly host functions
in [runtime_wasm_wasi.go][13]. `syscall.Chdir` is implemented with the same
[syscall_libc.go][14] used for other architectures, while `syscall.ReadDirent`
is stubbed (returns `syscall.ENOSYS`), in [syscall_libc_wasi.go][15].

## Concurrency

Current versions of the WebAssembly specification do not support parallelism,
such as threads or atomics needed to safely work in parallel.

TinyGo, however, supports goroutines by default and acts like `GOMAXPROCS=1`.

For example, the following code will run with the expected output, even if
the goroutines are defined in opposite dependency order.
```go
package main

import "fmt"

func main() {
	msg := make(chan int)
	finished := make(chan int)
	go func() {
		<-msg
		fmt.Println("consumer")
		finished <- 1
	}()
	go func() {
		fmt.Println("producer")
		msg <- 1
	}()
	<-finished
}
```

However, creating goroutines after main (`_start` in WASI) has undefined
behavior. For example, if that same function was exported (`//export:notMain`),
and called after main, the line that creates a goroutine panics at runtime.

Given problems like this, some choose a compile-time failure instead, via
`-scheduler=none`. Since code often needs to be custom in order to work with
wasm anyway, there may be limited impact to removing goroutine support.

## Optimizations

Below are some commonly used configurations that allow optimizing for size or
performance vs defaults. Note that sometimes one sacrifices the other.

### Binary size

Those with size constraints can reduce the `%.wasm` binary size by changing
`tinygo` flags. For example, a simple `cat` program can reduce from default of
260KB to 60KB using both flags below.

* `-scheduler=none`: Reduces size, but fails at compile time on goroutines.
* `--no-debug`: Strips DWARF, but retains the WebAssembly name section.

### Performance

* `-gc=leaking`: Avoids GC which improves performance for short-lived programs.
* `-opt=2`: Can also improve performance.

## Frequently Asked Questions

### How do I use json?
TinyGo doesn't yet implement [reflection APIs][16] needed by `encoding/json`.
Meanwhile, most users resort to non-reflective parsers, such as [gjson][17].

### Why does my wasm import WASI functions even when I don't use it?
TinyGo has a `wasm` target (for browsers) and a `wasi` target for runtimes that
support [WASI][3]. This document is written only about the `wasi` target.

Some users are surprised to see imports from WASI (`wasi_snapshot_preview1`),
when their neither has a main function nor uses memory. At least implementing
`panic` requires writing to the console, and `fd_write` is used for this.

A bare or standalone WebAssembly target doesn't yet exist, but if interested,
you can follow [this issue][19].

### Why is my wasm so big?
TinyGo defaults can be overridden for those who can sacrifice features or
performance for a [smaller binary](#binary-size). After that, tuning your
source code may reduce binary size further.

TinyGo minimally needs to implement garbage collection and `panic`, and the
wasm to implement that is often not considered big (~4KB). What's often
surprising to users are APIs that seem simple, but require a lot of supporting
functions, such as `fmt.Println`, which can require 100KB of wasm.

[1]: https://tinygo.org/
[2]: https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/
[3]: https://github.com/WebAssembly/WASI
[4]: https://tinygo.org/docs/guides/webassembly/
[5]: https://github.com/tinygo-org/tinygo#getting-help
[6]: https://github.com/tetratelabs/wazero/tree/main/site/content/languages/tinygo.md
[7]: https://github.com/tetratelabs/wazero/stargazers
[8]: https://github.com/tetratelabs/wazero/tree/main/examples/allocation/tinygo
[9]: https://github.com/tinygo-org/tinygo/issues/2788
[10]: https://github.com/tinygo-org/tinygo/issues/2787
[11]: https://github.com/tinygo-org/tinygo/blob/v0.25.0/targets/wasi.json
[12]: https://github.com/WebAssembly/wasi-libc
[13]: https://github.com/tinygo-org/tinygo/blob/v0.25.0/src/runtime/runtime_wasm_wasi.go#L34-L62
[14]: https://github.com/tinygo-org/tinygo/blob/v0.25.0/src/syscall/syscall_libc.go#L85-L92
[15]: https://github.com/tinygo-org/tinygo/blob/v0.25.0/src/syscall/syscall_libc_wasi.go#L263-L265
[16]: https://github.com/tinygo-org/tinygo/issues/2660
[17]: https://github.com/tidwall/gjson
[18]: https://github.com/tinygo-org/tinygo/issues/447
[19]: https://github.com/tinygo-org/tinygo/issues/3068
[20]: https://github.com/tinygo-org/tinygo/blob/v0.25.0/src/runtime/arch_tinygowasm.go#L47-L62
