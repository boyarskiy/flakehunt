// Package jest implements the flakehunt adapter for Jest test runner.
package jest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/boyarskiy/flakehunt/internal/model"
)

const (
	artifactFilename = "jest.json"
)

// Adapter implements model.Adapter for Jest.
type Adapter struct{}

// New creates a new Jest adapter.
func New() *Adapter {
	return &Adapter{}
}

// BuildCommand returns the command arguments with Jest JSON output configured.
// It injects --json, --outputFile, and optionally --runInBand.
func (a *Adapter) BuildCommand(runDir string, userCmd []string) []string {
	if len(userCmd) == 0 {
		return nil
	}

	outputPath := filepath.Join(runDir, artifactFilename)

	// Copy user command to avoid mutation
	result := make([]string, len(userCmd))
	copy(result, userCmd)

	// Check if user already specified execution mode flags
	hasExecutionMode := false
	executionModeFlags := []string{
		"--runInBand",
		"--maxWorkers",
		"-w",
		"--workerThreads",
	}
	for _, arg := range userCmd {
		for _, flag := range executionModeFlags {
			if strings.HasPrefix(arg, flag) || arg == "-i" {
				hasExecutionMode = true
				break
			}
		}
		if hasExecutionMode {
			break
		}
	}

	// Add --runInBand if no execution mode specified (preferred to reduce concurrency noise)
	if !hasExecutionMode {
		result = append(result, "--runInBand")
	}

	// Always inject --json and --outputFile
	result = append(result, "--json", "--outputFile", outputPath)

	return result
}

// Parse reads the Jest JSON output from runDir and returns test results.
func (a *Adapter) Parse(runDir string) (*model.RunResult, error) {
	artifactPath := filepath.Join(runDir, artifactFilename)

	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return nil, &ParseError{
			File:    artifactPath,
			Message: fmt.Sprintf("failed to read file: %v", err),
			Action:  "Ensure Jest completed and produced output. Check that --outputFile was used correctly.",
		}
	}

	if len(data) == 0 {
		return nil, &ParseError{
			File:    artifactPath,
			Message: "file is empty",
			Action:  "Ensure Jest completed successfully. The JSON output file should not be empty.",
		}
	}

	var jestOutput JestOutput
	if err := json.Unmarshal(data, &jestOutput); err != nil {
		return nil, &ParseError{
			File:    artifactPath,
			Message: fmt.Sprintf("invalid JSON: %v", err),
			Action:  "Ensure Jest produced valid JSON output. The file may be corrupted or incomplete.",
		}
	}

	tests, err := extractTests(&jestOutput, artifactPath)
	if err != nil {
		return nil, err
	}

	// Sort tests by TestID for deterministic output
	sort.Slice(tests, func(i, j int) bool {
		return tests[i].TestID < tests[j].TestID
	})

	return &model.RunResult{
		Tests: tests,
	}, nil
}

// ExpectedArtifact returns the path to the expected Jest artifact.
func (a *Adapter) ExpectedArtifact(runDir string) string {
	return filepath.Join(runDir, artifactFilename)
}

// extractTests converts Jest output to model.TestResult slice.
func extractTests(output *JestOutput, artifactPath string) ([]model.TestResult, error) {
	var tests []model.TestResult

	for i, testResult := range output.TestResults {
		// Validate required field: name (file path)
		if testResult.Name == "" {
			return nil, &ParseError{
				File:    artifactPath,
				Message: fmt.Sprintf("testResults[%d].name is missing or empty", i),
				Action:  "Jest output is malformed. Each test result must have a 'name' field containing the file path.",
			}
		}

		for j, assertion := range testResult.AssertionResults {
			// Validate required field: fullName
			if assertion.FullName == "" {
				return nil, &ParseError{
					File:    artifactPath,
					Message: fmt.Sprintf("testResults[%d].assertionResults[%d].fullName is missing or empty", i, j),
					Action:  "Jest output is malformed. Each assertion result must have a 'fullName' field.",
				}
			}

			// Validate required field: status
			if assertion.Status == "" {
				return nil, &ParseError{
					File:    artifactPath,
					Message: fmt.Sprintf("testResults[%d].assertionResults[%d].status is missing or empty", i, j),
					Action:  "Jest output is malformed. Each assertion result must have a 'status' field.",
				}
			}

			// Build TestID: <filePath>::<fullTestName>
			testID := fmt.Sprintf("%s::%s", testResult.Name, assertion.FullName)

			// Map Jest status to model.Outcome
			outcome, err := mapStatus(assertion.Status)
			if err != nil {
				return nil, &ParseError{
					File:    artifactPath,
					Message: fmt.Sprintf("testResults[%d].assertionResults[%d].status has unknown value: %q", i, j, assertion.Status),
					Action:  "Jest produced an unexpected status value. Expected: 'passed', 'failed', 'pending', or 'skipped'.",
				}
			}

			// Extract duration (Jest reports in milliseconds)
			duration := time.Duration(assertion.Duration) * time.Millisecond

			// Extract failure message
			var failureMsg string
			if len(assertion.FailureMessages) > 0 {
				failureMsg = strings.Join(assertion.FailureMessages, "\n")
			}

			tests = append(tests, model.TestResult{
				TestID:         testID,
				Outcome:        outcome,
				Duration:       duration,
				FailureMessage: failureMsg,
			})
		}
	}

	return tests, nil
}

// mapStatus converts Jest status strings to model.Outcome.
func mapStatus(status string) (model.Outcome, error) {
	switch status {
	case "passed":
		return model.OutcomePass, nil
	case "failed":
		return model.OutcomeFail, nil
	case "pending", "skipped", "todo", "disabled":
		return model.OutcomeSkip, nil
	default:
		return "", fmt.Errorf("unknown status: %s", status)
	}
}

// JestOutput represents the top-level Jest JSON output structure.
type JestOutput struct {
	NumFailedTestSuites  int          `json:"numFailedTestSuites"`
	NumFailedTests       int          `json:"numFailedTests"`
	NumPassedTestSuites  int          `json:"numPassedTestSuites"`
	NumPassedTests       int          `json:"numPassedTests"`
	NumPendingTestSuites int          `json:"numPendingTestSuites"`
	NumPendingTests      int          `json:"numPendingTests"`
	NumTotalTestSuites   int          `json:"numTotalTestSuites"`
	NumTotalTests        int          `json:"numTotalTests"`
	Success              bool         `json:"success"`
	TestResults          []TestResult `json:"testResults"`
}

// TestResult represents a Jest test file result.
type TestResult struct {
	Name             string            `json:"name"`
	Status           string            `json:"status"`
	AssertionResults []AssertionResult `json:"assertionResults"`
}

// AssertionResult represents a single Jest test assertion.
type AssertionResult struct {
	FullName        string   `json:"fullName"`
	Status          string   `json:"status"`
	Title           string   `json:"title"`
	Duration        int64    `json:"duration"`
	FailureMessages []string `json:"failureMessages"`
}

// ParseError provides actionable error information for parsing failures.
type ParseError struct {
	File    string
	Message string
	Action  string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error in %s: %s. %s", e.File, e.Message, e.Action)
}
