//go:build !wazero_testing

package buildoptions

// IstTest true if currently running unit tests. This can be used to
// insert the "test-time" assertions in the main code as `if buildoptions.IstTest { ... }` block,
// which will be optimized out by the final binary of wazero users.
const IstTest = false
