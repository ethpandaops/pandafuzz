package analyzer

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
	"github.com/sirupsen/logrus"
)

// StackParser provides advanced stack trace parsing capabilities
type StackParser interface {
	// ParseStackTrace parses a raw stack trace into structured frames
	ParseStackTrace(ctx context.Context, stackTrace string) (*ParsedStackTrace, error)

	// ExtractCrashLocation extracts the most relevant crash location
	ExtractCrashLocation(ctx context.Context, stackTrace string) (*CrashLocation, error)

	// GenerateSignature generates a crash signature for deduplication
	GenerateSignature(ctx context.Context, stackTrace string) (*types.CrashSignature, error)

	// DetectFormat attempts to detect the stack trace format
	DetectFormat(stackTrace string) StackTraceFormat

	// Start initializes the parser
	Start(ctx context.Context) error

	// Stop cleanly shuts down the parser
	Stop() error
}

// StackTraceFormat represents different stack trace formats
type StackTraceFormat string

const (
	FormatGDB              StackTraceFormat = "gdb"
	FormatLLDB             StackTraceFormat = "lldb"
	FormatAddressSanitizer StackTraceFormat = "asan"
	FormatThreadSanitizer  StackTraceFormat = "tsan"
	FormatValgrind         StackTraceFormat = "valgrind"
	FormatGo               StackTraceFormat = "go"
	FormatRust             StackTraceFormat = "rust"
	FormatJava             StackTraceFormat = "java"
	FormatPython           StackTraceFormat = "python"
	FormatGeneric          StackTraceFormat = "generic"
	FormatUnknown          StackTraceFormat = "unknown"
)

// ParsedStackTrace represents a fully parsed stack trace
type ParsedStackTrace struct {
	Format       StackTraceFormat
	Frames       []StackFrame
	ThreadInfo   *ThreadInfo
	SignalInfo   *SignalInfo
	CrashAddress string
	CrashReason  string
	Architecture string
	Platform     string
	Metadata     map[string]string
}

// StackFrame represents a single frame in the stack trace
type StackFrame struct {
	Number       int
	Address      string
	Function     string
	Module       string
	SourceFile   string
	LineNumber   int
	ColumnNumber int
	Offset       string
	IsInlined    bool
	IsSystem     bool
	IsSymbolized bool
	RawLine      string
}

// ThreadInfo contains thread-specific information
type ThreadInfo struct {
	ID       string
	Name     string
	State    string
	Priority int
}

// SignalInfo contains signal-specific information
type SignalInfo struct {
	Number      int
	Name        string
	Code        string
	Description string
	FaultAddr   string
}

// CrashLocation represents the most relevant location of a crash
type CrashLocation struct {
	Function   string
	Module     string
	SourceFile string
	LineNumber int
	Confidence float64
}

// StackParserConfig holds configuration for the stack parser
type StackParserConfig struct {
	MaxFramesToParse   int
	EnableDemangling   bool
	EnableSymbolCache  bool
	IgnoreSystemFrames bool
}

// stackParser implements the StackParser interface
type stackParser struct {
	log            logrus.FieldLogger
	config         StackParserConfig
	parsers        map[StackTraceFormat]frameParser
	demanglerCache map[string]string
}

// frameParser is a function type for format-specific frame parsing
type frameParser func(string) (*StackFrame, error)

// NewStackParser creates a new stack parser instance
func NewStackParser(log logrus.FieldLogger, config StackParserConfig) StackParser {
	return &stackParser{
		log:            log.WithField("component", "stack_parser"),
		config:         config,
		parsers:        make(map[StackTraceFormat]frameParser),
		demanglerCache: make(map[string]string),
	}
}

// Start initializes the parser with format-specific parsers
func (sp *stackParser) Start(ctx context.Context) error {
	sp.log.Info("Starting stack parser")

	// Register format-specific parsers
	sp.registerParsers()

	sp.log.WithField("formats", len(sp.parsers)).Info("Stack parser started")
	return nil
}

// Stop cleanly shuts down the parser
func (sp *stackParser) Stop() error {
	sp.log.Info("Stopping stack parser")
	return nil
}

