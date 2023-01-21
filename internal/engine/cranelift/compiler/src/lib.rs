// TODO: deletes below once matured.
#![allow(dead_code)]
#![allow(unused_variables)]

mod context;
mod func_env;
mod target;
mod validator;

use core::str::FromStr;
use cranelift_codegen::settings;
use cranelift_codegen::{ir, isa, Context};
use cranelift_wasm::wasmparser::ValType;
use std::mem::MaybeUninit;
use std::slice;

#[no_mangle]
extern "C" fn initialize_target(t: u32) {
    let target: target::WazeroTarget = unsafe { std::mem::transmute(t as u8) };
    target::initialize_target(target);
}

#[no_mangle]
extern "C" fn compile_function(body_ptr: *const u8, body_size: usize) {
    let body = unsafe { slice::from_raw_parts_mut(body_ptr as *mut u8, body_size as usize) };
    let context = context::DefaultContext {};
    compile_function_with_context(context, body);
}

#[no_mangle]
extern "C" fn _allocate(size: usize) -> *mut u8 {
    // Allocate the amount of bytes needed.
    let vec: Vec<MaybeUninit<u8>> = Vec::with_capacity(size);

    // into_raw leaks the memory to the caller.
    Box::into_raw(vec.into_boxed_slice()) as *mut u8
}

#[no_mangle]
extern "C" fn _deallocate(ptr: *mut u8, size: usize) {
    let _ = unsafe { Vec::from_raw_parts(ptr, 0, size) };
}

pub fn compile_function_with_context<T: context::Context + Copy>(ctx: T, wasm_body: &[u8]) {
    let isa = {
        let tuple =
            target_lexicon::Triple::from_str(target::arch()).expect("invalid triple literal");
        let isa_builder = isa::lookup(tuple).unwrap();
        let flag_builder = settings::builder();
        isa_builder
            .finish(settings::Flags::new(flag_builder))
            .unwrap()
    };

    let mut func_env = func_env::FuncEnvironment::new(&*isa, ctx);
    let mut validator = validator::new_validator(ctx);
    let mut func_translator = cranelift_wasm::FuncTranslator::new();
    let mut codegen_context = Context::new();
    codegen_context.func.signature = get_cranelift_signature(ctx, isa.pointer_type());

    // TODO: stack limit setup.
    let vmctx = codegen_context
        .func
        .create_global_value(ir::GlobalValueData::VMContext);
    func_env.vm_ctx = Some(vmctx);

    func_translator
        .translate_body(
            &mut validator,
            cranelift_wasm::wasmparser::FunctionBody::new(0, wasm_body),
            &mut codegen_context.func,
            &mut func_env,
        )
        .unwrap();

    let mut code_buf = Vec::new();
    let _ = codegen_context
        .compile_and_emit(&*isa, &mut code_buf)
        .unwrap();

    let compiled_code = codegen_context.compiled_code().unwrap();

    assert_eq!(
        compiled_code.alignment, 1,
        "Need to take into account the compiled code's alignment: {}",
        compiled_code.alignment
    );

    let relocs: Vec<FuncRelocationEntry> = compiled_code
        .buffer
        .relocs()
        .into_iter()
        .map(|item| mach_reloc_to_reloc(&codegen_context.func, item))
        .collect();

    ctx.compile_done(&code_buf, &relocs)
}

fn get_cranelift_signature<T: context::Context + Copy>(
    ctx: T,
    pointer_type: ir::Type,
) -> ir::Signature {
    let typ = ctx.current_func_type_index();
    get_cranelift_signature_at(ctx, pointer_type, typ)
}

fn get_cranelift_signature_at<T: context::Context + Copy>(
    ctx: T,
    pointer_type: ir::Type,
    typ: u32,
) -> ir::Signature {
    let mut sig = ir::Signature::new(target::calling_convention());

    // Add the callee/caller `vmctx` parameters.
    sig.params.push(ir::AbiParam::special(
        pointer_type,
        // By specifying ArgumentPurpose::VMContext here,
        // GlobalValue referenced by `ir::GlobalValueData::VMContext` is lowered to the actual address of *vmContext.
        // https://github.com/bytecodealliance/wasmtime/blob/v4.0.0/cranelift/codegen/src/legalizer/globalvalue.rs#L10-L24
        // https://github.com/bytecodealliance/wasmtime/blob/v4.0.0/cranelift/codegen/src/legalizer/globalvalue.rs#L54-L66
        ir::ArgumentPurpose::VMContext,
    ));
    sig.params.push(ir::AbiParam::new(pointer_type));

    let (params, returns) = ctx.type_lens(typ);
    let mut at: u32 = 0;
    while at < params {
        let p = ir::AbiParam::new(value_type(ctx.type_param_at(typ, at as u32)));
        sig.params.push(p);
        at += 1;
    }
    at = 0;
    while at < returns {
        let p = ir::AbiParam::new(value_type(ctx.type_result_at(typ, at as u32)));
        sig.returns.push(p);
        at += 1;
    }
    sig
}

#[repr(C)]
#[derive(Clone, Debug)]
pub struct FuncRelocationEntry {
    index: u32,
    offset: u32,
}

fn mach_reloc_to_reloc(
    func: &ir::Function,
    reloc: &cranelift_codegen::MachReloc,
) -> FuncRelocationEntry {
    let &cranelift_codegen::MachReloc {
        offset,
        kind,
        ref name,
        addend: _,
    } = reloc;

    assert_eq!(target::func_call_reloc_kind(), kind);
    let index = if let ir::ExternalName::User(user_func_ref) = *name {
        let ir::UserExternalName { namespace, index } =
            func.params.user_named_funcs()[user_func_ref];
        index
    } else {
        panic!("unsupported relocation {:?}", reloc)
    };
    FuncRelocationEntry { index, offset }
}

fn value_type(ty: ValType) -> ir::types::Type {
    match ty {
        ValType::I32 => ir::types::I32,
        ValType::I64 => ir::types::I64,
        ValType::F32 => ir::types::F32,
        ValType::F64 => ir::types::F64,
        ValType::V128 => ir::types::I8X16,
        ValType::FuncRef => unreachable!(),
        ValType::ExternRef => unreachable!(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn value_type_conversion() {
        let result = 2 + 2;
        assert_eq!(value_type(ValType::I32), ir::types::I32);
        assert_eq!(value_type(ValType::I64), ir::types::I64);
        assert_eq!(value_type(ValType::F32), ir::types::F32);
        assert_eq!(value_type(ValType::F64), ir::types::F64);
        assert_eq!(value_type(ValType::V128), ir::types::I8X16);
    }
}
