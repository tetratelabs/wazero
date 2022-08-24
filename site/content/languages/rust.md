+++
title = "Rust"
+++

## Introduction

Beginning with 1.30 [Rust][1] can generate `%.wasm` files instead of
architecture-specific binaries through three targets:

* `wasm32-unknown-emscripten`: mostly for browser (JavaScript) use.
* `wasm32-unknown-unknown`: for standalone use in or outside the browser.
* `wasm32-wasi`: for use outside the browser.

This document is maintained by wazero, which is a WebAssembly runtime that
embeds in Go applications. Hence, our notes focus on targets used outside the
browser, tested by wazero: `wasm32-unknown-unknown` and `wasm32-wasi`.

This document also focuses on `rustc` as opposed to `cargo`, for precision and
brevity.

## Overview

When Rust compiles a `%.rs` file with a `wasm32-*` target, the output `%.wasm`
depends on a subset of features in the [WebAssembly 1.0 Core specification][2].
The `wasm32-wasi` target depends on [WASI][3] host imports as well.

Unlike some compilers, Rust also supports importing custom host functions and
exporting functions back to the host.

Here's a basic example of source in Rust:

```rust
#[no_mangle]
pub extern "C" fn add(x: i32, y: i32) -> i32 {
    x + y
}
```

The following is the minimal command to build a wasm file.
```bash
rustc -o lib.wasm --target wasm32-unknown-unknown --crate-type cdylib lib.rs
```

The resulting wasm exports the `add` function so that the embedding host can
call it, regardless of if the host is written in Rust or not.

### Digging Deeper

There are a few things to unpack above, so let's start at the top.

The rust source includes `#[no_mangle]` and `extern "C"`. Add these to
functions you want to export to the WebAssembly host. You can read more about
this in [The Embedded Rust Book][4].

Next, you'll notice the flag `--crate-type cdylib` passed to `rustc`. This
compiles the source as a library, ex. without a `main` function.

## Disclaimer

This document includes notes contributed by the wazero community. While wazero
includes Rust examples, the community is less familiar with Rust. For more
help, consider the [Rust and WebAssembly book][5]

Meanwhile, please help us [maintain][6] this document and [star our GitHub
repository][9], if it is helpful. Together, we can make WebAssembly easier on
the next person.

## Memory

When Rust compiles rust into wasm, it configures the WebAssembly linear memory
to an initial size of 17 pages (1.1MB), and marks a position in that memory as
the heap base. All memory beyond that is used for the Rust heap.

Allocations within Rust (compiled to `%.wasm`) are managed as one would expect.
The `global_allocator` can allocate pages (`alloc_pages`) until `memory.grow`
on the host returns -1.

### Host Allocations

Sometimes a host function needs to allocate memory directly. For example, to
write JSON of a given length before invoking an exported function to parse it.

The below snippet is a realistic example of a function exported to the host,
who needs to allocate memory first.
```rust
#[no_mangle]
pub unsafe extern "C" fn configure(ptr: u32, len: u32) {
  let json = &ptr_to_string(ptr, len);
}
```
Note: WebAssembly uses 32-bit memory addressing, so a `uintptr` is 32-bits.

The general flow is that the host allocates memory by calling an allocation
function with the size needed. Then, it writes data, in this case JSON, to the
memory offset (`ptr`). At that point, it can call a host function, ex
`configure`, passing the `ptr` and `size` allocated. The guest wasm (compiled
from Rust) will be able to read the data. To ensure no memory leaks, the host
calls a free function, with the same `ptr`, afterwards and unconditionally.

Note: wazero includes an [example project][8] that shows this.

To allow the host to allocate memory, you need to define your own `malloc` and
`free` functions:
```webassembly
(func (export "malloc") (param $size i32) (result (;$ptr;) i32))
(func (export "free") (param $ptr i32) (param $size i32))
```

The below implements this in Rustlang:
```rust
use std::mem::MaybeUninit;
use std::slice;

unsafe fn ptr_to_string(ptr: u32, len: u32) -> String {
    let slice = slice::from_raw_parts_mut(ptr as *mut u8, len as usize);
    let utf8 = std::str::from_utf8_unchecked_mut(slice);
    return String::from(utf8);
}

#[no_mangle]
pub extern "C" fn alloc(size: u32) -> *mut u8 {
    // Allocate the amount of bytes needed.
    let vec: Vec<MaybeUninit<u8>> = Vec::with_capacity(size as usize);

    // into_raw leaks the memory to the caller.
    Box::into_raw(vec.into_boxed_slice()) as *mut u8
}

#[no_mangle]
pub unsafe extern "C" fn free(ptr: u32, size: u32) {
  // Retake the pointer which allows its memory to be freed.
  let _ = Vec::from_raw_parts(ptr as *mut u8, 0, size as usize);
}
```

As you can see, the above code is quite technical and should be kept separate
from your business logic as much as possible.

## System Calls

WebAssembly is a stack-based virtual machine specification, so operates at a
lower level than an operating system. For functionality the operating system
would otherwise provide, you must use the `wasm32-wasi` target. This imports
host defined in [WASI][2], described in [Specifications]({{< ref "/specs" >}}).

For example, `rustc -o hello.wasm --target wasm32-wasi hello.rs` compiles the
below into wasm that exports a `_start` WASI function corresponding to `main`.
```rust
fn main() {
  println!("Hello World!");
}
```

Note: wazero includes an [example WASI project][XXXX] including [source code][XXXX]
that implements `cat` without any WebAssembly-specific code.

## Optimizations

Below are some commonly used configurations that allow optimizing for size or
performance vs defaults. Note that sometimes one sacrifices the other.

### Binary size

Those with size constraints can reduce the `%.wasm` binary size by changing
the source or flags to `rustc`.

Source changes:
* [wee_alloc][10]: Smaller, WebAssembly-tuned memory allocator.

[`rustc` flags][11]:
* `-C debuginfo=0`: Strips DWARF, but retains the WebAssembly name section.
* `-C opt-level=3`: includes all optimizations.

Those using cargo should also use the `--release` flag, which corresponds to
`rustc -C debuginfo=0 -C opt-level=3`.

Those using the `wasm32-wasi` target should consider the [cargo-wasi][12] crate
as it dramatically reduces wasm size.

## Frequently Asked Questions

### Why is my wasm so big?
Rust defaults can be overridden for those who can sacrifice features or
performance for a [smaller binary](#binary-size). After that, tuning your
source code may reduce binary size further.

[1]: https://www.rust-lang.org/tools/install
[2]: https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/
[3]: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
[4]: https://docs.rust-embedded.org/book/interoperability/rust-with-c.html#no_mangle
[5]: https://rustwasm.github.io/docs/book
[6]: https://github.com/tetratelabs/wazero/tree/main/site/content/languages/rust.md
[7]: https://github.com/tetratelabs/wazero/stargazers
[8]: https://github.com/tetratelabs/wazero/tree/main/examples/allocation/rust
[9]: https://github.com/tetratelabs/wazero/tree/main/examples/wasi/testdata/cargo-wasi
[10]: https://github.com/rustwasm/wee_alloc
[11]: https://doc.rust-lang.org/cargo/reference/profiles.html#profile-settings
[12]: https://github.com/bytecodealliance/cargo-wasi
