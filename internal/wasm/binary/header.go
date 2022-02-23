package binary

// Magic is the 4 byte preamble (literally "\0asm") of the binary format
// See https://www.w3.org/TR/wasm-core-1/#binary-magic
var Magic = []byte{0x00, 0x61, 0x73, 0x6D}

// version is format version and doesn't change between known specification versions
// See https://www.w3.org/TR/wasm-core-1/#binary-version
var version = []byte{0x01, 0x00, 0x00, 0x00}
