package deduplication

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/deduplication/algorithms"
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
)

// ExampleUsage demonstrates how to use the crash deduplication service
func ExampleUsage(repo repository.CrashRepository) {
	ctx := context.Background()

	// 1. Create deduplication service with custom configuration
	config := Config{
		DefaultAlgorithm:    "hash_based",
		SimilarityThreshold: 0.85,
		BatchSize:           100,
		EnableStatistics:    true,
		MaxCandidates:       1000,
	}

	service := NewService(repo, config)

	// 2. Register multiple deduplication algorithms

	// Hash-based algorithm for exact matches
	hashConfig := algorithms.HashBasedConfig{
		UseInputHash:        false, // Don't use input hash for deduplication
		UseSignatureHash:    true,  // Use signature hash
		UseStackTrace:       true,  // Consider stack traces
		NormalizeStackTrace: true,  // Normalize to ignore addresses
		IgnoreAddresses:     true,
		IgnoreLineNumbers:   true,
		TopFramesCount:      5,
	}
	hashAlgo := algorithms.NewHashBased(hashConfig)

	// Fuzzy matching for similar crashes
	fuzzyConfig := algorithms.FuzzyMatchingConfig{
		MinSimilarity:    0.80,
		UseLevenshtein:   true,
		UseJaroWinkler:   true,
		UseLCS:           true,
		WeightStackTrace: 0.5,
		WeightFunctions:  0.3,
		WeightLibraries:  0.2,
		MaxEditDistance:  10,
	}
	fuzzyAlgo := algorithms.NewFuzzyMatching(fuzzyConfig)

	// Register algorithms
	if err := service.RegisterAlgorithm(hashAlgo); err != nil {
		log.Fatalf("Failed to register hash algorithm: %v", err)
	}
	if err := service.RegisterAlgorithm(fuzzyAlgo); err != nil {
		log.Fatalf("Failed to register fuzzy algorithm: %v", err)
	}

	// 3. Process a new crash
	newCrash := createSampleCrash()

	result, err := service.ProcessCrash(ctx, newCrash)
	if err != nil {
		log.Fatalf("Failed to process crash: %v", err)
	}

	// 4. Handle deduplication result
	if result.IsDuplicate {
		fmt.Printf("Crash is a duplicate of crash %s\n", result.OriginalCrash.ID)
		fmt.Printf("Confidence: %.2f\n", result.Confidence)
		fmt.Printf("Original crash has occurred %d times\n", result.OriginalCrash.OccurrenceCount)
	} else {
		fmt.Println("This is a new unique crash")

		// Save the new crash
		if err := repo.Create(ctx, newCrash); err != nil {
			log.Fatalf("Failed to save crash: %v", err)
		}

		// Check for similar crashes
		if len(result.SimilarCrashes) > 0 {
			fmt.Printf("Found %d similar crashes:\n", len(result.SimilarCrashes))
			for _, similar := range result.SimilarCrashes {
				fmt.Printf("  - %s (Type: %s, Severity: %s)\n",
					similar.ID, similar.Type, similar.Severity)
			}
		}
	}

	// 5. Process batch of crashes efficiently
	crashes := generateMultipleCrashes()

	batchResults, err := service.ProcessBatch(ctx, crashes)
	if err != nil {
		log.Printf("Batch processing encountered errors: %v", err)
	}

	// Analyze batch results
	duplicateCount := 0
	uniqueCount := 0
	for i, result := range batchResults {
		if result != nil {
			if result.IsDuplicate {
				duplicateCount++
			} else {
				uniqueCount++
				// Save unique crashes
				if err := repo.Create(ctx, crashes[i]); err != nil {
					log.Printf("Failed to save crash: %v", err)
				}
			}
		}
	}

	fmt.Printf("Batch processing complete: %d duplicates, %d unique crashes\n",
		duplicateCount, uniqueCount)

	// 6. Group similar crashes for analysis
	groups, err := service.GroupSimilarCrashes(ctx, "fuzzy_matching")
	if err != nil {
		log.Fatalf("Failed to group crashes: %v", err)
	}

	fmt.Printf("\nFound %d crash groups:\n", len(groups))
	for i, group := range groups {
		if len(group) > 1 {
			fmt.Printf("Group %d: %d similar crashes\n", i+1, len(group))
			fmt.Printf("  Representative: %s\n", group[0].String())
			fmt.Printf("  Common type: %s\n", group[0].Type)

			// Calculate total occurrences in group
			totalOccurrences := uint64(0)
			for _, crash := range group {
				totalOccurrences += crash.OccurrenceCount
			}
			fmt.Printf("  Total occurrences: %d\n", totalOccurrences)
		}
	}

	// 7. Get deduplication statistics
	if stats := service.GetStatistics(); stats != nil {
		fmt.Println("\nDeduplication Statistics:")
		fmt.Printf("  Total processed: %d\n", stats.TotalProcessed)
		fmt.Printf("  Duplicates found: %d\n", stats.DuplicatesFound)
		fmt.Printf("  Unique groups: %d\n", stats.UniqueGroups)
		fmt.Printf("  Average group size: %.2f\n", stats.AverageGroupSize)
		fmt.Printf("  Processing time: %s\n", stats.ProcessingTime)

		fmt.Println("  Algorithm usage:")
		for algo, count := range stats.AlgorithmUsage {
			fmt.Printf("    %s: %d times\n", algo, count)
		}
	}

	// 8. Find all duplicates of a specific crash
	specificCrashID := "crash_12345"
	duplicates, err := service.FindDuplicatesOf(ctx, specificCrashID, "hash_based")
	if err != nil {
		log.Printf("Failed to find duplicates: %v", err)
	} else {
		fmt.Printf("\nFound %d duplicates of crash %s\n", len(duplicates), specificCrashID)
	}
}

