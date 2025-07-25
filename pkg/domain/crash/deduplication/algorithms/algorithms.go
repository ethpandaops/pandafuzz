// Package algorithms provides various crash deduplication algorithms
// for identifying and grouping similar crashes.
package algorithms

// This package contains implementations of different deduplication algorithms:
//
// 1. Hash-based algorithm: Fast exact and near-exact matching using hashes
//    - Configurable normalization options
//    - Support for signature, input, and stack trace hashing
//    - Efficient for large-scale deduplication
//
// 2. Fuzzy matching algorithm: Advanced similarity matching using string metrics
//    - Levenshtein distance for edit-based similarity
//    - Jaro-Winkler distance for typo-tolerant matching
//    - Longest Common Subsequence for structural similarity
//    - Configurable weights for different crash components
//
// Usage:
//
//	// Create a hash-based algorithm
//	hashConfig := DefaultHashBasedConfig()
//	hashAlgo := NewHashBased(hashConfig)
//
//	// Create a fuzzy matching algorithm
//	fuzzyConfig := DefaultFuzzyMatchingConfig()
//	fuzzyAlgo := NewFuzzyMatching(fuzzyConfig)
//
//	// Register with deduplication service
//	service.RegisterAlgorithm(hashAlgo)
//	service.RegisterAlgorithm(fuzzyAlgo)
//
// Custom algorithms can be implemented by satisfying the Algorithm interface
// defined in the parent deduplication package.
