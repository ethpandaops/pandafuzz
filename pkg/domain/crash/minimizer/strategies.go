package minimizer

import (
	"context"
	"fmt"
	"math"
	"sort"
)

// MinimizationStrategy defines the interface for input minimization strategies
type MinimizationStrategy interface {
	// Minimize attempts to reduce the input while maintaining crash reproducibility
	Minimize(
		ctx context.Context,
		input []byte,
		verifier ReproductionVerifier,
		progress *MinimizationProgress,
	) ([]byte, error)

	// Name returns the strategy name
	Name() string

	// Description returns a human-readable description
	Description() string
}

// ReproductionVerifier verifies that an input still reproduces the crash
type ReproductionVerifier interface {
	// Verify checks if the input reproduces the crash
	Verify(ctx context.Context, input []byte) (bool, error)
}

// BinarySearchStrategy implements binary search minimization
type BinarySearchStrategy struct {
	chunkSizeThreshold int
}

// NewBinarySearchStrategy creates a new binary search strategy
func NewBinarySearchStrategy() *BinarySearchStrategy {
	return &BinarySearchStrategy{
		chunkSizeThreshold: 1, // Minimum chunk size
	}
}

// Name returns the strategy name
func (s *BinarySearchStrategy) Name() string {
	return "binary_search"
}

// Description returns a human-readable description
func (s *BinarySearchStrategy) Description() string {
	return "Recursively removes half of the input until minimal reproducing input is found"
}

// Minimize implements the binary search minimization algorithm
func (s *BinarySearchStrategy) Minimize(
	ctx context.Context,
	input []byte,
	verifier ReproductionVerifier,
	progress *MinimizationProgress,
) ([]byte, error) {
	if len(input) == 0 {
		return input, nil
	}

	bestInput := make([]byte, len(input))
	copy(bestInput, input)

	// Try removing chunks of decreasing size
	chunkSize := len(input) / 2
	for chunkSize >= s.chunkSizeThreshold {
		improved := false

		for offset := 0; offset <= len(bestInput)-chunkSize; {
			select {
			case <-ctx.Done():
				return bestInput, ctx.Err()
			default:
			}

			// Try removing chunk at current offset
			candidate := make([]byte, 0, len(bestInput)-chunkSize)
			candidate = append(candidate, bestInput[:offset]...)
			candidate = append(candidate, bestInput[offset+chunkSize:]...)

			progress.Iterations++
			progress.CurrentSize = len(candidate)

			// Verify if candidate still reproduces crash
			reproduces, err := verifier.Verify(ctx, candidate)
			if err != nil {
				return bestInput, fmt.Errorf("verification failed: %w", err)
			}

			if reproduces {
				bestInput = candidate
				progress.BestSize = len(bestInput)
				progress.ReductionRatio = float64(progress.OriginalSize-progress.BestSize) / float64(progress.OriginalSize)
				improved = true
				// Don't advance offset - try same position with reduced input
			} else {
				// Move to next position
				offset++
			}
		}

		// If no improvement at this chunk size, try smaller chunks
		if !improved {
			chunkSize /= 2
		}
	}

	return bestInput, nil
}

// DeltaDebuggingStrategy implements the delta debugging algorithm
type DeltaDebuggingStrategy struct {
	maxGranularity int
}

// NewDeltaDebuggingStrategy creates a new delta debugging strategy
func NewDeltaDebuggingStrategy() *DeltaDebuggingStrategy {
	return &DeltaDebuggingStrategy{
		maxGranularity: 2,
	}
}

// Name returns the strategy name
func (s *DeltaDebuggingStrategy) Name() string {
	return "delta_debugging"
}

// Description returns a human-readable description
func (s *DeltaDebuggingStrategy) Description() string {
	return "Systematically tests subsets to find minimal failure-inducing input"
}

