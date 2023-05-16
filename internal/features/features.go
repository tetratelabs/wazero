// Package features implements a feature flagging mechanism for wazero.
//
// Features are intended to control properties of the code that can only be
// enabled globally.
package features

import (
	"os"
	"strings"
	"sync/atomic"
)

const (
	// EnvVarName is the name of the environment variable which contains the
	// list of feature flags.
	EnvVarName = "WAZEROFEATURES"

	// HugePages is the feature flag representing the use of huge pages in
	// memory mappings on linux.
	HugePages Flags = 1 << iota
)

// Flag is a bit set representing feature flags.
type Flags uint32

var flags uint32

// EnableFromEnvironment extracts the list of wazero features enabled from the
// WAZEROFEATURES environment variable.
func EnableFromEnvironment() {
	features := os.Getenv(EnvVarName)
	Enable(strings.Split(features, ",")...)
}

// Enable the list of features passed as arguments.
//
// The function is idempotent and atomic, features that are already present are
// skipped.
//
// Unrecognized features are ignored.
func Enable(features ...string) {
	var flags Flags

	for _, feature := range features {
		switch feature {
		case "hugepages":
			flags |= HugePages
		}
	}

	Set(flags)
}

// Set sets the given feature flags.
func Set(features Flags) {
	for {
		oldFlags := atomic.LoadUint32(&flags)
		newFlags := oldFlags | uint32(features)
		if atomic.CompareAndSwapUint32(&flags, oldFlags, newFlags) {
			break
		}
	}
}

// Have returns true if the given features are enabled.
func Have(features Flags) bool {
	return (atomic.LoadUint32(&flags) & uint32(features)) == uint32(features)
}
