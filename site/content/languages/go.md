+++
title = "Go"
+++

## Introduction

When `GOARCH=wasm GOOS=js`, Go's compiler targets WebAssembly Binary format
(%.wasm).

Here's a typical compilation command:
```bash
$ GOOS=js GOARCH=wasm go build -o my.wasm .
```

The operating system is "js", but more specifically it is [wasm_exec.js][1].
This package runs the `%.wasm` just like `wasm_exec.js` would.

## Experimental

It is important to note that while there are some interesting features, such
as HTTP client support, the ABI (host functions imported by Wasm) used is
complicated and custom to Go. For this reason, there are few implementations
outside the web browser.

Moreover, Go defines js "EXPERIMENTAL... exempt from the Go compatibility
promise." While WebAssembly signatures haven't broken between 1.18 and 1.19,
then have in the past and can in the future.

For this reason implementations such as wazero's [gojs][14], cannot guarantee
portability from release to release, or that the code will work well in
production.

Due to lack of adoption, support and relatively high implementation overhead,
most choose [TinyGo]({{< relref "/tinygo.md" >}}) to compile source code, even if it supports less
features.

## WebAssembly Features

`GOARCH=wasm GOOS=js` uses instructions in [WebAssembly Core Specification 1.0]
[15] unless `GOWASM` includes features added afterwards.

Here are the valid [GOWASM values][16]:
* `satconv` - [Non-trapping Float-to-int Conversions][17]
* `signext` - [Sign-extension operators][18]

Note that both the above features are included [working draft][19] of
WebAssembly Core Specification 2.0.

## Constraints

Please read our overview of WebAssembly and
[constraints]({{< ref "_index.md#constraints" >}}). In short, expect
limitations in both language features and library choices when developing your
software.

`GOARCH=wasm GOOS=js` has a custom ABI which supports a subset of features in
the Go standard library. Notably, the host can implement time, crypto, file
system and HTTP client functions. Even where implemented, certain operations
will have no effect for reasons like ignoring HTTP request properties or fake
values returned (such as the pid). When not supported, many functions return
`syscall.ENOSYS` errors, or the string form: "not implemented on js".

Here are the more notable parts of Go which will not work when compiled via
`GOARCH=wasm GOOS=js`, resulting in `syscall.ENOSYS` errors:
* Raw network access. e.g. `net.Bind`
* File descriptor control (`fnctl`). e.g. `syscall.Pipe`
* Arbitrary syscalls. Ex `syscall.Syscall`
* Process control. e.g. `syscall.Kill`
* Kernel parameters. e.g. `syscall.Sysctl`
* Timezone-specific clock readings. e.g. `syscall.Gettimeofday`

## Memory

Memory layout begins with the "zero page" of size `runtime.minLegalPointer`
(4KB) which matches the `ssa.minZeroPage` in the compiler. It is then followed
by 8KB reserved for args and environment variables. This means the data section
begins at [ld.wasmMinDataAddr][12], offset 12288.

## System Calls

Please read our overview of WebAssembly and
[System Calls]({{< ref "_index.md#system-calls" >}}). In short, WebAssembly is
a stack-based virtual machine specification, so operates at a lower level than
an operating system.

"syscall/js.*" are host functions for features the operating system would
otherwise provide. These also manage the JavaScript object graph, including
functions to make and finalize objects, arrays and numbers (`js.Value`).

Each `js.Value` has a `js.ref`, which is either a numeric literal or an object
reference depending on its 64-bit bit pattern. When an object, the first 31
bits are its identifier.

There are several pre-defined values with constant `js.ref` patterns. These are
either constants, globals or otherwise needed in initializers.

For example, the "global" value includes properties like "fs" and "process"
which implement [system calls][7] needed for functions like `os.Getuid`.

Notably, not all system calls are implemented as some are stubbed by the
compiler to return zero values or `syscall.ENOSYS`. This means not all Go code
compiled to wasm will operate. For example, you cannot launch processes.

Details beyond this are best looking at the source code of [js.go][5], or its
unit tests.

## Concurrency

Please read our overview of WebAssembly and
[concurrency]({{< ref "_index.md#concurrency" >}}). In short, the current
WebAssembly specification does not support parallel processing.

Some internal code may seem strange knowing this. For example, Go's [function
wrapper][9] used for `GOOS=js` is implemented using locks. Seeing this, you may
feel the host side of this code (`_makeFuncWrapper`) should lock its ID
namespace for parallel use as well.

