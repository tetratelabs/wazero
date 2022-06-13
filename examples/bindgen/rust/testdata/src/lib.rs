use std::mem;
use bindgen_macro::wazero_bindgen;

/// Greets the name of whoever was passed in
#[wazero_bindgen]
pub fn greet(name: String) -> String {
    format!("Hello {}", name)
}

/// Tries to greet but fails every time
#[wazero_bindgen]
pub fn greet_err(_: String) -> Result<(), String> {
	Err(String::from("oops, there was an error"))
}

/// Returns the greeting as a tuple
#[wazero_bindgen]
pub fn greet_tuple(name: String) -> (String, String) {
	("Hello".to_string(), name)
}

/// Returns the greeting as a vector of bytes
#[wazero_bindgen]
pub fn greet_vec(name: String) -> Vec<u8> {
	format!("Hello {}", name).as_bytes().to_vec()
}

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
