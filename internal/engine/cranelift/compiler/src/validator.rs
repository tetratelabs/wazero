// TODO: deletes below once matured.
#![allow(dead_code)]
#![allow(unused_variables)]

use cranelift_wasm::wasmparser::{
    self, FuncValidator, GlobalType, MemoryType, TableType, ValType, WasmFuncType,
    WasmModuleResources,
};

pub struct ValidatorResources {
    types: Vec<FuncTypeImpl>,
}

impl ValidatorResources {
    // TODO: this should be created once per module.
    fn new() -> Self {
        let type_counts = unsafe { crate::type_counts() };
        let mut tps = Vec::with_capacity(type_counts as usize);
        let mut type_index = 0;
        while type_index < type_counts {
            let (mut params_len, mut results_len): (u32, u32) = (0, 0);
            unsafe {
                crate::type_lens(
                    type_index,
                    &mut params_len as *mut u32,
                    &mut results_len as *mut u32,
                )
            }

            tps.push(FuncTypeImpl {
                type_index,
                params_len,
                results_len,
            });
            type_index += 1;
        }
        Self { types: tps }
    }
}

pub fn new_validator() -> FuncValidator<impl WasmModuleResources> {
    let (function_index, type_index) =
        unsafe { (crate::func_index(), crate::current_func_type_index()) };
    wasmparser::FuncToValidate::new(
        function_index,
        type_index,
        ValidatorResources::new(),
        &cranelift_wasm::wasmparser::WasmFeatures::default(),
    )
    .into_validator(Default::default())
}

pub struct FuncTypeImpl {
    type_index: u32,
    params_len: u32,
    results_len: u32,
}

impl WasmFuncType for FuncTypeImpl {
    fn len_inputs(&self) -> usize {
        self.params_len as usize
    }

    fn len_outputs(&self) -> usize {
        self.results_len as usize
    }
    fn input_at(&self, at: u32) -> Option<ValType> {
        Some(crate::type_param_at(self.type_index, at))
    }
    fn output_at(&self, at: u32) -> Option<ValType> {
        Some(crate::type_result_at(self.type_index, at))
    }
}

impl WasmModuleResources for ValidatorResources {
    type FuncType = FuncTypeImpl;

    fn table_at(&self, _at: u32) -> Option<TableType> {
        todo!()
    }

    fn memory_at(&self, _at: u32) -> Option<MemoryType> {
        let (mut min, mut max): (u32, u32) = (0, 0);
        let res = unsafe { crate::memory_min_max(&mut min as *mut u32, &mut max as *mut u32) };
        assert_eq!(res, 1);
        Some(MemoryType {
            memory64: false,
            shared: false,
            // Note: How does this affect the compilation?
            initial: min as u64,
            maximum: Some(max as u64),
        })
    }

    fn tag_at(&self, _at: u32) -> Option<&Self::FuncType> {
        todo!()
    }

    fn global_at(&self, _at: u32) -> Option<GlobalType> {
        todo!()
    }

    fn func_type_at(&self, type_idx: u32) -> Option<&Self::FuncType> {
        Some(
            self.types
                .get(type_idx as usize)
                .expect(format!("{}", type_idx).as_str()),
        )
    }

    fn type_of_function(&self, func_idx: u32) -> Option<&Self::FuncType> {
        let tp = unsafe { crate::func_type_index(func_idx) };
        Some(self.types.get(tp as usize).unwrap())
    }

    fn element_type_at(&self, _at: u32) -> Option<ValType> {
        todo!()
    }

    fn element_count(&self) -> u32 {
        todo!()
    }

    fn data_count(&self) -> Option<u32> {
        todo!()
    }

    fn is_function_referenced(&self, _idx: u32) -> bool {
        todo!()
    }
}