// ParseStackTrace parses a raw stack trace into structured frames
func (sp *stackParser) ParseStackTrace(ctx context.Context, stackTrace string) (*ParsedStackTrace, error) {
	if stackTrace == "" {
		return nil, errors.New("stack trace cannot be empty")
	}

	// Detect format
	format := sp.DetectFormat(stackTrace)
	sp.log.WithField("format", format).Debug("Detected stack trace format")

	parsed := &ParsedStackTrace{
		Format:   format,
		Frames:   make([]StackFrame, 0),
		Metadata: make(map[string]string),
	}

	// Extract signal information if present
	parsed.SignalInfo = sp.extractSignalInfo(stackTrace)

	// Extract thread information if present
	parsed.ThreadInfo = sp.extractThreadInfo(stackTrace)

	// Extract architecture and platform
	parsed.Architecture = sp.extractArchitecture(stackTrace)
	parsed.Platform = sp.extractPlatform(stackTrace)

	// Parse frames based on format
	frames, err := sp.parseFramesByFormat(stackTrace, format)
	if err != nil {
		return nil, fmt.Errorf("failed to parse frames: %w", err)
	}

	// Filter and enhance frames
	for _, frame := range frames {
		if sp.config.IgnoreSystemFrames && frame.IsSystem {
			continue
		}

		// Demangle function names if enabled
		if sp.config.EnableDemangling && frame.Function != "" {
			frame.Function = sp.demangleSymbol(frame.Function)
		}

		parsed.Frames = append(parsed.Frames, frame)

		// Limit frames if configured
		if sp.config.MaxFramesToParse > 0 && len(parsed.Frames) >= sp.config.MaxFramesToParse {
			break
		}
	}

	// Extract crash address and reason
	parsed.CrashAddress = sp.extractCrashAddress(stackTrace)
	parsed.CrashReason = sp.extractCrashReason(stackTrace)

	return parsed, nil
}

// DetectFormat attempts to detect the stack trace format
func (sp *stackParser) DetectFormat(stackTrace string) StackTraceFormat {
	stackLower := strings.ToLower(stackTrace)

	// Check for format-specific indicators
	formatIndicators := map[StackTraceFormat][]string{
		FormatAddressSanitizer: {"==error", "addresssanitizer", "asan", "heap-buffer-overflow", "stack-buffer-overflow"},
		FormatThreadSanitizer:  {"==warning", "threadsanitizer", "tsan", "data race"},
		FormatValgrind:         {"==", "valgrind", "memcheck", "invalid read", "invalid write"},
		FormatGDB:              {"#0", "gdb", "program received signal", "backtrace"},
		FormatLLDB:             {"frame #", "lldb", "stop reason"},
		FormatGo:               {"goroutine", "panic:", "runtime."},
		FormatRust:             {"thread", "panicked at", "stack backtrace:"},
		FormatJava:             {"at ", ".java:", "exception in thread"},
		FormatPython:           {"traceback", `file "`, "line ", "in <module>"},
	}

	for format, indicators := range formatIndicators {
		matchCount := 0
		for _, indicator := range indicators {
			if strings.Contains(stackLower, indicator) {
				matchCount++
			}
		}
		// Require at least 2 indicators for a confident match
		if matchCount >= 2 {
			return format
		}
	}

	// Check for generic frame patterns
	if sp.hasGenericFramePattern(stackTrace) {
		return FormatGeneric
	}

	return FormatUnknown
}

// hasGenericFramePattern checks if the trace has generic frame patterns
func (sp *stackParser) hasGenericFramePattern(stackTrace string) bool {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`#\d+\s+0x[0-9a-fA-F]+`),         // #0 0x12345678
		regexp.MustCompile(`\[\s*\d+\]\s+0x[0-9a-fA-F]+`),   // [0] 0x12345678
		regexp.MustCompile(`at\s+0x[0-9a-fA-F]+`),           // at 0x12345678
		regexp.MustCompile(`^\s*0x[0-9a-fA-F]+\s+in\s+\S+`), // 0x12345678 in function
	}

	for _, pattern := range patterns {
		if pattern.MatchString(stackTrace) {
			return true
		}
	}
	return false
}

