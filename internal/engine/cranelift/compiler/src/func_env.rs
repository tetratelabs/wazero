// TODO: deletes below once matured.
#![allow(dead_code)]
#![allow(unused_variables)]

use cranelift_codegen::cursor::FuncCursor;
use cranelift_codegen::ir;
use cranelift_codegen::ir::condcodes::IntCC;
use cranelift_codegen::ir::{
    FuncRef, Function, Heap, HeapStyle, Inst, InstBuilder, SigRef, Signature, Table, Value,
};
use cranelift_codegen::isa::TargetFrontendConfig;
use cranelift_codegen::isa::TargetIsa;
use cranelift_wasm::wasmparser;

use cranelift_wasm::{
    FuncIndex, FuncTranslationState, FunctionBuilder, GlobalIndex, GlobalVariable, MemoryIndex,
    TableIndex, TargetEnvironment, TypeIndex, WasmResult, WasmType,
};

pub struct FuncEnvironment<'module_environment> {
    isa: &'module_environment (dyn TargetIsa + 'module_environment),
    pub vm_ctx: Option<ir::GlobalValue>,
}

impl<'module_environment> FuncEnvironment<'module_environment> {
    pub fn new(isa: &'module_environment (dyn TargetIsa + 'module_environment)) -> Self {
        FuncEnvironment { isa, vm_ctx: None }
    }
}

impl<'module_environment> TargetEnvironment for FuncEnvironment<'module_environment> {
    fn target_config(&self) -> TargetFrontendConfig {
        self.isa.frontend_config()
    }
}

impl<'module_environment> cranelift_wasm::FuncEnvironment for FuncEnvironment<'module_environment> {
    fn is_wasm_parameter(&self, _signature: &Signature, index: usize) -> bool {
        index >= 2 // First two params are callee/scaller vmContexts.
    }

    fn is_wasm_return(&self, _signature: &Signature, _index: usize) -> bool {
        true
    }

    fn after_locals(&mut self, _num_locals_defined: usize) {}

    fn make_global(
        &mut self,
        _func: &mut Function,
        _index: GlobalIndex,
    ) -> WasmResult<GlobalVariable> {
        todo!()
    }

    fn make_heap(&mut self, func: &mut Function, _index: MemoryIndex) -> WasmResult<Heap> {
        let vmctx = self.vm_ctx.unwrap();
        let pointer_type = self.isa.pointer_type();

        let is_memory_imported = crate::is_memory_imported();
        if !is_memory_imported {
            // This makes all the access to this variable re-load the base address
            // from the vmctx. That is necessary considering that memory buffer can grow.
            let read_only = false;

            let heap_base =
                func.create_global_value(ir::GlobalValueData::Load {
                    base: vmctx,
                    offset: ir::immediates::Offset32::new(
                        crate::vmctx::vm_context_memory_base_offset(),
                    ),
                    global_type: pointer_type,
                    readonly: read_only,
                });

            let heap_bound =
                func.create_global_value(ir::GlobalValueData::Load {
                    base: vmctx,
                    offset: ir::immediates::Offset32::new(
                        crate::vmctx::vm_context_memory_base_offset(),
                    ),
                    global_type: pointer_type,
                    readonly: read_only,
                });

            Ok(func.create_heap(ir::HeapData {
                base: heap_base,
                min_size: 0.into(),
                // https://github.com/bytecodealliance/wasmtime/blob/v4.0.0/crates/wasmtime/src/config.rs#L1164-L1191
                // offset_guard_size: ir::immediates::Uimm64::new(0x1_0000),
                // This seems not used for dynamic memory?
                offset_guard_size: ir::immediates::Uimm64::new(0),
                style: HeapStyle::Dynamic {
                    bound_gv: heap_bound,
                },
                // We don't support 64-bit Wasm.
                index_type: ir::types::I32,
            }))
        } else {
            todo!("imported memory access")
        }
    }

    fn make_table(&mut self, _func: &mut Function, _index: TableIndex) -> WasmResult<Table> {
        todo!()
    }