// Minimize implements the delta debugging minimization algorithm
func (s *DeltaDebuggingStrategy) Minimize(
	ctx context.Context,
	input []byte,
	verifier ReproductionVerifier,
	progress *MinimizationProgress,
) ([]byte, error) {
	if len(input) == 0 {
		return input, nil
	}

	// Start with granularity 2 (split in half)
	n := s.maxGranularity
	bestInput := make([]byte, len(input))
	copy(bestInput, input)

	for n <= len(bestInput) {
		improved := false
		subsetSize := len(bestInput) / n

		// Try each subset
		for i := 0; i < n; i++ {
			select {
			case <-ctx.Done():
				return bestInput, ctx.Err()
			default:
			}

			start := i * subsetSize
			end := start + subsetSize
			if i == n-1 {
				end = len(bestInput) // Handle remainder
			}

			// Create candidate without this subset
			candidate := make([]byte, 0, len(bestInput)-(end-start))
			candidate = append(candidate, bestInput[:start]...)
			candidate = append(candidate, bestInput[end:]...)

			progress.Iterations++
			progress.CurrentSize = len(candidate)

			// Test if candidate still reproduces
			reproduces, err := verifier.Verify(ctx, candidate)
			if err != nil {
				return bestInput, fmt.Errorf("verification failed: %w", err)
			}

			if reproduces {
				bestInput = candidate
				progress.BestSize = len(bestInput)
				progress.ReductionRatio = float64(progress.OriginalSize-progress.BestSize) / float64(progress.OriginalSize)
				improved = true
				n = s.maxGranularity // Reset granularity
				break
			}
		}

		if !improved {
			// Try complementary subsets
			for i := 0; i < n; i++ {
				select {
				case <-ctx.Done():
					return bestInput, ctx.Err()
				default:
				}

				start := i * subsetSize
				end := start + subsetSize
				if i == n-1 {
					end = len(bestInput)
				}

				// Create candidate with only this subset
				candidate := bestInput[start:end]

				progress.Iterations++
				progress.CurrentSize = len(candidate)

				reproduces, err := verifier.Verify(ctx, candidate)
				if err != nil {
					return bestInput, fmt.Errorf("verification failed: %w", err)
				}

				if reproduces {
					bestInput = candidate
					progress.BestSize = len(bestInput)
					progress.ReductionRatio = float64(progress.OriginalSize-progress.BestSize) / float64(progress.OriginalSize)
					improved = true
					n = s.maxGranularity
					break
				}
			}
		}

		if !improved {
			// Increase granularity
			n = int(math.Min(float64(n*2), float64(len(bestInput))))
		}
	}

	return bestInput, nil
}

// HierarchicalStrategy tries to preserve structure while minimizing
type HierarchicalStrategy struct {
	levelDetector StructureLevelDetector
}

// StructureLevelDetector detects structural levels in input
type StructureLevelDetector interface {
	// DetectLevels identifies hierarchical levels in the input
	DetectLevels(input []byte) []StructureLevel
}

// StructureLevel represents a level in the input structure
type StructureLevel struct {
	Start int
	End   int
	Depth int
	Type  string
}

// NewHierarchicalStrategy creates a new hierarchical strategy
func NewHierarchicalStrategy() *HierarchicalStrategy {
	return &HierarchicalStrategy{
		levelDetector: &defaultLevelDetector{},
	}
}

// Name returns the strategy name
func (s *HierarchicalStrategy) Name() string {
	return "hierarchical"
}

// Description returns a human-readable description
func (s *HierarchicalStrategy) Description() string {
	return "Minimizes while preserving hierarchical structure of the input"
}

// Minimize implements hierarchical minimization
func (s *HierarchicalStrategy) Minimize(
	ctx context.Context,
	input []byte,
	verifier ReproductionVerifier,
	progress *MinimizationProgress,
) ([]byte, error) {
	if len(input) == 0 {
		return input, nil
	}

	// Detect structure levels
	levels := s.levelDetector.DetectLevels(input)
	if len(levels) == 0 {
		// Fall back to binary search if no structure detected
		bs := NewBinarySearchStrategy()
		return bs.Minimize(ctx, input, verifier, progress)
	}

	bestInput := make([]byte, len(input))
	copy(bestInput, input)

	// Sort levels by depth (deepest first)
	sort.Slice(levels, func(i, j int) bool {
		return levels[i].Depth > levels[j].Depth
	})

	// Try removing each level
	for _, level := range levels {
		select {
		case <-ctx.Done():
			return bestInput, ctx.Err()
		default:
		}

		// Skip if level is outside current input bounds
		if level.Start >= len(bestInput) || level.End > len(bestInput) {
			continue
		}

		// Create candidate without this level
		candidate := make([]byte, 0, len(bestInput)-(level.End-level.Start))
		candidate = append(candidate, bestInput[:level.Start]...)
		candidate = append(candidate, bestInput[level.End:]...)

		progress.Iterations++
		progress.CurrentSize = len(candidate)

		// Verify if candidate still reproduces
		reproduces, err := verifier.Verify(ctx, candidate)
		if err != nil {
			return bestInput, fmt.Errorf("verification failed: %w", err)
		}

		if reproduces {
			bestInput = candidate
			progress.BestSize = len(bestInput)
			progress.ReductionRatio = float64(progress.OriginalSize-progress.BestSize) / float64(progress.OriginalSize)

			// Update levels after removal
			levels = s.adjustLevels(levels, level.Start, level.End-level.Start)
		}
	}

	return bestInput, nil
}

// adjustLevels updates level positions after a removal
func (s *HierarchicalStrategy) adjustLevels(levels []StructureLevel, removeStart, removeLen int) []StructureLevel {
	adjusted := make([]StructureLevel, 0, len(levels))
	for _, level := range levels {
		if level.End <= removeStart {
			// Level is before removal
			adjusted = append(adjusted, level)
		} else if level.Start >= removeStart+removeLen {
			// Level is after removal
			level.Start -= removeLen
			level.End -= removeLen
			adjusted = append(adjusted, level)
		}
		// Skip levels that overlap with removed section
	}
	return adjusted
}

// defaultLevelDetector provides basic structure detection
type defaultLevelDetector struct{}