// parseFramesByFormat parses frames based on the detected format
func (sp *stackParser) parseFramesByFormat(stackTrace string, format StackTraceFormat) ([]StackFrame, error) {
	parser, exists := sp.parsers[format]
	if !exists {
		// Fallback to generic parser
		parser = sp.parseGenericFrame
	}

	frames := make([]StackFrame, 0)
	scanner := bufio.NewScanner(strings.NewReader(stackTrace))
	lineNumber := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNumber++

		frame, err := parser(line)
		if err != nil {
			// Log but continue parsing
			sp.log.WithError(err).WithField("line", lineNumber).Debug("Failed to parse frame")
			continue
		}

		if frame != nil {
			frame.RawLine = line
			frames = append(frames, *frame)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning stack trace: %w", err)
	}

	return frames, nil
}

// registerParsers registers format-specific frame parsers
func (sp *stackParser) registerParsers() {
	sp.parsers[FormatGDB] = sp.parseGDBFrame
	sp.parsers[FormatLLDB] = sp.parseLLDBFrame
	sp.parsers[FormatAddressSanitizer] = sp.parseASANFrame
	sp.parsers[FormatThreadSanitizer] = sp.parseTSANFrame
	sp.parsers[FormatValgrind] = sp.parseValgrindFrame
	sp.parsers[FormatGo] = sp.parseGoFrame
	sp.parsers[FormatRust] = sp.parseRustFrame
	sp.parsers[FormatJava] = sp.parseJavaFrame
	sp.parsers[FormatPython] = sp.parsePythonFrame
	sp.parsers[FormatGeneric] = sp.parseGenericFrame
}

// Format-specific frame parsers

func (sp *stackParser) parseGDBFrame(line string) (*StackFrame, error) {
	// GDB format: #0  0x00007ffff7a62428 in __GI_raise (sig=sig@entry=6) at ../sysdeps/unix/sysv/linux/raise.c:54
	pattern := regexp.MustCompile(`#(\d+)\s+(?:0x([0-9a-fA-F]+)\s+)?(?:in\s+)?(\S+)?(?:\s+\(([^)]*)\))?\s*(?:at\s+([^:]+):(\d+))?`)

	matches := pattern.FindStringSubmatch(line)
	if len(matches) == 0 {
		return nil, nil
	}

	frame := &StackFrame{
		IsSymbolized: true,
	}

	// Frame number
	if matches[1] != "" {
		frame.Number, _ = strconv.Atoi(matches[1])
	}

	// Address
	if matches[2] != "" {
		frame.Address = "0x" + matches[2]
	}

	// Function
	if matches[3] != "" {
		frame.Function = matches[3]
		if frame.Function == "??" || strings.HasPrefix(frame.Function, "0x") {
			frame.IsSymbolized = false
		}
	}

	// Source file
	if matches[5] != "" {
		frame.SourceFile = matches[5]
		frame.Module = sp.extractModuleFromPath(frame.SourceFile)
	}

	// Line number
	if matches[6] != "" {
		frame.LineNumber, _ = strconv.Atoi(matches[6])
	}

	// Check if system frame
	frame.IsSystem = sp.isSystemFrame(frame)

	return frame, nil
}

func (sp *stackParser) parseLLDBFrame(line string) (*StackFrame, error) {
	// LLDB format: frame #0: 0x00007fff6c7c6c5a libsystem_kernel.dylib`__pthread_kill + 10
	pattern := regexp.MustCompile(`frame\s+#(\d+):\s+0x([0-9a-fA-F]+)\s+(\S+?)(?:` + "`" + `(\S+))?(?:\s+\+\s+(\d+))?`)

	matches := pattern.FindStringSubmatch(line)
	if len(matches) == 0 {
		return nil, nil
	}

	frame := &StackFrame{
		IsSymbolized: true,
	}

	// Frame number
	if matches[1] != "" {
		frame.Number, _ = strconv.Atoi(matches[1])
	}

	// Address
	if matches[2] != "" {
		frame.Address = "0x" + matches[2]
	}

	// Module
	if matches[3] != "" {
		frame.Module = matches[3]
	}

	// Function
	if matches[4] != "" {
		frame.Function = matches[4]
	} else {
		frame.IsSymbolized = false
	}

	// Offset
	if matches[5] != "" {
		frame.Offset = matches[5]
	}

	// Check if system frame
	frame.IsSystem = sp.isSystemFrame(frame)

	return frame, nil
}

