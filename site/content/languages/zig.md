+++
title = "Zig"
+++

## Introduction

Beginning with 0.4.0 [Zig][1] can generate `%.wasm` files instead of
architecture-specific binaries through three targets:

* `wasm32-emscripten`: mostly for browser (JavaScript) use.
* `wasm32-freestanding`: for standalone use in or outside the browser.
* `wasm32-wasi`: for use outside the browser.

This document is maintained by wazero, which is a WebAssembly runtime that
embeds in Go applications. Hence, our notes focus on targets used outside the
browser, tested by wazero: `wasm32-freestanding` and `wasm32-wasi`.

## Overview

When Zig compiles a `%.zig` file with a `wasm32-*` target, the output `%.wasm`
depends on a subset of features in the [WebAssembly 2.0 
Core specification]({{< ref "/specs#core" >}}) and [WASI]({{< ref "/specs#wasi" >}}) host
functions.

Unlike some compilers, Zig also supports importing custom host functions and
exporting functions back to the host.

Here's a basic example of source in Zig:

```zig
export fn add(a: i32, b: i32) i32 {
    return a + b;
}
```

The following is the minimal command to build a Wasm file.

```bash
zig build-lib -dynamic -target wasm32-freestanding main.zig
```

The resulting Wasm `export`s the `add` function so that the embedding host can
call it, regardless of if the host is written in Zig or not.

Notice we are using `zig build-lib -dynamic`: this
compiles the source as a library, i.e. without a `main` function.

## Disclaimer

This document includes notes contributed by the wazero community for Zig 0.10.1. 
While wazero includes Zig examples, and maintainers contribute to Zig, this
isn't a Zig official document. For more help, consider the [WebAssembly Documentation][4] 
or joining the [#programming-discussion channel on 
Zig's Discord][5]. 

Meanwhile, please help us [maintain][6] this document and [star our GitHub
repository][7], if it is helpful. Together, we can make WebAssembly easier on
the next person.

## Constraints

Please read our overview of WebAssembly and
[constraints]({{< ref "_index.md#constraints" >}}). In short, expect
limitations in both language features and library choices when developing your
software.

## Memory

The Zig language performs no memory management on behalf of the programmer. 
However, Zig has no default allocator. Instead, functions which need to allocate 
accept an `Allocator` parameter.

### Host Allocations

Sometimes a host function needs to allocate memory directly. For example, to write JSON
of a given length before invoking an exported function to parse it.

```zig
pub export fn configure(ptr: [*]const u8, size: u32) void {
    _configure(message[0..size]) catch |err| @panic(switch (err) {
        error.OutOfMemory => "out of memory",
    });
}
```

The general flow is that the host allocates memory by calling an allocation
function with the size needed. Then, it writes data, in this case JSON, to the
memory offset (`ptr`). At that point, it can call a host function, ex
`configure`, passing the `ptr` and `size` allocated. The guest Wasm (compiled
from Zig) will be able to read the data. To ensure no memory leaks, the host
calls a free function, with the same `ptr`, afterwards and unconditionally.

Note: wazero includes an [example project][9] that shows this.

The [zig example][9] does a few things of interest:
* Uses `@ptrToInt` to change a Zig pointer to a numeric type
* Uses `[*]u8` as an argument to take a pointer and slices it to build back a
string
* It also shows how to import a host function using the `extern` directive

To allow the host to allocate memory, you need to define your own `malloc` and
`free` functions:
```webassembly
(func (export "malloc") (param $size i32) (result (;$ptr;) i32))
(func (export "free") (param $ptr i32) (param $size i32))
```

Because Zig easily allows end-users to [plug their own allocators][12], it relatively easy to 
export custom `malloc`/`free` pairs to the host.

For instance, the following code exports `malloc`, `free` from Zig's `page_allocator`:

```zig
const allocator = std.heap.page_allocator;

pub export fn malloc(length: usize) ?[*]u8 {
    const buff = allocator.alloc(u8, length) catch return null;
    return buff.ptr;
}

pub export fn free(buf: [*]u8, length: usize) void {
    allocator.free(buf[0..length]);
}
```

## System Calls

Please read our overview of WebAssembly and
[System Calls]({{< ref "_index.md#system-calls" >}}). In short, WebAssembly is
a stack-based virtual machine specification, so operates at a lower level than
an operating system.

For functionality the operating system would otherwise provide, you must use
the `wasm32-wasi` target. This imports host functions in
[WASI]({{< ref "/specs#wasi" >}}).

Zig's standard library support for WASI is under active development. 
In general, you should favor use of the standard library when compiling against 
wasm32-wasi target (e.g. `std.io`).

Note: wazero includes an [example WASI project][10] including [source code][11]
that implements `cat` without any WebAssembly-specific code.

## Concurrency

Please read our overview of WebAssembly and
[concurrency]({{< ref "_index.md#concurrency" >}}). In short, the current
WebAssembly specification does not support parallel processing.

## Optimizations

Below are some commonly used configurations that allow optimizing for size or
performance vs defaults. Note that sometimes one sacrifices the other.

### Binary size

Those with `%.wasm` binary size constraints can change their source, 
e.g. picking a [different allocator][9b] or set `zig` flags to reduce it.

[`zig` flags][13]:
Zig provides several flags to control binary size, speed of execution, 
safety checks. For instance you may use
* `-ODebug`: Fast build, enabled safety checks, slower runtime performance, 
  larger binary size
* `-OReleaseSafe`: Medium runtime performance, enabled safety checks, 
  slower compilation speed, larger binary size
* `-OReleaseSmall`: Medium runtime performance, disabled safety checks, 
  slower compilation speed, smaller binary size

### Performance

Those with runtime performance constraints can change their source or set
`zig` flags to improve it.

[`zig` flags][13]:
* `-OReleaseFast`: Enable additional optimizations, possibly at the cost of 
  increased binary size.

## Frequently Asked Questions

### Why is my `%.wasm` binary so big?
Zig defaults can be overridden for those who can sacrifice features or
performance for a [smaller binary](#binary-size). After that, tuning your
source code may reduce binary size further.

[1]: https://ziglang.org/download/0.4.0/release-notes.html
[2]: https://ziglang.org/documentation/0.10.1/#WASI
[4]: https://ziglang.org/documentation/0.10.1/#WebAssembly
[5]: https://discord.gg/gxsFFjE
[6]: https://github.com/tetratelabs/wazero/tree/main/site/content/languages/zig.md
[7]: https://github.com/tetratelabs/wazero/stargazers
[9]: https://github.com/tetratelabs/wazero/tree/main/examples/allocation/zig
[9b]: https://ziglang.org/documentation/0.10.1/#Memory
[10]: https://github.com/tetratelabs/wazero/tree/main/imports/wasi_snapshot_preview1/example/testdata/zig
[11]: https://github.com/tetratelabs/wazero/blob/main/imports/wasi_snapshot_preview1/example/testdata/zig/cat.zig
[13]: https://ziglang.org/documentation/0.10.1/#Build-Mode
