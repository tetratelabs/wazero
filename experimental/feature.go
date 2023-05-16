package experimental

import "github.com/tetratelabs/wazero/internal/features"

// EnableFeatures enables the list of features passed as arguments.
//
// Features are global because they affect properties of wazero which cannot
// be decoupled, such as interaction with system capabilities that affect the
// entire process instead of a particular wazero.Runtime instance.
//
// Currently, the following features are supported:
//
//   - "hugepages" on linux, enables the use of huge pages in memory mappings
//
// Features are always turned off by default, and may be activated by calling
// this function, which should usually happen before creating any wazero.Runtime
// instances. Once enabled, features cannot be disabled.
func EnableFeatures(featureNames ...string) { features.Enable(featureNames...) }
