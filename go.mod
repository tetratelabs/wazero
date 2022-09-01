module github.com/tetratelabs/wazero

// This should be the minimum supported Go version per
// https://go.dev/doc/devel/release (1 version behind latest)
go 1.18

// All the beta tags are retracted and replaced with "pre" to prevent users
// from accidentally upgrading into the broken beta 1
retract (
	v1.0.0-beta1
	v1.0.0-beta.1
	v1.0.0-beta.2
	v1.0.0-beta.3
)
