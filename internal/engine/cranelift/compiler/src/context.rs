use crate::FuncRelocationEntry;
use cranelift_wasm::wasmparser::ValType;

#[derive(Copy, Clone, Eq, PartialEq, Hash, Debug)]
pub struct DefaultContext {}

impl<'a> Context for &'a DefaultContext {}

pub trait Context {
    fn compile_done(&self, code: &Vec<u8>, relocs: &Vec<FuncRelocationEntry>) {
        unsafe {
            host_calls::compile_done(
                code.as_ptr() as *mut u8,
                code.len(),
                relocs.as_ptr() as *mut u8,
                relocs.len(),
            )
        }
    }
    fn type_counts(self) -> u32 {
        unsafe { host_calls::type_counts() }
    }
    fn type_lens(&self, tp: u32, params_len_ptr: *mut u32, results_len_ptr: *mut u32) {
        unsafe { host_calls::type_lens(tp, params_len_ptr, results_len_ptr) }
    }
    fn type_param_at(&self, tp: u32, at: u32) -> ValType {
        unsafe { std::mem::transmute(host_calls::_type_param_at(tp, at) as u8) }
    }
    fn type_result_at(&self, tp: u32, at: u32) -> ValType {
        unsafe { std::mem::transmute(host_calls::_type_result_at(tp, at) as u8) }
    }
    fn func_index(self) -> u32 {
        unsafe { host_calls::func_index() }
    }
    fn is_locally_defined_function(&self, idx: u32) -> bool {
        unsafe { host_calls::_is_locally_defined_function(idx) == 1 }
    }
    fn func_type_index(&self, at: u32) -> u32 {
        unsafe { host_calls::func_type_index(at) }
    }
    fn current_func_type_index(self) -> u32 {
        unsafe { host_calls::current_func_type_index() }
    }
    fn memory_min_max(&self, min_ptr: *mut u32, max_ptr: *mut u32) -> u32 {
        unsafe { host_calls::memory_min_max(min_ptr, max_ptr) }
    }
    fn is_memory_imported(self) -> bool {
        unsafe { host_calls::_is_memory_imported() == 1 }
    }
    fn memory_instance_base_offset(self) -> i32 {
        unsafe { host_calls::memory_instance_base_offset() }
    }
    fn vm_context_local_memory_offset(self) -> i32 {
        unsafe { host_calls::vm_context_local_memory_offset() }
    }
    fn vm_context_imported_memory_offset(self) -> i32 {
        unsafe { host_calls::vm_context_imported_memory_offset() }
    }
    fn vm_context_imported_function_offset(&self, idx: u32) -> i32 {
        unsafe { host_calls::vm_context_imported_function_offset(idx) }
    }
}

mod host_calls {
    #[cfg(not(test))]
    #[link(wasm_import_module = "wazero")]
    extern "C" {
        #[link_name = "compile_done"]
        pub fn compile_done(
            code_ptr: *mut u8,
            code_size: usize,
            relocs_ptr: *mut u8,
            relocs_size: usize,
        );
        #[link_name = "type_counts"]
        pub fn type_counts() -> u32;
        #[link_name = "type_lens"]
        pub fn type_lens(_tp: u32, params_len_ptr: *mut u32, results_len_ptr: *mut u32);
        #[link_name = "type_param_at"]
        pub fn _type_param_at(_tp: u32, _at: u32) -> u32;
        #[link_name = "type_result_at"]
        pub fn _type_result_at(_tp: u32, _at: u32) -> u32;
        #[link_name = "func_index"]
        pub fn func_index() -> u32;
        #[link_name = "is_locally_defined_function"]
        pub fn _is_locally_defined_function(_idx: u32) -> u32;
        #[link_name = "func_type_index"]
        pub fn func_type_index(_at: u32) -> u32;
        #[link_name = "current_func_type_index"]
        pub fn current_func_type_index() -> u32;
        #[link_name = "memory_min_max"]
        pub fn memory_min_max(min_ptr: *mut u32, max_ptr: *mut u32) -> u32;
        #[link_name = "is_memory_imported"]
        pub fn _is_memory_imported() -> u32;
        #[link_name = "memory_instance_base_offset"]
        pub fn memory_instance_base_offset() -> i32;
        #[link_name = "vm_context_local_memory_offset"]
        pub fn vm_context_local_memory_offset() -> i32;
        #[link_name = "vm_context_imported_memory_offset"]
        pub fn vm_context_imported_memory_offset() -> i32;
        #[link_name = "vm_context_imported_function_offset"]
        pub fn vm_context_imported_function_offset(_idx: u32) -> i32;
    }
}
