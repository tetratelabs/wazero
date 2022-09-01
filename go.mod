module github.com/tetratelabs/wazero

// This should be the minimum supported Go version per
// https://go.dev/doc/devel/release (1 version behind latest)
go 1.18

// Wrong naming convention
retract v1.0.0-beta1
// Has memory bug and quickly fixed in v1.0.0-beta.2.
retract v1.0.0-beta.1