func (sp *stackParser) parseASANFrame(line string) (*StackFrame, error) {
	// ASAN format: #0 0x7f8a0a6c3b8f in __interceptor_malloc (/usr/lib/x86_64-linux-gnu/libasan.so.5+0x10db8f)
	pattern := regexp.MustCompile(`\s*#(\d+)\s+0x([0-9a-fA-F]+)\s+(?:in\s+)?(\S+)?(?:\s+\(([^)]+)\))?`)

	matches := pattern.FindStringSubmatch(line)
	if len(matches) == 0 {
		return nil, nil
	}

	frame := &StackFrame{
		IsSymbolized: true,
	}

	// Frame number
	if matches[1] != "" {
		frame.Number, _ = strconv.Atoi(matches[1])
	}

	// Address
	if matches[2] != "" {
		frame.Address = "0x" + matches[2]
	}

	// Function
	if matches[3] != "" {
		frame.Function = matches[3]
		if strings.HasPrefix(frame.Function, "<") || frame.Function == "" {
			frame.IsSymbolized = false
		}
	}

	// Module info (may include offset)
	if matches[4] != "" {
		moduleInfo := matches[4]
		if idx := strings.LastIndex(moduleInfo, "+"); idx > 0 {
			frame.Module = moduleInfo[:idx]
			frame.Offset = moduleInfo[idx+1:]
		} else {
			frame.Module = moduleInfo
		}
	}

	// Check if system frame
	frame.IsSystem = sp.isSystemFrame(frame)

	return frame, nil
}

func (sp *stackParser) parseTSANFrame(line string) (*StackFrame, error) {
	// TSAN format similar to ASAN
	return sp.parseASANFrame(line)
}

func (sp *stackParser) parseValgrindFrame(line string) (*StackFrame, error) {
	// Valgrind format: ==12345== at 0x4C2FB0F: malloc (in /usr/lib/valgrind/vgpreload_memcheck-amd64-linux.so)
	pattern := regexp.MustCompile(`==\d+==\s+(?:at|by)\s+0x([0-9A-F]+):\s+(\S+)\s*(?:\(in\s+([^)]+)\))?`)

	matches := pattern.FindStringSubmatch(line)
	if len(matches) == 0 {
		return nil, nil
	}

	frame := &StackFrame{
		IsSymbolized: true,
	}

	// Address
	if matches[1] != "" {
		frame.Address = "0x" + matches[1]
	}

	// Function
	if matches[2] != "" {
		frame.Function = matches[2]
		if frame.Function == "???" {
			frame.IsSymbolized = false
		}
	}

	// Module
	if matches[3] != "" {
		frame.Module = matches[3]
	}

	// Check if system frame
	frame.IsSystem = sp.isSystemFrame(frame)

	return frame, nil
}

func (sp *stackParser) parseGoFrame(line string) (*StackFrame, error) {
	// Go format: main.(*Server).Start(0xc0000b4000, 0xc0000a6000, 0x0, 0x0)
	//            /path/to/file.go:123 +0x265

	// Try function line
	funcPattern := regexp.MustCompile(`^(\S+)\([^)]*\)$`)
	if matches := funcPattern.FindStringSubmatch(strings.TrimSpace(line)); matches != nil {
		return &StackFrame{
			Function:     matches[1],
			IsSymbolized: true,
		}, nil
	}

	// Try file/line info
	filePattern := regexp.MustCompile(`^\s*(\S+):(\d+)\s*(?:\+0x([0-9a-fA-F]+))?`)
	if matches := filePattern.FindStringSubmatch(line); matches != nil {
		frame := &StackFrame{
			SourceFile:   matches[1],
			IsSymbolized: true,
		}

		if matches[2] != "" {
			frame.LineNumber, _ = strconv.Atoi(matches[2])
		}

		if matches[3] != "" {
			frame.Offset = "0x" + matches[3]
		}

		frame.Module = sp.extractModuleFromPath(frame.SourceFile)
		return frame, nil
	}

	return nil, nil
}

