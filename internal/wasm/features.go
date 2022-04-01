package wasm

import (
	"fmt"
)

// Features are the currently enabled features.
//
// Note: This is a bit flag until we have too many (>63). Flags are simpler to manage in multiple places than a map.
type Features uint64

// Features20191205 include those finished in WebAssembly 1.0 (20191205).
//
// See https://github.com/WebAssembly/proposals/blob/main/finished-proposals.md
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205
const Features20191205 = FeatureMutableGlobal

const (
	// FeatureMutableGlobal decides if global vars are allowed to be imported or exported (ExternTypeGlobal)
	// See https://github.com/WebAssembly/mutable-global
	FeatureMutableGlobal Features = 1 << iota

	// FeatureSignExtensionOps decides if parsing should succeed on wasm.GlobalType Mutable
	// See https://github.com/WebAssembly/spec/blob/main/proposals/sign-extension-ops/Overview.md
	FeatureSignExtensionOps
)

// Set assigns the value for the given feature.
func (f Features) Set(feature Features, val bool) Features {
	if val {
		return f | feature
	}
	return f &^ feature
}

// Get returns the value of the given feature.
func (f Features) Get(feature Features) bool {
	return f&feature != 0
}

// Require fails with a configuration error if the given feature is not enabled
func (f Features) Require(feature Features) error {
	if f&feature == 0 {
		return fmt.Errorf("feature %s is disabled", feature)
	}
	return nil
}

// String implements fmt.Stringer by returning each enabled feature.
func (f Features) String() string {
	switch f {
	case 0:
		return ""
	case FeatureMutableGlobal:
		// match https://github.com/WebAssembly/mutable-global
		return "mutable-global"
	case FeatureSignExtensionOps:
		// match https://github.com/WebAssembly/spec/blob/main/proposals/sign-extension-ops/Overview.md
		return "sign-extension-ops"
	default:
		return "undefined" // TODO: when there are multiple features join known ones on pipe (|)
	}
}