Digging deeper, you'll notice the [atomics][10] defined by `GOARCH=wasm` are
not actually implemented with locks, rather it is awaiting the ["Threads"
proposal][11].

In summary, while goroutines are supported in `GOARCH=wasm GOOS=js`, they won't
be able to run in parallel until the WebAssembly Specification includes atomics
and Go's compiler is updated to use them.

## Error handling

There are several `js.Value` used to implement `GOARCH=wasm GOOS=js` including
the global, file system, HTTP round tripping, processes, etc. All of these have
functions that may return an error on `js.Value.Call`.

However, `js.Value.Call` does not return an error result. Internally, this
dispatches to the wasm imported function `valueCall`, and interprets its two
results: the real result and a boolean, represented by an integer.

When false, `js.Value.Call` panics with a `js.Error` constructed from the first
result. This result must be an object with one of the below properties:

* JavaScript (GOOS=js): the string property "message" can be anything.
* Syscall error (GOARCH=wasm): the string property "code" is constrained.
  * The code must be like "EIO" in [errnoByCode][13] to avoid a panic.

Details beyond this are best looking at the source code of [js.go][5], or its
unit tests.

## Identifying wasm compiled by Go

If you have a `%.wasm` file compiled by Go (via [asm.go][2]), it has a custom
section named "go.buildid".

You can verify this with wasm-objdump, a part of [wabt][3]:
```
$ wasm-objdump --section=go.buildid -x my.wasm

example3.wasm:  file format wasm 0x1

Section Details:

Custom:
- name: "go.buildid"
```

## Module Exports

Until [wasmexport][4] is implemented, the [compiled][2] WebAssembly exports are
always the same:

* "mem" - (memory 265) 265 = data section plus 16MB
* "run" - (func (param $argc i32) (param $argv i32)) the entrypoint
* "resume" - (func) continues work after a timer delay
* "getsp" - (func (result i32)) returns the stack pointer

## Module Imports

Go's [compiles][3] all WebAssembly imports in the module "go", and only
functions are imported.

Except for the "debug" function, all function names are prefixed by their go
package. Here are the defaults:

* "debug" - is always function index zero, but it has unknown use.
* "runtime.*" - supports system-call like functionality `GOARCH=wasm`
* "syscall/js.*" - supports the JavaScript model `GOOS=js`

## PC_B calling conventions

The assembly `CallImport` instruction doesn't compile signatures to WebAssembly
function types, invoked by the `call` instruction.

Instead, the compiler generates the same signature for all functions: a single
parameter of the stack pointer, and invokes them via `call.indirect`.

Specifically, any function compiled with `CallImport` has the same function
type: `(func (param $sp i32))`. `$sp` is the base memory offset to read and
write parameters to the stack (at 8 byte strides even if the value is 32-bit).

So, implementors need to read the actual parameters from memory. Similarly, if
there are results, the implementation must write those to memory.

For example, `func walltime() (sec int64, nsec int32)` writes its results to
memory at offsets `sp+8` and `sp+16` respectively.