func (sp *stackParser) parseRustFrame(line string) (*StackFrame, error) {
	// Rust format: 0: std::panic::catch_unwind
	//                 at /rustc/xxx/library/std/src/panic.rs:142:14

	// Try frame header
	headerPattern := regexp.MustCompile(`^\s*(\d+):\s+(.+)$`)
	if matches := headerPattern.FindStringSubmatch(line); matches != nil {
		frame := &StackFrame{
			IsSymbolized: true,
		}

		if matches[1] != "" {
			frame.Number, _ = strconv.Atoi(matches[1])
		}

		if matches[2] != "" {
			frame.Function = strings.TrimSpace(matches[2])
		}

		return frame, nil
	}

	// Try location line
	locPattern := regexp.MustCompile(`^\s*at\s+([^:]+):(\d+):(\d+)`)
	if matches := locPattern.FindStringSubmatch(line); matches != nil {
		frame := &StackFrame{
			SourceFile:   matches[1],
			IsSymbolized: true,
		}

		if matches[2] != "" {
			frame.LineNumber, _ = strconv.Atoi(matches[2])
		}

		if matches[3] != "" {
			frame.ColumnNumber, _ = strconv.Atoi(matches[3])
		}

		frame.Module = sp.extractModuleFromPath(frame.SourceFile)
		return frame, nil
	}

	return nil, nil
}

func (sp *stackParser) parseJavaFrame(line string) (*StackFrame, error) {
	// Java format: at com.example.MyClass.method(MyClass.java:42)
	pattern := regexp.MustCompile(`^\s*at\s+(\S+)\(([^:)]+):?(\d+)?\)`)

	matches := pattern.FindStringSubmatch(line)
	if len(matches) == 0 {
		return nil, nil
	}

	frame := &StackFrame{
		IsSymbolized: true,
	}

	// Function (class.method)
	if matches[1] != "" {
		frame.Function = matches[1]
	}

	// Source file
	if matches[2] != "" {
		frame.SourceFile = matches[2]
		// Extract module from package name
		if parts := strings.Split(frame.Function, "."); len(parts) > 2 {
			frame.Module = strings.Join(parts[:2], ".")
		}
	}

	// Line number
	if matches[3] != "" {
		frame.LineNumber, _ = strconv.Atoi(matches[3])
	}

	return frame, nil
}

func (sp *stackParser) parsePythonFrame(line string) (*StackFrame, error) {
	// Python format: File "/path/to/file.py", line 42, in function_name
	pattern := regexp.MustCompile(`^\s*File\s+"([^"]+)",\s+line\s+(\d+)(?:,\s+in\s+(\S+))?`)

	matches := pattern.FindStringSubmatch(line)
	if len(matches) == 0 {
		return nil, nil
	}

	frame := &StackFrame{
		IsSymbolized: true,
	}

	// Source file
	if matches[1] != "" {
		frame.SourceFile = matches[1]
		frame.Module = sp.extractModuleFromPath(frame.SourceFile)
	}

	// Line number
	if matches[2] != "" {
		frame.LineNumber, _ = strconv.Atoi(matches[2])
	}

	// Function
	if matches[3] != "" {
		frame.Function = matches[3]
	}

	return frame, nil
}

func (sp *stackParser) parseGenericFrame(line string) (*StackFrame, error) {
	// Try various generic patterns
	patterns := []struct {
		pattern *regexp.Regexp
		parser  func([]string) *StackFrame
	}{
		{
			// #0 0x12345678 in function at file.c:123
			regexp.MustCompile(`#(\d+)\s+0x([0-9a-fA-F]+)\s+(?:in\s+)?(\S+)?(?:\s+at\s+([^:]+):(\d+))?`),
			func(m []string) *StackFrame {
				frame := &StackFrame{IsSymbolized: true}
				if m[1] != "" {
					frame.Number, _ = strconv.Atoi(m[1])
				}
				if m[2] != "" {
					frame.Address = "0x" + m[2]
				}
				if m[3] != "" {
					frame.Function = m[3]
				}
				if m[4] != "" {
					frame.SourceFile = m[4]
				}
				if m[5] != "" {
					frame.LineNumber, _ = strconv.Atoi(m[5])
				}
				return frame
			},
		},
		{
			// Simple address and function: 0x12345678 <function+offset>
			regexp.MustCompile(`0x([0-9a-fA-F]+)\s+<([^>+]+)(?:\+0x([0-9a-fA-F]+))?>?`),
			func(m []string) *StackFrame {
				frame := &StackFrame{IsSymbolized: true}
				if m[1] != "" {
					frame.Address = "0x" + m[1]
				}
				if m[2] != "" {
					frame.Function = m[2]
				}
				if m[3] != "" {
					frame.Offset = "0x" + m[3]
				}
				return frame
			},
		},
	}

	for _, p := range patterns {
		if matches := p.pattern.FindStringSubmatch(line); matches != nil {
			return p.parser(matches), nil
		}
	}

	return nil, nil
}

