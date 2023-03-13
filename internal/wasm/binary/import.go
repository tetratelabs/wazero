package binary

import (
	"bytes"
	"fmt"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func decodeImport(
	r *bytes.Reader,
	idx uint32,
	memorySizer memorySizer,
	memoryLimitPages uint32,
	enabledFeatures api.CoreFeatures,
) (i wasm.Import, err error) {
	if i.Module, _, err = decodeUTF8(r, "import module"); err != nil {
		err = fmt.Errorf("import[%d] error decoding module: %w", idx, err)
		return
	}

	if i.Name, _, err = decodeUTF8(r, "import name"); err != nil {
		err = fmt.Errorf("import[%d] error decoding name: %w", idx, err)
		return
	}

	b, err := r.ReadByte()
	if err != nil {
		err = fmt.Errorf("import[%d] error decoding type: %w", idx, err)
		return
	}
	i.Type = b
	switch i.Type {
	case wasm.ExternTypeFunc:
		i.DescFunc, _, err = leb128.DecodeUint32(r)
	case wasm.ExternTypeTable:
		i.DescTable, err = decodeTable(r, enabledFeatures)
	case wasm.ExternTypeMemory:
		i.DescMem, err = decodeMemory(r, memorySizer, memoryLimitPages)
	case wasm.ExternTypeGlobal:
		i.DescGlobal, err = decodeGlobalType(r)
	default:
		err = fmt.Errorf("%w: invalid byte for importdesc: %#x", ErrInvalidByte, b)
	}
	if err != nil {
		err = fmt.Errorf("import[%d] %s[%s.%s]: %w", idx, wasm.ExternTypeName(i.Type), i.Module, i.Name, err)
	}
	return
}
