package minimizer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Mock verifier for testing
type mockVerifier struct {
	mock.Mock
	verifyFunc func([]byte) bool
}

func (m *mockVerifier) Verify(ctx context.Context, input []byte) (bool, error) {
	if m.verifyFunc != nil {
		return m.verifyFunc(input), nil
	}
	args := m.Called(ctx, input)
	return args.Bool(0), args.Error(1)
}

func TestBinarySearchStrategy(t *testing.T) {
	strategy := NewBinarySearchStrategy()

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "binary_search", strategy.Name())
	})

	t.Run("Description", func(t *testing.T) {
		assert.NotEmpty(t, strategy.Description())
	})

	t.Run("EmptyInput", func(t *testing.T) {
		ctx := context.Background()
		verifier := &mockVerifier{}
		progress := &MinimizationProgress{}

		result, err := strategy.Minimize(ctx, []byte{}, verifier, progress)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("MinimizeSuccess", func(t *testing.T) {
		ctx := context.Background()
		input := []byte("ABCDEFGHIJKLMNOP")
		verifier := &mockVerifier{
			verifyFunc: func(candidate []byte) bool {
				// Only the first 2 bytes are needed to reproduce the crash
				return len(candidate) >= 2 && candidate[0] == 'A' && candidate[1] == 'B'
			},
		}
		progress := &MinimizationProgress{
			OriginalSize: len(input),
			CurrentSize:  len(input),
			BestSize:     len(input),
		}

		result, err := strategy.Minimize(ctx, input, verifier, progress)
		require.NoError(t, err)
		assert.Equal(t, []byte("AB"), result)
		assert.Equal(t, 2, progress.BestSize)
		assert.Greater(t, progress.Iterations, 0)
		assert.Greater(t, progress.ReductionRatio, 0.8) // At least 80% reduction
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		input := []byte("ABCDEFGH")
		verifier := &mockVerifier{}
		progress := &MinimizationProgress{
			OriginalSize: len(input),
			CurrentSize:  len(input),
			BestSize:     len(input),
		}

		// Cancel immediately
		cancel()

		result, err := strategy.Minimize(ctx, input, verifier, progress)
		assert.Equal(t, context.Canceled, err)
		assert.Equal(t, input, result)
	})
}

func TestDeltaDebuggingStrategy(t *testing.T) {
	strategy := NewDeltaDebuggingStrategy()

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "delta_debugging", strategy.Name())
	})

	t.Run("Description", func(t *testing.T) {
		assert.NotEmpty(t, strategy.Description())
	})

	t.Run("MinimizeWithSubsets", func(t *testing.T) {
		ctx := context.Background()
		input := []byte("ABCD")
		verifier := &mockVerifier{
			verifyFunc: func(candidate []byte) bool {
				// Only "C" reproduces the crash
				return string(candidate) == "C"
			},
		}
		progress := &MinimizationProgress{
			OriginalSize: len(input),
			CurrentSize:  len(input),
			BestSize:     len(input),
		}

		result, err := strategy.Minimize(ctx, input, verifier, progress)
		require.NoError(t, err)
		assert.Equal(t, []byte("C"), result)
	})
}

func TestHierarchicalStrategy(t *testing.T) {
	strategy := NewHierarchicalStrategy()

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "hierarchical", strategy.Name())
	})

	t.Run("Description", func(t *testing.T) {
		assert.NotEmpty(t, strategy.Description())
	})

	t.Run("MinimizeWithStructure", func(t *testing.T) {
		ctx := context.Background()
		input := []byte("{[()]}")
		verifier := &mockVerifier{
			verifyFunc: func(candidate []byte) bool {
				// Must have matching brackets
				stack := 0
				for _, b := range candidate {
					switch b {
					case '{', '[', '(':
						stack++
					case '}', ']', ')':
						stack--
						if stack < 0 {
							return false
						}
					}
				}
				return stack == 0 && len(candidate) > 0
			},
		}
		progress := &MinimizationProgress{
			OriginalSize: len(input),
			CurrentSize:  len(input),
			BestSize:     len(input),
		}

		result, err := strategy.Minimize(ctx, input, verifier, progress)
		require.NoError(t, err)
		assert.True(t, len(result) < len(input))
		assert.True(t, len(result) > 0)
	})
}

func TestTokenBasedStrategy(t *testing.T) {
	strategy := NewTokenBasedStrategy()

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "token_based", strategy.Name())
	})

	t.Run("Description", func(t *testing.T) {
		assert.NotEmpty(t, strategy.Description())
	})

	t.Run("MinimizeByTokens", func(t *testing.T) {
		ctx := context.Background()
		input := []byte("hello world test")
		verifier := &mockVerifier{
			verifyFunc: func(candidate []byte) bool {
				// Must contain "hello"
				return len(candidate) >= 5 && string(candidate[:5]) == "hello"
			},
		}
		progress := &MinimizationProgress{
			OriginalSize: len(input),
			CurrentSize:  len(input),
			BestSize:     len(input),
		}

		result, err := strategy.Minimize(ctx, input, verifier, progress)
		require.NoError(t, err)
		assert.True(t, len(result) < len(input))
		assert.Contains(t, string(result), "hello")
	})
}

func TestDefaultTokenizer(t *testing.T) {
	tokenizer := &defaultTokenizer{}

	t.Run("BasicTokenization", func(t *testing.T) {
		input := []byte("hello, world!")
		tokens := tokenizer.Tokenize(input)

		assert.GreaterOrEqual(t, len(tokens), 3) // At least "hello", delimiter(s), "world"
		assert.Equal(t, "hello", string(tokens[0].Data))
		assert.Equal(t, "word", tokens[0].Type)
	})

	t.Run("EmptyInput", func(t *testing.T) {
		tokens := tokenizer.Tokenize([]byte{})
		assert.Empty(t, tokens)
	})
}

func TestDefaultLevelDetector(t *testing.T) {
	detector := &defaultLevelDetector{}

	t.Run("NestedStructure", func(t *testing.T) {
		input := []byte("{[()]}")
		levels := detector.DetectLevels(input)

		assert.Len(t, levels, 3)
		// Verify we have nested levels
		depths := make(map[int]bool)
		for _, level := range levels {
			depths[level.Depth] = true
		}
		assert.True(t, len(depths) > 1, "Should have multiple depth levels")
	})

	t.Run("NoStructure", func(t *testing.T) {
		input := []byte("plain text")
		levels := detector.DetectLevels(input)
		assert.Empty(t, levels)
	})
}
