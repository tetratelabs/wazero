// TODO: deletes below once matured.
#![allow(dead_code)]
#![allow(unused_variables)]

use crate::context::Context;
use cranelift_wasm::wasmparser::{
    self, FuncValidator, GlobalType, MemoryType, TableType, ValType, WasmFuncType,
    WasmModuleResources,
};

pub struct ValidatorResources<T: Context + Copy> {
    types: Vec<FuncType>,
    ctx: T,
}

impl<T: Context + Copy> ValidatorResources<T> {
    // TODO: this should be created once per module.
    fn new(ctx: T) -> Self {
        let type_counts = ctx.type_counts();
        let mut tps = Vec::with_capacity(type_counts as usize);
        let mut type_index = 0;
        while type_index < type_counts {
            let (params_len, results_len) = ctx.type_lens(type_index);

            let mut params: Vec<ValType> = Vec::with_capacity(params_len as usize);
            let mut i = 0;
            while i < params_len {
                params.push(ctx.type_param_at(type_index, i));
                i += 1;
            }
            i = 0;
            let mut results: Vec<ValType> = Vec::with_capacity(results_len as usize);
            while i < results_len {
                results.push(ctx.type_result_at(type_index, i));
                i += 1;
            }

            tps.push(FuncType {
                type_index,
                params,
                results,
            });
            type_index += 1;
        }
        Self {
            types: tps,
            ctx: ctx,
        }
    }
}

pub fn new_validator<T: Context + Copy>(ctx: T) -> FuncValidator<impl WasmModuleResources> {
    let func_index = ctx.func_index();
    let current_func_type_index = ctx.current_func_type_index();
    wasmparser::FuncToValidate::new(
        func_index,
        current_func_type_index,
        ValidatorResources::new(ctx),
        &cranelift_wasm::wasmparser::WasmFeatures::default(),
    )
    .into_validator(Default::default())
}

pub struct FuncType {
    type_index: u32,
    params: Vec<ValType>,
    results: Vec<ValType>,
}

impl WasmFuncType for FuncType {
    fn len_inputs(&self) -> usize {
        self.params.len()
    }

    fn len_outputs(&self) -> usize {
        self.results.len()
    }

    fn input_at(&self, at: u32) -> Option<ValType> {
        self.params.get(at as usize).copied()
    }
    fn output_at(&self, at: u32) -> Option<ValType> {
        self.results.get(at as usize).copied()
    }
}

impl<T: Context + Copy> WasmModuleResources for ValidatorResources<T> {
    type FuncType = FuncType;

    fn table_at(&self, _at: u32) -> Option<TableType> {
        todo!()
    }

    fn memory_at(&self, _at: u32) -> Option<MemoryType> {
        let (min, max) = self.ctx.memory_min_max();
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
        let tp = self.ctx.func_type_index(func_idx);
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
