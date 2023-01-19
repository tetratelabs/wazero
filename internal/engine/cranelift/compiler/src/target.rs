static mut ARCH: &str = "";
static mut CALLING_CONVENTION: cranelift_codegen::isa::CallConv =
    cranelift_codegen::isa::CallConv::SystemV;
static mut FUNC_CALL_RELOC_KIND: cranelift_codegen::binemit::Reloc =
    cranelift_codegen::binemit::Reloc::X86SecRel;

#[derive(Debug)]
pub enum WazeroTarget {
    /// Arm64Darwin corresponds to GOARCH=arm64 GOOS=darwin
    Arm64Darwin,
    /// Arm64Linux corresponds to GOARCH=arm64 GOOS=linux
    Arm64Linux,
    /// Amd64Darwin corresponds to GOARCH=arm64 GOOS=darwin
    Amd64Darwin,
    /// Amd64Linux corresponds to GOARCH=arm64 GOOS=linux
    Amd64Linux,
}

pub fn initialize_target(t: WazeroTarget) {
    match t {
        WazeroTarget::Arm64Darwin => {
            unsafe {
                ARCH = "aarch64";
                CALLING_CONVENTION = cranelift_codegen::isa::CallConv::WasmtimeAppleAarch64;
                // https://github.com/bytecodealliance/wasmtime/blob/v4.0.0/cranelift/codegen/src/isa/aarch64/abi.rs#L984-L994
                // https://github.com/bytecodealliance/wasmtime/blob/v4.0.0/cranelift/codegen/src/isa/aarch64/inst/emit.rs#L3057-L3066
                FUNC_CALL_RELOC_KIND = cranelift_codegen::binemit::Reloc::Arm64Call;
            };
        }
        WazeroTarget::Arm64Linux => todo!(),
        WazeroTarget::Amd64Linux => todo!(),
        WazeroTarget::Amd64Darwin => todo!(),
    }
}

pub fn arch() -> &'static str {
    unsafe { ARCH }
}

pub fn calling_convention() -> cranelift_codegen::isa::CallConv {
    unsafe { CALLING_CONVENTION }
}

pub fn func_call_reloc_kind() -> cranelift_codegen::binemit::Reloc {
    unsafe { FUNC_CALL_RELOC_KIND }
}
