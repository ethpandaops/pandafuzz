package fuzzer

// HongFuzzConfig holds HongFuzz-specific configuration options
type HongFuzzConfig struct {
	PersistentMode     bool   // Enable persistent mode for LLVMFuzzerTestOneInput targets
	HardwareFeedback   string // Hardware feedback type: "none", "instructions", "branches", "edges"
	VerifyCrashes      bool   // Enable crash verification to reduce false positives
	NetworkPort        int    // Port for network fuzzing (0 = disabled)
	MutationsPerRun    int    // Number of mutations per run
	UseInstrumentation bool   // Enable instrumentation for better coverage
	MinimizeCorpus     bool   // Enable corpus minimization
	ReportFile         string // Path to report file for detailed stats
	MaxFileSize        int    // Maximum file size in bytes for generated inputs
}
