//go:build !amd64 || tinygo

package platform

var CpuFeatures CpuFeatureFlags = &cpuFeatureFlags{}

// cpuFeatureFlags implements CpuFeatureFlags for unsupported platforms
type cpuFeatureFlags struct{}

// Has implements the same method on the CpuFeatureFlags interface
func (c *cpuFeatureFlags) Has(cpuFeature CpuFeature) bool { return false }

// HasExtra implements the same method on the CpuFeatureFlags interface
func (c *cpuFeatureFlags) HasExtra(cpuFeature CpuFeature) bool { return false }

func cpuid(arg1, arg2 uint32) (eax, ebx, ecx, edx uint32) {
	panic("unsupported")
}
