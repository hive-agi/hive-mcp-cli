package setup

import (
	"fmt"

	"github.com/hive-agi/hive-mcp-cli/internal/pkg/util"
)

// Step defines the interface for setup steps with idempotent execution
type Step interface {
	Name() string
	Check() (bool, error) // Returns true if already done
	Run() error
	Rollback() error
}

// Result captures the outcome of a step execution
type Result struct {
	StepName string
	Skipped  bool
	Error    error
}

// Runner manages step execution with progress reporting
type Runner struct {
	Steps    []Step
	Results  []Result
	OnStart  func(step Step)
	OnDone   func(step Step, skipped bool, err error)
}

// NewRunner creates a runner with default console output
func NewRunner(steps []Step) *Runner {
	return &Runner{
		Steps:   steps,
		Results: make([]Result, 0, len(steps)),
		OnStart: func(step Step) {
			fmt.Printf("→ %s...\n", step.Name())
		},
		OnDone: func(step Step, skipped bool, err error) {
			if err != nil {
				fmt.Printf("  ✗ %s: %v\n", step.Name(), err)
			} else if skipped {
				fmt.Printf("  ✓ %s (already done)\n", step.Name())
			} else {
				fmt.Printf("  ✓ %s\n", step.Name())
			}
		},
	}
}

// RunAll executes all steps in order, stopping on first error
func (r *Runner) RunAll() error {
	for _, step := range r.Steps {
		if r.OnStart != nil {
			r.OnStart(step)
		}

		// Check if already done
		done, err := step.Check()
		if err != nil {
			result := Result{StepName: step.Name(), Error: err}
			r.Results = append(r.Results, result)
			if r.OnDone != nil {
				r.OnDone(step, false, err)
			}
			return fmt.Errorf("check failed for %s: %w", step.Name(), err)
		}

		if done {
			result := Result{StepName: step.Name(), Skipped: true}
			r.Results = append(r.Results, result)
			if r.OnDone != nil {
				r.OnDone(step, true, nil)
			}
			continue
		}

		// Execute step
		err = step.Run()
		result := Result{StepName: step.Name(), Error: err}
		r.Results = append(r.Results, result)

		if r.OnDone != nil {
			r.OnDone(step, false, err)
		}

		if err != nil {
			return fmt.Errorf("step %s failed: %w", step.Name(), err)
		}
	}
	return nil
}

// RollbackFrom rolls back from the given step index backwards
func (r *Runner) RollbackFrom(index int) []error {
	var errors []error
	for i := index; i >= 0; i-- {
		if i < len(r.Steps) {
			if err := r.Steps[i].Rollback(); err != nil {
				errors = append(errors, fmt.Errorf("rollback %s: %w", r.Steps[i].Name(), err))
			}
		}
	}
	return errors
}

// RunAll is a convenience function for simple usage
func RunAll(steps []Step) error {
	runner := NewRunner(steps)
	return runner.RunAll()
}

// expandPath expands ~ to home directory using shared utility
func expandPath(path string) string {
	return util.ExpandPath(path)
}
