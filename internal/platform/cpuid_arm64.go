//go:build gc

package platform

import "runtime"

// CpuFeatures exposes the capabilities for this CPU, queried via the Has, HasExtra methods.
var CpuFeatures = loadCpuFeatureFlags()

// cpuFeatureFlags implements CpuFeatureFlags interface.
type cpuFeatureFlags struct {
	isar0 uint64
	isar1 uint64
}

// implemented in cpuid_arm64.s
func getisar0() uint64

// implemented in cpuid_arm64.s
func getisar1() uint64

func loadCpuFeatureFlags() CpuFeatureFlags {
	switch runtime.GOOS {
	case "darwin", "windows":
		// These OSes require ARMv8.1, which includes atomic instructions.
		return &cpuFeatureFlags{
			isar0: uint64(CpuFeatureArm64Atomic),
			isar1: 0,
		}
	case "linux", "freebsd":
		// These OSes allow reading the instruction set attribute registers.
		return &cpuFeatureFlags{
			isar0: getisar0(),
			isar1: getisar1(),
		}
	default:
		return &cpuFeatureFlags{}
	}
}

// Has implements the same method on the CpuFeatureFlags interface.
func (f *cpuFeatureFlags) Has(cpuFeature CpuFeature) bool {
	return (f.isar0 & uint64(cpuFeature)) != 0
}

// HasExtra implements the same method on the CpuFeatureFlags interface.
func (f *cpuFeatureFlags) HasExtra(cpuFeature CpuFeature) bool {
	return (f.isar1 & uint64(cpuFeature)) != 0
}

// Raw implements the same method on the CpuFeatureFlags interface.
func (f *cpuFeatureFlags) Raw() uint64 {
	// Below, we only set the first 4 bits for the features we care about,
	// instead of setting all the unnecessary bits obtained from the CPUID instruction.
	var ret uint64
	switch runtime.GOARCH {
	case "arm64":
		if f.Has(CpuFeatureArm64Atomic) {
			ret = 1 << 0
		}
	case "amd64":
		if f.Has(CpuFeatureAmd64SSE3) {
			ret = 1 << 0
		}
		if f.Has(CpuFeatureAmd64SSE4_1) {
			ret |= 1 << 1
		}
		if f.Has(CpuFeatureAmd64SSE4_2) {
			ret |= 1 << 2
		}
		if f.HasExtra(CpuExtraFeatureAmd64ABM) {
			ret |= 1 << 3
		}
	}
	return ret
}