// Helper methods

func (sp *stackParser) isSystemFrame(frame *StackFrame) bool {
	systemIndicators := []string{
		"libc", "libpthread", "libsystem", "ntdll", "kernel32",
		"__libc_", "_start", "__pthread_", "clone", "start_thread",
		"__GI_", "syscall", "_dl_", "__cxa_",
	}

	combined := strings.ToLower(frame.Function + frame.Module + frame.SourceFile)
	for _, indicator := range systemIndicators {
		if strings.Contains(combined, indicator) {
			return true
		}
	}

	return false
}

func (sp *stackParser) extractModuleFromPath(path string) string {
	// Extract module name from file path
	base := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		base = path[idx+1:]
	} else if idx := strings.LastIndex(path, "\\"); idx >= 0 {
		base = path[idx+1:]
	}

	// Remove file extension
	if idx := strings.LastIndex(base, "."); idx > 0 {
		base = base[:idx]
	}

	return base
}

func (sp *stackParser) demangleSymbol(symbol string) string {
	// Check cache first
	if demangled, exists := sp.demanglerCache[symbol]; exists {
		return demangled
	}

	// Simple C++ demangling heuristics
	demangled := symbol

	// Handle common C++ patterns
	if strings.HasPrefix(symbol, "_Z") {
		// Simplified demangling - in production, use a proper demangler
		demangled = strings.TrimPrefix(symbol, "_Z")
		demangled = strings.ReplaceAll(demangled, "St", "std::")
		demangled = strings.ReplaceAll(demangled, "NS", "::")
	}

	// Cache result
	if sp.config.EnableSymbolCache {
		sp.demanglerCache[symbol] = demangled
	}

	return demangled
}

// Extract methods

func (sp *stackParser) extractSignalInfo(stackTrace string) *SignalInfo {
	// Common signal patterns
	patterns := []struct {
		pattern *regexp.Regexp
		parser  func([]string) *SignalInfo
	}{
		{
			// GDB style: Program received signal SIGSEGV, Segmentation fault.
			regexp.MustCompile(`Program received signal (\w+),\s*(.+)\.`),
			func(m []string) *SignalInfo {
				return &SignalInfo{
					Name:        m[1],
					Description: m[2],
				}
			},
		},
		{
			// ASAN style: SEGV on unknown address 0x000000000000
			regexp.MustCompile(`(\w+) on \w+ address (0x[0-9a-fA-F]+)`),
			func(m []string) *SignalInfo {
				return &SignalInfo{
					Name:      m[1],
					FaultAddr: m[2],
				}
			},
		},
		{
			// Generic: Signal 11 (SIGSEGV)
			regexp.MustCompile(`Signal (\d+) \((\w+)\)`),
			func(m []string) *SignalInfo {
				num, _ := strconv.Atoi(m[1])
				return &SignalInfo{
					Number: num,
					Name:   m[2],
				}
			},
		},
	}

	for _, p := range patterns {
		if matches := p.pattern.FindStringSubmatch(stackTrace); matches != nil {
			return p.parser(matches)
		}
	}

	return nil
}

