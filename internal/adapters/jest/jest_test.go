package jest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boyarskiy/flakehunt/internal/model"
)

func TestBuildCommand(t *testing.T) {
	adapter := New()

	tests := []struct {
		name     string
		runDir   string
		userCmd  []string
		wantArgs []string
	}{
		{
			name:    "basic command adds runInBand and json flags",
			runDir:  "/tmp/runs/001",
			userCmd: []string{"npx", "jest"},
			wantArgs: []string{
				"npx", "jest",
				"--runInBand",
				"--json", "--outputFile", "/tmp/runs/001/jest.json",
			},
		},
		{
			name:    "preserves user test pattern",
			runDir:  "/tmp/runs/001",
			userCmd: []string{"npx", "jest", "src/utils.test.ts"},
			wantArgs: []string{
				"npx", "jest", "src/utils.test.ts",
				"--runInBand",
				"--json", "--outputFile", "/tmp/runs/001/jest.json",
			},
		},
		{
			name:    "does not add runInBand if already specified",
			runDir:  "/tmp/runs/001",
			userCmd: []string{"npx", "jest", "--runInBand"},
			wantArgs: []string{
				"npx", "jest", "--runInBand",
				"--json", "--outputFile", "/tmp/runs/001/jest.json",
			},
		},
		{
			name:    "does not add runInBand if maxWorkers specified",
			runDir:  "/tmp/runs/001",
			userCmd: []string{"npx", "jest", "--maxWorkers=4"},
			wantArgs: []string{
				"npx", "jest", "--maxWorkers=4",
				"--json", "--outputFile", "/tmp/runs/001/jest.json",
			},
		},
		{
			name:    "does not add runInBand if -w specified",
			runDir:  "/tmp/runs/001",
			userCmd: []string{"npx", "jest", "-w", "2"},
			wantArgs: []string{
				"npx", "jest", "-w", "2",
				"--json", "--outputFile", "/tmp/runs/001/jest.json",
			},
		},
		{
			name:    "does not add runInBand if -i specified",
			runDir:  "/tmp/runs/001",
			userCmd: []string{"npx", "jest", "-i"},
			wantArgs: []string{
				"npx", "jest", "-i",
				"--json", "--outputFile", "/tmp/runs/001/jest.json",
			},
		},
		{
			name:     "empty command returns nil",
			runDir:   "/tmp/runs/001",
			userCmd:  []string{},
			wantArgs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapter.BuildCommand(tt.runDir, tt.userCmd)

			if len(got) != len(tt.wantArgs) {
				t.Fatalf("BuildCommand() returned %d args, want %d\ngot:  %v\nwant: %v",
					len(got), len(tt.wantArgs), got, tt.wantArgs)
			}

			for i := range got {
				if got[i] != tt.wantArgs[i] {
					t.Errorf("BuildCommand()[%d] = %q, want %q", i, got[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestExpectedArtifact(t *testing.T) {
	adapter := New()
	runDir := "/tmp/flakehunt/runs/001"

	got := adapter.ExpectedArtifact(runDir)
	want := "/tmp/flakehunt/runs/001/jest.json"

	if got != want {
		t.Errorf("ExpectedArtifact() = %q, want %q", got, want)
	}
}

func TestParse(t *testing.T) {
	adapter := New()

	tests := []struct {
		name        string
		fixture     string
		wantTests   []model.TestResult
		wantErr     bool
		errContains string
	}{
		{
			name:    "passing tests",
			fixture: "passing.json",
			wantTests: []model.TestResult{
				{
					TestID:   "/project/src/math.test.js::math add adds two numbers",
					Outcome:  model.OutcomePass,
					Duration: 5 * time.Millisecond,
				},
				{
					TestID:   "/project/src/math.test.js::math subtract subtracts two numbers",
					Outcome:  model.OutcomePass,
					Duration: 3 * time.Millisecond,
				},
			},
		},
		{
			name:    "failing tests",
			fixture: "failing.json",
			wantTests: []model.TestResult{
				{
					TestID:         "/project/src/api.test.js::api fetchData handles errors",
					Outcome:        model.OutcomeFail,
					Duration:       150 * time.Millisecond,
					FailureMessage: "Error: expect(received).toBe(expected)\n\nExpected: \"error\"\nReceived: \"success\"",
				},
				{
					TestID:   "/project/src/api.test.js::api fetchData returns data",
					Outcome:  model.OutcomePass,
					Duration: 100 * time.Millisecond,
				},
			},
		},
		{
			name:    "skipped tests",
			fixture: "skipped.json",
			wantTests: []model.TestResult{
				{
					TestID:   "/project/src/feature.test.js::feature disabled functionality is disabled",
					Outcome:  model.OutcomeSkip,
					Duration: 0,
				},
				{
					TestID:   "/project/src/feature.test.js::feature enabled functionality works",
					Outcome:  model.OutcomePass,
					Duration: 10 * time.Millisecond,
				},
				{
					TestID:   "/project/src/feature.test.js::feature todo functionality needs implementation",
					Outcome:  model.OutcomeSkip,
					Duration: 0,
				},
			},
		},
		{
			name:        "malformed JSON",
			fixture:     "malformed.json",
			wantErr:     true,
			errContains: "invalid JSON",
		},
		{
			name:        "missing file path",
			fixture:     "missing_name.json",
			wantErr:     true,
			errContains: "testResults[0].name is missing or empty",
		},
		{
			name:        "missing fullName",
			fixture:     "missing_fullname.json",
			wantErr:     true,
			errContains: "testResults[0].assertionResults[0].fullName is missing or empty",
		},
		{
			name:        "missing status",
			fixture:     "missing_status.json",
			wantErr:     true,
			errContains: "testResults[0].assertionResults[0].status is missing or empty",
		},
		{
			name:        "unknown status",
			fixture:     "unknown_status.json",
			wantErr:     true,
			errContains: "has unknown value",
		},
		{
			name:      "empty test results",
			fixture:   "empty_results.json",
			wantTests: []model.TestResult{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory with fixture
			tmpDir := t.TempDir()
			fixturePath := filepath.Join("testdata", tt.fixture)
			fixtureData, err := os.ReadFile(fixturePath)
			if err != nil {
				t.Fatalf("Failed to read fixture %s: %v", fixturePath, err)
			}

			destPath := filepath.Join(tmpDir, "jest.json")
			if err := os.WriteFile(destPath, fixtureData, 0644); err != nil {
				t.Fatalf("Failed to write fixture: %v", err)
			}

			got, err := adapter.Parse(tmpDir)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse() expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Parse() error = %q, want error containing %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("Parse() unexpected error: %v", err)
			}

			if len(got.Tests) != len(tt.wantTests) {
				t.Fatalf("Parse() returned %d tests, want %d\ngot:  %+v\nwant: %+v",
					len(got.Tests), len(tt.wantTests), got.Tests, tt.wantTests)
			}

			for i := range got.Tests {
				gotTest := got.Tests[i]
				wantTest := tt.wantTests[i]

				if gotTest.TestID != wantTest.TestID {
					t.Errorf("Test[%d].TestID = %q, want %q", i, gotTest.TestID, wantTest.TestID)
				}
				if gotTest.Outcome != wantTest.Outcome {
					t.Errorf("Test[%d].Outcome = %q, want %q", i, gotTest.Outcome, wantTest.Outcome)
				}
				if gotTest.Duration != wantTest.Duration {
					t.Errorf("Test[%d].Duration = %v, want %v", i, gotTest.Duration, wantTest.Duration)
				}
				if wantTest.FailureMessage != "" && gotTest.FailureMessage != wantTest.FailureMessage {
					t.Errorf("Test[%d].FailureMessage = %q, want %q", i, gotTest.FailureMessage, wantTest.FailureMessage)
				}
			}
		})
	}
}

func TestParseFileErrors(t *testing.T) {
	adapter := New()

	t.Run("missing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		_, err := adapter.Parse(tmpDir)

		if err == nil {
			t.Fatal("Parse() expected error for missing file, got nil")
		}

		parseErr, ok := err.(*ParseError)
		if !ok {
			t.Fatalf("Parse() error should be *ParseError, got %T", err)
		}

		if parseErr.File == "" {
			t.Error("ParseError.File should not be empty")
		}
		if parseErr.Action == "" {
			t.Error("ParseError.Action should not be empty (actionable error)")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		tmpDir := t.TempDir()
		destPath := filepath.Join(tmpDir, "jest.json")
		if err := os.WriteFile(destPath, []byte{}, 0644); err != nil {
			t.Fatalf("Failed to write empty file: %v", err)
		}

		_, err := adapter.Parse(tmpDir)

		if err == nil {
			t.Fatal("Parse() expected error for empty file, got nil")
		}

		if !strings.Contains(err.Error(), "empty") {
			t.Errorf("Parse() error should mention 'empty', got: %v", err)
		}
	})
}

func TestParseDeterministicOrder(t *testing.T) {
	adapter := New()
	tmpDir := t.TempDir()

	// Use fixture with multiple tests to verify stable sorting
	fixturePath := filepath.Join("testdata", "passing.json")
	fixtureData, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("Failed to read fixture: %v", err)
	}

	destPath := filepath.Join(tmpDir, "jest.json")
	if err := os.WriteFile(destPath, fixtureData, 0644); err != nil {
		t.Fatalf("Failed to write fixture: %v", err)
	}

	// Parse multiple times and verify same order
	var firstOrder []string
	for i := 0; i < 5; i++ {
		result, err := adapter.Parse(tmpDir)
		if err != nil {
			t.Fatalf("Parse() failed: %v", err)
		}

		var testIDs []string
		for _, test := range result.Tests {
			testIDs = append(testIDs, test.TestID)
		}

		if i == 0 {
			firstOrder = testIDs
		} else {
			for j, id := range testIDs {
				if id != firstOrder[j] {
					t.Errorf("Parse() run %d: test order differs at index %d: got %q, want %q",
						i, j, id, firstOrder[j])
				}
			}
		}
	}
}
