// Package sysfs includes a low-level filesystem interface and utilities needed
// for WebAssembly host functions (ABI) such as WASI.
//
// The name sysfs was chosen because wazero's public API has a "sys" package,
// which was named after https://github.com/golang/sys.
package sysfs

// Single-word zero for use when we need a valid pointer to 0 bytes.
var zero uintptr
