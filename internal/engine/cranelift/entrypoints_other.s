//go:build !amd64 && !arm64

#include "funcdata.h"
#include "textflag.h"

#define FUNCTION_ATTRIBUTES NOSPLIT|NOFRAME, $0-48

TEXT ·entryPointNoParamNoResult(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO

TEXT ·entryPointNoParamI32Result(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO

TEXT ·entryPointNoParamI64Result(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO

TEXT ·entryPointNoParamF32Result(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO

TEXT ·entryPointNoParamF64Result(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO

TEXT ·entryPointNoParamI32PlusMultiResult(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO

TEXT ·entryPointNoParamI64PlusMultiResult(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO

TEXT ·entryPointNoParamF32PlusMultiResult(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO

TEXT ·entryPointNoParamF64PlusMultiResult(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO

TEXT ·entryPointWithParamNoResult(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO

TEXT ·entryPointWithParamI32Result(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO

TEXT ·entryPointWithParamI64Result(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO

TEXT ·entryPointWithParamF32Result(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO

TEXT ·entryPointWithParamF64Result(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO

TEXT ·entryPointWithParamI32PlusMultiResult(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO

TEXT ·entryPointWithParamI64PlusMultiResult(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO

TEXT ·entryPointWithParamF32PlusMultiResult(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO

TEXT ·entryPointWithParamF64PlusMultiResult(SB), FUNCTION_ATTRIBUTES
	UNDEF // TODO