Note: WebAssembly compatible calling conventions has been discussed and
[attempted](https://go-review.googlesource.com/c/go/+/350737) in Go before.

## Go-defined exported functions

[Several functions][6] differ in calling convention by using WebAssembly type
signatures instead of the single SP parameter summarized above. Functions used
by the host have a "wasm_export_" prefix, which is stripped. For example,
"wasm_export_run" is exported as "run", defined in [rt0_js_wasm.s][7]

Here is an overview of the Go-defined exported functions:
 * "run" - Accepts "argc" and "argv" i32 params and begins the "wasm_pc_f_loop"
 * "resume" - Nullary function that resumes execution until it needs an event.
 * "getsp" - Returns the i32 stack pointer (SP)

## User-defined Host Functions

Users can define their own "go" module function imports by defining a func
without a body in their source and a `%_wasm.s` or `%_js.s` file that uses the
`CallImport` instruction.

For example, given `func logString(msg string)` and the below assembly:
```assembly
#include "textflag.h"

TEXT ·logString(SB), NOSPLIT, $0
CallImport
RET
```

If the package was `main`, the WebAssembly function name would be
"main.logString". If it was `util` and your `go.mod` module was
"github.com/user/me", the WebAssembly function name would be
"github.com/user/me/util.logString".

Regardless of whether the function import was built-in to Go, or defined by an
end user, all imports use `CallImport` conventions. Since these compile to a
signature unrelated to the source, more care is needed implementing the host
side, to ensure the proper count of parameters are read and results written to
the Go stack.

## Hacking

If you run into an issue where you need to change Go's sourcecode, the first
thing you should do is read the [contributing guide][20], which details how to
confirm an issue exists and a fix would be accepted. Assuming they say yes, the
next step is to ensure you can build and test go.

### Make a branch for your changes

First, clone upstream or your fork of golang/go and make a branch off `master`
for your work, as GitHub pull requests are against that branch.

```bash
$ git clone --depth=1 https://github.com/golang/go.git
$ cd go
$ git checkout -b my-fix
```

### Build a branch-specific `go` binary

While your change may not affect the go binary itself, there are checks inside
go that require version matching. Build a go binary from source to avoid these:

```bash
$ cd src
$ GOOS=js GOARCH=wasm ./make.bash
Building Go cmd/dist using /usr/local/go. (go1.19 darwin/amd64)
Building Go toolchain1 using /usr/local/go.
--snip--
$ cd ..
$ bin/go version
go version devel go1.19-c5da4fb7ac Fri Jul 22 20:12:19 2022 +0000 darwin/amd64
```

Tips:
* The above `bin/go` was built with whatever go version you had in your path!
* `GOARCH` here is what the resulting `go` binary can target. It isn't the
  architecture of the current host (`GOHOSTARCH`).

### Setup ENV variables for your branch.

To test the Go you just built, you need to have `GOROOT` set to your workspace,
and your PATH configured to find both `bin/go` and `misc/wasm/go_js_wasm_exec`.

```bash
$ export GOROOT=$PWD
$ export PATH=${GOROOT}/misc/wasm:${GOROOT}/bin:$PATH
```

Tip: `go_js_wasm_exec` is used because Go doesn't embed a WebAssembly runtime
like wazero. In other words, go can't run the wasm it just built. Instead,
`go test` uses Node.js which it assumes is installed on your host!

### Iterate until ready to submit

Now, you should be all set and can iterate similar to normal Go development.
The main thing to keep in mind is where files are, and remember to set
`GOOS=js GOARCH=wasm` when running go commands.

For example, if you fixed something in the `syscall/js` package
(`${GOROOT}/src/syscall/js`), test it like so:
```bash
$ GOOS=js GOARCH=wasm go test syscall/js
ok  	syscall/js	1.093s
```

[1]: https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js
[2]: https://github.com/golang/go/blob/go1.19/src/cmd/link/internal/wasm/asm.go
[3]: https://github.com/WebAssembly/wabt
[4]: https://github.com/golang/proposal/blob/master/design/42372-wasmexport.md
[5]: https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go
[6]: https://github.com/golang/go/blob/go1.19/src/cmd/internal/obj/wasm/wasmobj.go#L794-L812
[7]: https://github.com/golang/go/blob/go1.19/src/runtime/rt0_js_wasm.s#L17-L21
[8]: https://github.com/golang/go/blob/go1.19/src/syscall/syscall_js.go#L292-L306
[9]: https://github.com/golang/go/blob/go1.19/src/syscall/js/func.go#L41-L44
[10]: https://github.com/golang/go/blob/go1.19/src/runtime/internal/atomic/atomic_wasm.go#L5-L6
[11]: https://github.com/WebAssembly/proposals
[12]: https://github.com/golang/go/blob/go1.19/src/cmd/link/internal/ld/data.go#L2457
[13]: https://github.com/golang/go/blob/go1.19/src/syscall/tables_js.go#L371-L494
[14]: https://github.com/tetratelabs/wazero/tree/main/imports/go/example
[15]: https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/
[16]: https://github.com/golang/go/blob/go1.19/src/internal/buildcfg/cfg.go#L133-L147
[17]: https://github.com/WebAssembly/spec/blob/wg-2.0.draft1/proposals/nontrapping-float-to-int-conversion/Overview.md
[18]: https://github.com/WebAssembly/spec/blob/wg-2.0.draft1/proposals/sign-extension-ops/Overview.md
[19]: https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/
[20]: https://github.com/golang/go/blob/go1.19/CONTRIBUTING.md
