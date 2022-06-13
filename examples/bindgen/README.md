# Bindgen
This is currently a proof-of-concept example illustrating how we can use a bindgen wrapper to call Rust guest functions and pass arbitrary data types such as strings. It also provides utilities for handling errors using the Rust `Result` type.

## Example
To run the example in this directory, compile the Rust code with

    go generate ./...

Then run the Go host with

    go run main.go

        Hello Wazero
        oops, there was an error


## How To Implement
If you're implementing this in your own project, you need to do a couple of things.

### Rust (Guest)
The Rust (guest) side needs to provide allocation and a de-allocation functions to the host.
```rust
#[no_mangle]
pub unsafe extern fn allocate(size: i32) -> *const u8 {
	let mut buffer = Vec::with_capacity(size as usize);
	let pointer = buffer.as_mut_ptr();
	mem::forget(buffer);
	pointer as *const u8
}

#[no_mangle]
pub unsafe extern fn deallocate(pointer: *mut u8, size: i32) {
	drop(Vec::from_raw_parts(pointer, size as usize, size as usize));
}

```

Then you can create your own functions and decorate them with the `#[wazer_bindgen]` macro.
```rust
#[wazero_bindgen]
pub fn greet(name: String) -> Result<(), String> {
    Ok(format!("Hello {}", name))
}
```

### Go (Host)
The host needs to perform several steps. See ([main.go](./rust/main.go) for a full example):
1. Create a runtime.
2. Instantiate the bindgen module in the runtime.
3. Instantiate the custom WASM module in the runtime.
4. Bind the bindgen instance to the custom wasm module.
5. Use bindgen to execute exported guest functions.
