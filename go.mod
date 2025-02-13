module github.com/tetratelabs/wazero

// Floor Go version of wazero (current - 2)
go 1.22.0

// All the beta tags are retracted and replaced with "pre" to prevent users
// from accidentally upgrading into the broken beta 1.
retract (
	v1.0.0-beta1
	v1.0.0-beta.3
	v1.0.0-beta.2
	v1.0.0-beta.1
)
