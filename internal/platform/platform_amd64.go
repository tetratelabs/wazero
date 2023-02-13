package platform

// init verifies that the current CPU supports the required AMD64 instructions
func init() {
	// ensure SSE4.1 is supported
	archRequirementsVerified = CpuFeatures.Has(CpuFeatureSSE4_1)
}