// Helper function to create a sample crash
func createSampleCrash() *types.Crash {
	stackTrace := `#0 0x7fff12345678 in malloc at malloc.c:123
#1 0x7fff23456789 in processBuffer at buffer.c:456
#2 0x7fff34567890 in handleRequest at server.c:789
#3 0x7fff45678901 in main at main.c:101`

	targetInfo := types.TargetInfo{
		Name:        "example_server",
		Version:     "1.2.3",
		Command:     "./example_server --port 8080",
		Environment: "production",
	}

	crash, _ := types.NewCrash(
		[]byte("malformed_input_data_12345"),
		stackTrace,
		targetInfo,
	)

	return crash
}

// Helper function to generate multiple test crashes
func generateMultipleCrashes() []*types.Crash {
	crashes := make([]*types.Crash, 10)

	// Create some duplicate crashes
	for i := 0; i < 5; i++ {
		stackTrace := `#0 0x7fff12345678 in malloc
#1 0x7fff23456789 in processData
#2 0x7fff34567890 in main`

		targetInfo := types.TargetInfo{
			Name:    "test_app",
			Version: "1.0.0",
			Command: "./test_app",
		}

		crash, _ := types.NewCrash(
			[]byte(fmt.Sprintf("input_%d", i)),
			stackTrace,
			targetInfo,
		)
		crashes[i] = crash
	}

	// Create some unique crashes
	for i := 5; i < 10; i++ {
		stackTrace := fmt.Sprintf(`#0 0x7fff%d in function%d
#1 0x7fff%d in caller%d
#2 0x7fff%d in main`, i, i, i+1, i, i+2)

		targetInfo := types.TargetInfo{
			Name:    "test_app",
			Version: "1.0.0",
			Command: "./test_app",
		}

		crash, _ := types.NewCrash(
			[]byte(fmt.Sprintf("unique_input_%d", i)),
			stackTrace,
			targetInfo,
		)
		crashes[i] = crash
	}

	return crashes
}

