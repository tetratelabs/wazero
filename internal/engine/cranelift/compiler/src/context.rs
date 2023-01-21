use crate::FuncRelocationEntry;
use cranelift_wasm::wasmparser::ValType;

#[derive(Copy, Clone, Eq, PartialEq, Hash, Debug)]
pub struct DefaultContext;
impl Context for DefaultContext {}

pub trait Context {
    fn compile_done(&self, code: &Vec<u8>, relocs: &Vec<FuncRelocationEntry>) {
        unsafe {
            compile_done(
                code.as_ptr() as *mut u8,
                code.len(),
                relocs.as_ptr() as *mut u8,
                relocs.len(),
            )
        }
    }

    fn type_counts(&self) -> u32 {
        unsafe { type_counts() }
    }

    fn type_lens(&self, tp: u32) -> (u32, u32) {
        let (mut params, mut returns): (u32, u32) = (0, 0);
        unsafe {
            type_lens(tp, &mut params as *mut u32, &mut returns as *mut u32);
        };
        (params, returns)
    }

    fn type_param_at(&self, tp: u32, at: u32) -> ValType {
        unsafe { std::mem::transmute(type_param_at(tp, at) as u8) }
    }

    fn type_result_at(&self, tp: u32, at: u32) -> ValType {
        unsafe { std::mem::transmute(type_result_at(tp, at) as u8) }
    }

    fn func_index(&self) -> u32 {
        unsafe { func_index() }
    }

    fn is_locally_defined_function(&self, idx: u32) -> bool {
        unsafe { is_locally_defined_function(idx) == 1 }
    }

    fn func_type_index(&self, at: u32) -> u32 {
        unsafe { func_type_index(at) }
    }

    fn current_func_type_index(&self) -> u32 {
        unsafe { current_func_type_index() }
    }

    fn memory_min_max(&self) -> (u32, u32) {
        let (mut min, mut max): (u32, u32) = (0, 0);
        unsafe {
            memory_min_max(&mut min as *mut u32, &mut max as *mut u32);
        }
        (min, max)
    }

    fn is_memory_imported(&self) -> bool {
        unsafe { is_memory_imported() == 1 }
    }

    fn memory_instance_base_offset(&self) -> i32 {
        unsafe { memory_instance_base_offset() }
    }

    fn vm_context_local_memory_offset(&self) -> i32 {
        unsafe { vm_context_local_memory_offset() }
    }

    fn vm_context_imported_memory_offset(&self) -> i32 {
        unsafe { vm_context_imported_memory_offset() }
    }

    fn vm_context_imported_function_offset(&self, idx: u32) -> i32 {
        unsafe { vm_context_imported_function_offset(idx) }
    }
}

macro_rules! define_wazero_import {
    ($name:ident, $param:tt) => {
        #[cfg(not(test))]
        #[link(wasm_import_module = "wazero")]
        extern "C" {
             #[link_name = stringify!($name)]
            pub fn $name $param;
        }
        #[cfg(test)]
        unsafe fn $name $param {
            panic!("wazero host calls must not be accessed in tests")
        }
    };
    ($name:ident, $param:tt, $result:tt) => {
        #[cfg(not(test))]
        #[link(wasm_import_module = "wazero")]
        extern "C" {
             #[link_name = stringify!($name)]
            fn $name $param -> $result;
        }
        #[cfg(test)]
        unsafe fn $name $param -> $result {
            panic!("wazero host calls must not be accessed in tests")
        }
    };
}

define_wazero_import!(
    compile_done,
    (
        code_ptr: *mut u8,
        code_size: usize,
        relocs_ptr: *mut u8,
        relocs_size: usize,
    )
);

define_wazero_import!(type_counts, (), u32);
define_wazero_import!(
    type_lens,
    (tp: u32, params_len_ptr: *mut u32, results_len_ptr: *mut u32)
);
define_wazero_import!(type_param_at, (tp: u32, at: u32), u32);
define_wazero_import!(type_result_at, (tp: u32, at: u32), u32);
define_wazero_import!(func_index, (), u32);
define_wazero_import!(is_locally_defined_function, (_idx: u32), u32);
define_wazero_import!(func_type_index, (_at: u32), u32);
define_wazero_import!(current_func_type_index, (), u32);
define_wazero_import!(memory_min_max, (min_ptr: *mut u32, max_ptr: *mut u32), u32);
define_wazero_import!(is_memory_imported, (), u32);
define_wazero_import!(memory_instance_base_offset, (), i32);
define_wazero_import!(vm_context_local_memory_offset, (), i32);
define_wazero_import!(vm_context_imported_function_offset, (_idx: u32), i32);
define_wazero_import!(vm_context_imported_memory_offset, (), i32);