// DetectLevels implements basic bracket/delimiter detection
func (d *defaultLevelDetector) DetectLevels(input []byte) []StructureLevel {
	var levels []StructureLevel
	var stack []int

	delimiters := map[byte]byte{
		'(': ')',
		'[': ']',
		'{': '}',
		'<': '>',
	}

	for i, b := range input {
		if _, isOpen := delimiters[b]; isOpen {
			stack = append(stack, i)
		} else {
			for open, close := range delimiters {
				if b == close && len(stack) > 0 {
					start := stack[len(stack)-1]
					if input[start] == open {
						levels = append(levels, StructureLevel{
							Start: start,
							End:   i + 1,
							Depth: len(stack),
							Type:  string([]byte{open, close}),
						})
						stack = stack[:len(stack)-1]
						break
					}
				}
			}
		}
	}

	return levels
}

// TokenBasedStrategy minimizes based on token boundaries
type TokenBasedStrategy struct {
	tokenizer Tokenizer
}

// Tokenizer splits input into tokens
type Tokenizer interface {
	// Tokenize splits input into tokens
	Tokenize(input []byte) []Token
}

// Token represents a unit in the input
type Token struct {
	Data  []byte
	Type  string
	Start int
	End   int
}

// NewTokenBasedStrategy creates a new token-based strategy
func NewTokenBasedStrategy() *TokenBasedStrategy {
	return &TokenBasedStrategy{
		tokenizer: &defaultTokenizer{},
	}
}

// Name returns the strategy name
func (s *TokenBasedStrategy) Name() string {
	return "token_based"
}

// Description returns a human-readable description
func (s *TokenBasedStrategy) Description() string {
	return "Minimizes by removing tokens while preserving crash"
}

// Minimize implements token-based minimization
func (s *TokenBasedStrategy) Minimize(
	ctx context.Context,
	input []byte,
	verifier ReproductionVerifier,
	progress *MinimizationProgress,
) ([]byte, error) {
	if len(input) == 0 {
		return input, nil
	}

	// Tokenize input
	tokens := s.tokenizer.Tokenize(input)
	if len(tokens) == 0 {
		return input, nil
	}

	// Start with all tokens
	activeTokens := make([]bool, len(tokens))
	for i := range activeTokens {
		activeTokens[i] = true
	}

	// Try removing each token
	changed := true
	for changed {
		changed = false

		for i := range tokens {
			if !activeTokens[i] {
				continue
			}

			select {
			case <-ctx.Done():
				return s.reconstructInput(tokens, activeTokens), ctx.Err()
			default:
			}

			// Try removing this token
			activeTokens[i] = false
			candidate := s.reconstructInput(tokens, activeTokens)

			progress.Iterations++
			progress.CurrentSize = len(candidate)

			// Verify if candidate still reproduces
			reproduces, err := verifier.Verify(ctx, candidate)
			if err != nil {
				activeTokens[i] = true // Restore token
				return s.reconstructInput(tokens, activeTokens), fmt.Errorf("verification failed: %w", err)
			}

			if reproduces {
				// Keep token removed
				progress.BestSize = len(candidate)
				progress.ReductionRatio = float64(progress.OriginalSize-progress.BestSize) / float64(progress.OriginalSize)
				changed = true
			} else {
				// Restore token
				activeTokens[i] = true
			}
		}
	}

	return s.reconstructInput(tokens, activeTokens), nil
}

// reconstructInput rebuilds input from active tokens
func (s *TokenBasedStrategy) reconstructInput(tokens []Token, activeTokens []bool) []byte {
	var result []byte
	for i, token := range tokens {
		if activeTokens[i] {
			result = append(result, token.Data...)
		}
	}
	return result
}

// defaultTokenizer provides basic tokenization
type defaultTokenizer struct{}

// Tokenize implements basic whitespace and delimiter tokenization
func (t *defaultTokenizer) Tokenize(input []byte) []Token {
	var tokens []Token
	var current []byte
	start := 0

	isDelimiter := func(b byte) bool {
		return b == ' ' || b == '\t' || b == '\n' || b == '\r' ||
			b == ',' || b == ';' || b == ':' || b == '.' ||
			b == '(' || b == ')' || b == '[' || b == ']' ||
			b == '{' || b == '}' || b == '<' || b == '>'
	}

	for i, b := range input {
		if isDelimiter(b) {
			// Save current token if not empty
			if len(current) > 0 {
				tokens = append(tokens, Token{
					Data:  current,
					Type:  "word",
					Start: start,
					End:   i,
				})
				current = nil
			}
			// Add delimiter as token
			tokens = append(tokens, Token{
				Data:  []byte{b},
				Type:  "delimiter",
				Start: i,
				End:   i + 1,
			})
			start = i + 1
		} else {
			if len(current) == 0 {
				start = i
			}
			current = append(current, b)
		}
	}

	// Add final token if exists
	if len(current) > 0 {
		tokens = append(tokens, Token{
			Data:  current,
			Type:  "word",
			Start: start,
			End:   len(input),
		})
	}

	return tokens
}