// AdvancedDeduplicationExample shows advanced deduplication scenarios
func AdvancedDeduplicationExample(repo repository.CrashRepository) {
	ctx := context.Background()

	// Create service with custom configuration for high-volume processing
	config := Config{
		DefaultAlgorithm:    "fuzzy_matching",
		SimilarityThreshold: 0.75, // Lower threshold for broader matching
		BatchSize:           500,  // Larger batch size
		EnableStatistics:    true,
		MaxCandidates:       5000, // More candidates for thorough deduplication
	}

	service := NewService(repo, config)

	// Register custom-configured algorithms

	// Strict hash-based for critical crashes
	strictHashConfig := algorithms.HashBasedConfig{
		UseInputHash:        true, // Also consider input
		UseSignatureHash:    true,
		UseStackTrace:       true,
		NormalizeStackTrace: false, // Don't normalize for strict matching
		TopFramesCount:      10,    // Consider more frames
	}
	strictHashAlgo := algorithms.NewHashBased(strictHashConfig)

	// Lenient fuzzy matching for exploratory deduplication
	lenientFuzzyConfig := algorithms.FuzzyMatchingConfig{
		MinSimilarity:    0.60, // Lower threshold
		UseLevenshtein:   true,
		UseJaroWinkler:   true,
		UseLCS:           true,
		WeightStackTrace: 0.4, // Lower weight on exact stack trace
		WeightFunctions:  0.4, // Higher weight on function names
		WeightLibraries:  0.2,
		MaxEditDistance:  20, // Allow more edits
	}
	lenientFuzzyAlgo := algorithms.NewFuzzyMatching(lenientFuzzyConfig)

	service.RegisterAlgorithm(strictHashAlgo)
	service.RegisterAlgorithm(lenientFuzzyAlgo)

	// Example: Processing crashes with different severity levels
	criticalCrashes, _ := repo.FindBySeverity(ctx, types.SeverityCritical)

	fmt.Println("Processing critical crashes with strict deduplication...")
	for _, crash := range criticalCrashes {
		result, err := service.ProcessCrash(ctx, crash)
		if err != nil {
			continue
		}

		if result.IsDuplicate && result.Confidence >= 0.95 {
			// High confidence duplicate of critical crash
			fmt.Printf("Critical crash %s is duplicate of %s (confidence: %.2f)\n",
				crash.ID, result.OriginalCrash.ID, result.Confidence)

			// Mark as duplicate in metadata
			crash.SetMetadata("duplicate_of", result.OriginalCrash.ID)
			crash.SetMetadata("dedup_confidence", fmt.Sprintf("%.2f", result.Confidence))
			repo.Update(ctx, crash)
		}
	}

	// Example: Periodic deduplication maintenance
	fmt.Println("\nRunning periodic deduplication maintenance...")

	// Find all crashes from the last 24 hours
	recentCrashes, _ := repo.FindRecent(ctx, time.Now().Add(-24*time.Hour))
	_ = recentCrashes // Note: In a real implementation, you might analyze these

	// Group them to find patterns
	groups, _ := service.GroupSimilarCrashes(ctx, "fuzzy_matching")

	// Analyze groups for trending issues
	for _, group := range groups {
		if len(group) >= 5 { // Significant cluster
			// Calculate metrics for the group
			var totalOccurrences uint64
			var severityCount = make(map[types.Severity]int)

			for _, crash := range group {
				totalOccurrences += crash.OccurrenceCount
				severityCount[crash.Severity]++
			}

			// Alert on trending crash patterns
			if totalOccurrences > 100 {
				fmt.Printf("ALERT: Trending crash pattern detected!\n")
				fmt.Printf("  Group size: %d unique crashes\n", len(group))
				fmt.Printf("  Total occurrences: %d\n", totalOccurrences)
				fmt.Printf("  Severity distribution: %+v\n", severityCount)
				fmt.Printf("  Representative crash: %s\n", group[0].String())
			}
		}
	}
}