func (sp *stackParser) extractThreadInfo(stackTrace string) *ThreadInfo {
	// Thread patterns
	patterns := []struct {
		pattern *regexp.Regexp
		parser  func([]string) *ThreadInfo
	}{
		{
			// Thread 1 "process_name" received signal
			regexp.MustCompile(`Thread (\d+)\s+"([^"]+)"`),
			func(m []string) *ThreadInfo {
				return &ThreadInfo{
					ID:   m[1],
					Name: m[2],
				}
			},
		},
		{
			// [Thread 0x7ffff7fc1700 (LWP 12345)]
			regexp.MustCompile(`\[Thread (0x[0-9a-fA-F]+) \(LWP (\d+)\)\]`),
			func(m []string) *ThreadInfo {
				return &ThreadInfo{
					ID: m[2],
				}
			},
		},
	}

	for _, p := range patterns {
		if matches := p.pattern.FindStringSubmatch(stackTrace); matches != nil {
			return p.parser(matches)
		}
	}

	return nil
}

func (sp *stackParser) extractArchitecture(stackTrace string) string {
	archPatterns := map[string][]string{
		"x86_64": {"x86_64", "amd64", "x64"},
		"x86":    {"i386", "i486", "i586", "i686", "x86_32"},
		"arm64":  {"arm64", "aarch64"},
		"arm":    {"arm", "armv7", "armhf"},
		"mips":   {"mips", "mipsel"},
		"ppc":    {"ppc", "powerpc"},
	}

	stackLower := strings.ToLower(stackTrace)
	for arch, patterns := range archPatterns {
		for _, pattern := range patterns {
			if strings.Contains(stackLower, pattern) {
				return arch
			}
		}
	}

	return "unknown"
}

func (sp *stackParser) extractPlatform(stackTrace string) string {
	platformPatterns := map[string][]string{
		"linux":   {"linux", "ubuntu", "debian", "redhat", "centos"},
		"windows": {"windows", "win32", "win64", "mingw"},
		"darwin":  {"darwin", "macos", "osx"},
		"freebsd": {"freebsd"},
		"android": {"android"},
	}

	stackLower := strings.ToLower(stackTrace)
	for platform, patterns := range platformPatterns {
		for _, pattern := range patterns {
			if strings.Contains(stackLower, pattern) {
				return platform
			}
		}
	}

	return "unknown"
}

func (sp *stackParser) extractCrashAddress(stackTrace string) string {
	// Look for crash address patterns
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?:at|access to) address (0x[0-9a-fA-F]+)`),
		regexp.MustCompile(`faulting address:\s*(0x[0-9a-fA-F]+)`),
		regexp.MustCompile(`SEGV on \w+ address (0x[0-9a-fA-F]+)`),
		regexp.MustCompile(`pc\s+(0x[0-9a-fA-F]+)`),
	}

	for _, pattern := range patterns {
		if matches := pattern.FindStringSubmatch(stackTrace); len(matches) > 1 {
			return matches[1]
		}
	}

	return ""
}

func (sp *stackParser) extractCrashReason(stackTrace string) string {
	// Extract crash reason from various formats
	reasons := []struct {
		pattern *regexp.Regexp
		extract func([]string) string
	}{
		{
			regexp.MustCompile(`(?i)(heap-buffer-overflow|stack-buffer-overflow|use-after-free|double-free)`),
			func(m []string) string { return m[1] },
		},
		{
			regexp.MustCompile(`(?i)(null pointer dereference|access violation|segmentation fault)`),
			func(m []string) string { return m[1] },
		},
		{
			regexp.MustCompile(`(?i)assertion\s+'([^']+)'\s+failed`),
			func(m []string) string { return fmt.Sprintf("assertion failed: %s", m[1]) },
		},
	}

	for _, r := range reasons {
		if matches := r.pattern.FindStringSubmatch(stackTrace); matches != nil {
			return r.extract(matches)
		}
	}

	return "unknown"
}

// ExtractCrashLocation extracts the most relevant crash location
func (sp *stackParser) ExtractCrashLocation(ctx context.Context, stackTrace string) (*CrashLocation, error) {
	parsed, err := sp.ParseStackTrace(ctx, stackTrace)
	if err != nil {
		return nil, fmt.Errorf("failed to parse stack trace: %w", err)
	}

	// Find the first non-system, symbolized frame
	for _, frame := range parsed.Frames {
		if !frame.IsSystem && frame.IsSymbolized && frame.Function != "" {
			return &CrashLocation{
				Function:   frame.Function,
				Module:     frame.Module,
				SourceFile: frame.SourceFile,
				LineNumber: frame.LineNumber,
				Confidence: sp.calculateLocationConfidence(frame),
			}, nil
		}
	}

	// If no suitable frame found, use the first frame
	if len(parsed.Frames) > 0 {
		frame := parsed.Frames[0]
		return &CrashLocation{
			Function:   frame.Function,
			Module:     frame.Module,
			SourceFile: frame.SourceFile,
			LineNumber: frame.LineNumber,
			Confidence: 0.5,
		}, nil
	}

	return nil, errors.New("no suitable crash location found")
}

