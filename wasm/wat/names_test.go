package wat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeNameSection(t *testing.T) {
	m, err := parseModule(simpleExample)
	require.NoError(t, err)

	require.Equal(t, []byte{
		0x0, /* module subsection ID zero */
		0x8, /* 8 bytes to follow */
		0x7, /* the module name $simple is 7 characters long */
		'$', 's', 'i', 'm', 'p', 'l', 'e',
		0x1, /* function subsection ID one */
		0x9, /* 9 bytes to follow */
		0x1, /* one function name */
		0x0, /* the function index is zero */
		0x6, /* the function name $hello is 6 characters long */
		'$', 'h', 'e', 'l', 'l', 'o',
	}, encodeNameSection(m))
}

func TestEncodeNameSubsection(t *testing.T) {
	subsectionID := uint8(1)
	name := "$simple"
	require.Equal(t, append([]byte{
		subsectionID,
		byte(1 + len(name)), // 1 is the size of len(name) in LEB128 encoding
		byte(len(name))}, []byte(name)...), encodeNameSubsection(subsectionID, encodeName(name)))
}

func TestEncodeNameMapEntry(t *testing.T) {
	index := uint32(1)
	name := "$hello"
	require.Equal(t, append([]byte{
		byte(index),
		byte(len(name))}, []byte(name)...), encodeNameMapEntry(index, name))
}

func TestEncodeName(t *testing.T) {
	// We expect a length (in LEB128) prefixed string encoding
	require.Equal(t, []byte{6, '$', 'h', 'e', 'l', 'l', 'o'}, encodeName("$hello"))
}