    fn make_indirect_sig(&mut self, _func: &mut Function, _index: TypeIndex) -> WasmResult<SigRef> {
        todo!()
    }

    fn make_direct_func(&mut self, func: &mut Function, index: FuncIndex) -> WasmResult<FuncRef> {
        let index_u32 = index.as_u32();
        let sig = unsafe {
            let typ = crate::func_type_index(index_u32);
            crate::get_cranelift_signature_at(self.isa.pointer_type(), typ)
        };
        let signature = func.import_signature(sig);
        let name =
            ir::ExternalName::User(func.declare_imported_user_function(ir::UserExternalName {
                namespace: 0,
                index: index.as_u32(),
            }));
        Ok(func.import_function(ir::ExtFuncData {
            name,
            signature,
            // See https://github.com/bytecodealliance/wasmtime/blob/v4.0.0/crates/cranelift/src/func_environ.rs#L1518-L1531
            colocated: unsafe { crate::is_locally_defined_function(index_u32) },
        }))
    }

    fn translate_call_indirect(
        &mut self,
        _builder: &mut FunctionBuilder,
        _table_index: TableIndex,
        _table: Table,
        _sig_index: TypeIndex,
        _sig_ref: SigRef,
        _callee: Value,
        _call_args: &[Value],
    ) -> WasmResult<Inst> {
        todo!()
    }

    fn translate_call(
        &mut self,
        mut pos: FuncCursor,
        callee_index: FuncIndex,
        callee: FuncRef,
        call_args: &[Value],
    ) -> WasmResult<Inst> {
        // Original Wasm params + callee/caller vmCtx.
        let mut args = Vec::with_capacity(call_args.len() + 2);

        // Get the caller vmctx.
        let caller_vmctx = pos
            .func
            .special_param(ir::ArgumentPurpose::VMContext)
            .unwrap();

        let local_fn = unsafe { crate::is_locally_defined_function(callee_index.as_u32()) };
        if local_fn {
            // callee/caller vmCtx.
            // Note that if this is calling a local function, the vmCtx are the same.
            args.push(caller_vmctx);
            args.push(caller_vmctx);
            // Then Wasm params follow.
            args.extend_from_slice(call_args);
            return Ok(pos.ins().call(callee, &args));
        }
        todo!("calling imported functions.")
    }

    fn translate_memory_grow(
        &mut self,
        _pos: FuncCursor,
        _index: MemoryIndex,
        _heap: Heap,
        _val: Value,
    ) -> WasmResult<Value> {
        todo!()
    }

    fn translate_memory_size(
        &mut self,
        _pos: FuncCursor,
        _index: MemoryIndex,
        _heap: Heap,
    ) -> WasmResult<Value> {
        todo!()
    }

    fn translate_memory_copy(
        &mut self,
        _pos: FuncCursor,
        _src_index: MemoryIndex,
        _src_heap: Heap,
        _dst_index: MemoryIndex,
        _dst_heap: Heap,
        _dst: Value,
        _src: Value,
        _len: Value,
    ) -> WasmResult<()> {
        todo!()
    }

    fn translate_memory_fill(
        &mut self,
        _pos: FuncCursor,
        _index: MemoryIndex,
        _heap: Heap,
        _dst: Value,
        _val: Value,
        _len: Value,
    ) -> WasmResult<()> {
        todo!()
    }

    fn translate_memory_init(
        &mut self,
        _pos: FuncCursor,
        _index: MemoryIndex,
        _heap: Heap,
        _seg_index: u32,
        _dst: Value,
        _src: Value,
        _len: Value,
    ) -> WasmResult<()> {
        todo!()
    }

    fn translate_data_drop(&mut self, _pos: FuncCursor, _seg_index: u32) -> WasmResult<()> {
        todo!()
    }

    fn translate_table_size(
        &mut self,
        _pos: FuncCursor,
        _index: TableIndex,
        _table: Table,
    ) -> WasmResult<Value> {
        todo!()
    }

    fn translate_table_grow(
        &mut self,
        _pos: FuncCursor,
        _table_index: TableIndex,
        _table: Table,
        _delta: Value,
        _init_value: Value,
    ) -> WasmResult<Value> {
        todo!()
    }