func (sp *stackParser) calculateLocationConfidence(frame StackFrame) float64 {
	confidence := 0.0

	if frame.IsSymbolized {
		confidence += 0.3
	}
	if frame.Function != "" {
		confidence += 0.2
	}
	if frame.SourceFile != "" {
		confidence += 0.2
	}
	if frame.LineNumber > 0 {
		confidence += 0.2
	}
	if !frame.IsSystem {
		confidence += 0.1
	}

	return confidence
}

// GenerateSignature generates a crash signature for deduplication
func (sp *stackParser) GenerateSignature(ctx context.Context, stackTrace string) (*types.CrashSignature, error) {
	parsed, err := sp.ParseStackTrace(ctx, stackTrace)
	if err != nil {
		// Fallback to basic signature generation
		return types.NewCrashSignature(stackTrace)
	}

	// Build enhanced signature with parsed information
	topFrames := make([]string, 0, 5)
	functionNames := make([]string, 0)
	libraryNames := make([]string, 0)

	for _, frame := range parsed.Frames {
		if frame.IsSystem || !frame.IsSymbolized {
			continue
		}

		if frame.Function != "" && len(topFrames) < 5 {
			topFrames = append(topFrames, frame.Function)
		}

		if frame.Function != "" {
			functionNames = append(functionNames, frame.Function)
		}

		if frame.Module != "" {
			libraryNames = append(libraryNames, frame.Module)
		}
	}

	// Create signature using the parsed data
	sig := &types.CrashSignature{
		TopFrames:     topFrames,
		FunctionNames: functionNames,
		LibraryNames:  sp.uniqueStrings(libraryNames),
		SignatureType: string(parsed.Format),
		Confidence:    sp.calculateSignatureConfidence(parsed),
	}

	// Generate hash
	sig.Hash = sp.generateEnhancedHash(sig, parsed)

	return sig, nil
}

func (sp *stackParser) calculateSignatureConfidence(parsed *ParsedStackTrace) float64 {
	if len(parsed.Frames) == 0 {
		return 0.0
	}

	symbolizedCount := 0
	nonSystemCount := 0

	for _, frame := range parsed.Frames {
		if frame.IsSymbolized {
			symbolizedCount++
		}
		if !frame.IsSystem {
			nonSystemCount++
		}
	}

	symbolizationRatio := float64(symbolizedCount) / float64(len(parsed.Frames))
	nonSystemRatio := float64(nonSystemCount) / float64(len(parsed.Frames))

	// Weight factors
	confidence := (symbolizationRatio * 0.6) + (nonSystemRatio * 0.4)

	// Boost confidence for known formats
	if parsed.Format != FormatUnknown && parsed.Format != FormatGeneric {
		confidence = confidence*0.8 + 0.2
	}

	return confidence
}

func (sp *stackParser) generateEnhancedHash(sig *types.CrashSignature, parsed *ParsedStackTrace) string {
	// Build components for hashing
	components := make([]string, 0)

	// Add top frames
	components = append(components, sig.TopFrames...)

	// Add crash type information
	if parsed.CrashReason != "" {
		components = append(components, parsed.CrashReason)
	}

	// Add signal information if available
	if parsed.SignalInfo != nil && parsed.SignalInfo.Name != "" {
		components = append(components, parsed.SignalInfo.Name)
	}

	// Use the types package helper to generate the actual hash
	newSig, _ := types.NewCrashSignature(strings.Join(components, "|"))
	if newSig != nil {
		return newSig.Hash
	}
	return ""
}

func (sp *stackParser) uniqueStrings(strs []string) []string {
	seen := make(map[string]bool)
	unique := make([]string, 0)

	for _, str := range strs {
		if !seen[str] {
			seen[str] = true
			unique = append(unique, str)
		}
	}

	return unique
}
