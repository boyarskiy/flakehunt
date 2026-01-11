// Package runner implements the execution loop for flakehunt.
package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/boyarskiy/flakehunt/internal/model"
)

// Config holds the configuration for the runner.
type Config struct {
	Runs     int
	Timeout  time.Duration
	OutDir   string
	KeepRuns int
	Tool     model.Tool
	Command  []string
	Adapter  model.Adapter
}

// Result holds the results of all runs.
type Result struct {
	RunResults   []*model.RunResult
	RunsExecuted int
	LatestDir    string
}

// Run executes the test command repeatedly and collects results.
func Run(ctx context.Context, cfg *Config) (*Result, error) {
	if cfg.Runs <= 0 {
		return nil, fmt.Errorf("--runs must be a positive integer, got %d", cfg.Runs)
	}
	if len(cfg.Command) == 0 {
		return nil, fmt.Errorf("test command is required after --")
	}
	if cfg.Adapter == nil {
		return nil, fmt.Errorf("adapter is required")
	}

	// Create output directories
	latestDir := filepath.Join(cfg.OutDir, "latest")
	runsDir := filepath.Join(latestDir, "runs")

	// Remove existing latest directory to ensure clean state
	if err := os.RemoveAll(latestDir); err != nil {
		return nil, fmt.Errorf("failed to clean output directory %s: %w", latestDir, err)
	}

	if err := os.MkdirAll(runsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create runs directory %s: %w", runsDir, err)
	}

	var runResults []*model.RunResult
	startTime := time.Now()

	for i := 1; i <= cfg.Runs; i++ {
		// Check timeout
		if cfg.Timeout > 0 && time.Since(startTime) >= cfg.Timeout {
			break
		}

		// Check context cancellation
		if ctx.Err() != nil {
			break
		}

		runDir := filepath.Join(runsDir, fmt.Sprintf("%03d", i))
		if err := os.MkdirAll(runDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create run directory %s: %w", runDir, err)
		}

		result, err := executeRun(ctx, cfg, runDir, i)
		if err != nil {
			// Record the error but continue with other runs
			result = &model.RunResult{
				RunIndex: i,
				Error:    err.Error(),
			}
		}
		runResults = append(runResults, result)
	}

	// Apply keep-runs cleanup
	if cfg.KeepRuns > 0 && len(runResults) > cfg.KeepRuns {
		if err := cleanupOldRuns(runsDir, cfg.KeepRuns); err != nil {
			// Log but don't fail on cleanup errors
			fmt.Fprintf(os.Stderr, "warning: failed to cleanup old runs: %v\n", err)
		}
	}

	return &Result{
		RunResults:   runResults,
		RunsExecuted: len(runResults),
		LatestDir:    latestDir,
	}, nil
}

// executeRun executes a single test run.
func executeRun(ctx context.Context, cfg *Config, runDir string, runIndex int) (*model.RunResult, error) {
	// Build the command with adapter-specific arguments
	cmdArgs := cfg.Adapter.BuildCommand(runDir, cfg.Command)
	if len(cmdArgs) == 0 {
		return nil, fmt.Errorf("adapter returned empty command")
	}

	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = filepath.Dir(cfg.OutDir) // Run from project root

	// Capture stdout and stderr
	stdoutFile, err := os.Create(filepath.Join(runDir, "stdout.txt"))
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout.txt: %w", err)
	}
	defer stdoutFile.Close()

	stderrFile, err := os.Create(filepath.Join(runDir, "stderr.txt"))
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr.txt: %w", err)
	}
	defer stderrFile.Close()

	cmd.Stdout = io.MultiWriter(stdoutFile, os.Stdout)
	cmd.Stderr = io.MultiWriter(stderrFile, os.Stderr)

	// Execute the command
	// Note: We don't treat non-zero exit as an error since tests may fail
	_ = cmd.Run()

	// Verify artifact exists
	artifactPath := cfg.Adapter.ExpectedArtifact(runDir)
	if _, err := os.Stat(artifactPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("expected artifact not found: %s. Ensure the test command produces the required output file", artifactPath)
	}

	// Parse the results
	result, err := cfg.Adapter.Parse(runDir)
	if err != nil {
		return nil, fmt.Errorf("failed to parse run results: %w", err)
	}
	result.RunIndex = runIndex

	return result, nil
}

// cleanupOldRuns removes run directories beyond the keepRuns limit.
func cleanupOldRuns(runsDir string, keepRuns int) error {
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return err
	}

	// Sort entries by name (which is the run number)
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}
	sort.Strings(dirs)

	// Remove oldest directories beyond keep limit
	if len(dirs) > keepRuns {
		toRemove := dirs[:len(dirs)-keepRuns]
		for _, dir := range toRemove {
			path := filepath.Join(runsDir, dir)
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("failed to remove %s: %w", path, err)
			}
		}
	}

	return nil
}

// ParseRunIndex extracts the run index from a run directory name.
func ParseRunIndex(name string) (int, error) {
	return strconv.Atoi(name)
}
