module github.com/tetratelabs/wazero

// This should be the minimum supported Go version per
// https://go.dev/doc/devel/release (1 version behind latest)
go 1.18

// Retract the first tag, which had the wrong naming convention. Also, retract
// the first beta as it had a memory bug and quickly fixed in v1.0.0-beta.2.
retract [v1.0.0-beta1, v1.0.0-beta.1]
