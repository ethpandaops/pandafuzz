// Package minimizer provides functionality for reducing crash inputs to minimal reproducers.
//
// The minimizer service supports multiple strategies for input reduction while ensuring
// that the minimized input still reproduces the original crash. It includes features
// for resumable minimization, resource limits, and progress tracking.
//
// Example usage:
//
//	// Create minimizer service
//	minimizer, err := minimizer.NewService(crashRepo, fuzzerFactory)
//	if err != nil {
//	    return err
//	}
//
//	// Configure minimization options
//	options := &MinimizationOptions{
//	    MaxIterations: 1000,
//	    Timeout:       30 * time.Minute,
//	    Strategies:    []string{"binary_search", "delta_debugging"},
//	    ResourceLimits: &ResourceLimits{
//	        MaxMemory:        1024 * 1024 * 1024, // 1GB
//	        MaxExecutionTime: 5 * time.Second,
//	    },
//	}
//
//	// Start minimization
//	result, err := minimizer.MinimizeCrash(ctx, crashID, options)
//	if err != nil {
//	    return err
//	}
//
//	// Check progress
//	progress, err := minimizer.GetProgress(crashID)
//	if err != nil {
//	    return err
//	}
//
//	// Resume from saved state
//	result, err = minimizer.ResumeMinimization(ctx, crashID, savedProgress, options)
//
// The minimizer supports multiple strategies:
//   - Binary Search: Recursively removes half of the input
//   - Delta Debugging: Systematically tests subsets
//   - Hierarchical: Preserves input structure while minimizing
//   - Token-based: Removes tokens while preserving crash
//
// Each strategy implements the MinimizationStrategy interface and can be
// registered with the service for use during minimization.
package minimizer