    fn translate_table_get(
        &mut self,
        _builder: &mut FunctionBuilder,
        _table_index: TableIndex,
        _table: Table,
        _index: Value,
    ) -> WasmResult<Value> {
        todo!()
    }

    fn translate_table_set(
        &mut self,
        _builder: &mut FunctionBuilder,
        _table_index: TableIndex,
        _table: Table,
        _value: Value,
        _index: Value,
    ) -> WasmResult<()> {
        todo!()
    }

    fn translate_table_copy(
        &mut self,
        _pos: FuncCursor,
        _dst_table_index: TableIndex,
        _dst_table: Table,
        _src_table_index: TableIndex,
        _src_table: Table,
        _dst: Value,
        _src: Value,
        _len: Value,
    ) -> WasmResult<()> {
        todo!()
    }

    fn translate_table_fill(
        &mut self,
        _pos: FuncCursor,
        _table_index: TableIndex,
        _dst: Value,
        _val: Value,
        _len: Value,
    ) -> WasmResult<()> {
        todo!()
    }

    fn translate_table_init(
        &mut self,
        _pos: FuncCursor,
        _seg_index: u32,
        _table_index: TableIndex,
        _table: Table,
        _dst: Value,
        _src: Value,
        _len: Value,
    ) -> WasmResult<()> {
        todo!()
    }

    fn translate_elem_drop(&mut self, _pos: FuncCursor, _seg_index: u32) -> WasmResult<()> {
        todo!()
    }

    fn translate_ref_null(&mut self, _pos: FuncCursor, _ty: WasmType) -> WasmResult<Value> {
        todo!()
    }

    fn translate_ref_is_null(&mut self, _pos: FuncCursor, _value: Value) -> WasmResult<Value> {
        todo!()
    }

    fn translate_ref_func(
        &mut self,
        _pos: FuncCursor,
        _func_index: FuncIndex,
    ) -> WasmResult<Value> {
        todo!()
    }

    fn translate_custom_global_get(
        &mut self,
        _pos: FuncCursor,
        _global_index: GlobalIndex,
    ) -> WasmResult<Value> {
        todo!()
    }

    fn translate_custom_global_set(
        &mut self,
        _pos: FuncCursor,
        _global_index: GlobalIndex,
        _val: Value,
    ) -> WasmResult<()> {
        todo!()
    }

    fn translate_atomic_wait(
        &mut self,
        _pos: FuncCursor,
        _index: MemoryIndex,
        _heap: Heap,
        _addr: Value,
        _expected: Value,
        _timeout: Value,
    ) -> WasmResult<Value> {
        todo!()
    }

    fn translate_atomic_notify(
        &mut self,
        _pos: FuncCursor,
        _index: MemoryIndex,
        _heap: Heap,
        _addr: Value,
        _count: Value,
    ) -> WasmResult<Value> {
        todo!()
    }

    fn translate_loop_header(&mut self, _builder: &mut FunctionBuilder) -> WasmResult<()> {
        todo!()
    }

    fn before_translate_operator(
        &mut self,
        _op: &wasmparser::Operator,
        _builder: &mut FunctionBuilder,
        _state: &FuncTranslationState,
    ) -> WasmResult<()> {
        Ok(())
    }

    fn after_translate_operator(
        &mut self,
        _op: &wasmparser::Operator,
        _builder: &mut FunctionBuilder,
        _state: &FuncTranslationState,
    ) -> WasmResult<()> {
        Ok(())
    }

    fn before_translate_function(
        &mut self,
        _builder: &mut FunctionBuilder,
        _state: &FuncTranslationState,
    ) -> WasmResult<()> {
        Ok(())
    }

    fn after_translate_function(
        &mut self,
        _builder: &mut FunctionBuilder,
        _state: &FuncTranslationState,
    ) -> WasmResult<()> {
        Ok(())
    }

    fn unsigned_add_overflow_condition(&self) -> IntCC {
        todo!()
    }
}
