static mut MEMORY_BASE_OFFSET: i32 = 0;
static mut MEMORY_LENGTH_OFFSET: i32 = 0;

pub fn vm_context_memory_base_offset() -> i32 {
    unsafe { MEMORY_BASE_OFFSET }
}

pub fn vm_context_memory_length_offset() -> i32 {
    unsafe { MEMORY_LENGTH_OFFSET }
}

pub fn initialize_vm_context_offsets() {
    unsafe {
        MEMORY_BASE_OFFSET = crate::vm_context_memory_base_offset();
        MEMORY_LENGTH_OFFSET = crate::vm_context_memory_length_offset();
    }
}
