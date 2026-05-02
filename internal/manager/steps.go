package manager

import (
	"context"
	"fmt"
	"io"
	"os"
)

// Step represents a single setup operation.
type Step struct {
	Name     string
	Fn       func(ctx context.Context) error
	Optional bool // if true, errors are logged but don't stop execution
}

// StepResult records the outcome of a single step.
type StepResult struct {
	Name    string
	Skipped bool
	Err     error
}

// ProgressFunc is called to report step progress.
type ProgressFunc func(step string, current, total int)

// RunSteps executes a sequence of steps, reporting progress.
// It stops on the first non-optional error and returns all results collected so far.
func RunSteps(ctx context.Context, steps []Step, progress ProgressFunc) []StepResult {
	results := make([]StepResult, 0, len(steps))
	for i, step := range steps {
		select {
		case <-ctx.Done():
			return results
		default:
		}

		if progress != nil {
			progress(step.Name, i+1, len(steps))
		}

		err := step.Fn(ctx)
		if err != nil {
			if step.Optional {
				fmt.Fprintf(os.Stderr, "  [optional] %s: %v\n", step.Name, err)
				err = nil
			} else {
				return append(results, StepResult{Name: step.Name, Err: err})
			}
		}

		results = append(results, StepResult{Name: step.Name, Err: err})
	}
	return results
}

// PrintResults writes step results to w.
func PrintResults(w io.Writer, results []StepResult) {
	for _, r := range results {
		switch {
		case r.Err != nil:
			fmt.Fprintf(w, "  FAIL  %s: %v\n", r.Name, r.Err)
		case r.Skipped:
			fmt.Fprintf(w, "  SKIP  %s\n", r.Name)
		default:
			fmt.Fprintf(w, "  OK    %s\n", r.Name)
		}
	}
}
