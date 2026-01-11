package cypress

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
		expected []string
	}{
		{
			name:     "basic cypress run",
			runDir:   "/tmp/runs/001",
			userCmd:  []string{"npx", "cypress", "run"},
			expected: []string{"npx", "cypress", "run", "--reporter", "junit", "--reporter-options", "mochaFile=/tmp/runs/001/[hash].xml"},
		},
		{
			name:     "cypress with spec",
			runDir:   "/tmp/runs/002",
			userCmd:  []string{"npx", "cypress", "run", "--spec", "cypress/e2e/login.cy.js"},
			expected: []string{"npx", "cypress", "run", "--spec", "cypress/e2e/login.cy.js", "--reporter", "junit", "--reporter-options", "mochaFile=/tmp/runs/002/[hash].xml"},
		},
		{
			name:     "empty user command",
			runDir:   "/tmp/runs/003",
			userCmd:  []string{},
			expected: nil,
		},
		{
			name:     "cypress with browser option",
			runDir:   "/tmp/runs/004",
			userCmd:  []string{"npx", "cypress", "run", "--browser", "chrome"},
			expected: []string{"npx", "cypress", "run", "--browser", "chrome", "--reporter", "junit", "--reporter-options", "mochaFile=/tmp/runs/004/[hash].xml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adapter.BuildCommand(tt.runDir, tt.userCmd)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d args, got %d: %v", len(tt.expected), len(result), result)
				return
			}

			for i, arg := range tt.expected {
				if result[i] != arg {
					t.Errorf("arg[%d]: expected %q, got %q", i, arg, result[i])
				}
			}
		})
	}
}

func TestBuildCommandDoesNotMutateInput(t *testing.T) {
	adapter := New()
	original := []string{"npx", "cypress", "run"}
	userCmd := make([]string, len(original))
	copy(userCmd, original)

	adapter.BuildCommand("/tmp/runs/001", userCmd)

	for i, arg := range original {
		if userCmd[i] != arg {
			t.Errorf("input was mutated: arg[%d] changed from %q to %q", i, arg, userCmd[i])
		}
	}
}

func TestExpectedArtifact(t *testing.T) {
	adapter := New()
	runDir := "/tmp/runs/001"

	result := adapter.ExpectedArtifact(runDir)
	if result != runDir {
		t.Errorf("expected %q, got %q", runDir, result)
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name          string
		fixture       string
		expectedTests []model.TestResult
		expectError   bool
		errorContains string
	}{
		{
			name:    "passing tests",
			fixture: "passing",
			expectedTests: []model.TestResult{
				{
					TestID:   "cypress/e2e/login.cy.js::should display login form",
					Outcome:  model.OutcomePass,
					Duration: 500 * time.Millisecond,
				},
				{
					TestID:   "cypress/e2e/login.cy.js::should login successfully",
					Outcome:  model.OutcomePass,
					Duration: 1 * time.Second,
				},
			},
		},
		{
			name:    "failing tests",
			fixture: "failing",
			expectedTests: []model.TestResult{
				{
					TestID:   "cypress/e2e/checkout.cy.js::should add item to cart",
					Outcome:  model.OutcomePass,
					Duration: 800 * time.Millisecond,
				},
				{
					TestID:         "cypress/e2e/checkout.cy.js::should complete purchase",
					Outcome:        model.OutcomeFail,
					Duration:       1200 * time.Millisecond,
					FailureMessage: "AssertionError: expected button to be visible",
				},
			},
		},
		{
			name:    "skipped tests",
			fixture: "skipped",
			expectedTests: []model.TestResult{
				{
					TestID:   "cypress/e2e/settings.cy.js::should delete account",
					Outcome:  model.OutcomeSkip,
					Duration: 0,
				},
				{
					TestID:   "cypress/e2e/settings.cy.js::should load settings",
					Outcome:  model.OutcomePass,
					Duration: 500 * time.Millisecond,
				},
				{
					TestID:   "cypress/e2e/settings.cy.js::should update profile",
					Outcome:  model.OutcomePass,
					Duration: 500 * time.Millisecond,
				},
			},
		},
		{
			name:    "retries - last occurrence wins",
			fixture: "retries",
			expectedTests: []model.TestResult{
				{
					TestID:   "cypress/e2e/flaky.cy.js::flaky test",
					Outcome:  model.OutcomePass, // Last occurrence passed
					Duration: 1500 * time.Millisecond,
				},
				{
					TestID:   "cypress/e2e/flaky.cy.js::stable test",
					Outcome:  model.OutcomePass,
					Duration: 500 * time.Millisecond,
				},
			},
		},
		{
			name:    "multiple XML files",
			fixture: "multiple",
			expectedTests: []model.TestResult{
				{
					TestID:   "cypress/e2e/spec1.cy.js::test from spec1",
					Outcome:  model.OutcomePass,
					Duration: 500 * time.Millisecond,
				},
				{
					TestID:   "cypress/e2e/spec2.cy.js::test from spec2",
					Outcome:  model.OutcomePass,
					Duration: 600 * time.Millisecond,
				},
			},
		},
		{
			name:    "single testsuite format",
			fixture: "single_suite",
			expectedTests: []model.TestResult{
				{
					TestID:   "cypress/e2e/single.cy.js::single test",
					Outcome:  model.OutcomePass,
					Duration: 500 * time.Millisecond,
				},
			},
		},
		{
			name:    "error element treated as failure",
			fixture: "error",
			expectedTests: []model.TestResult{
				{
					TestID:         "cypress/e2e/error.cy.js::error test",
					Outcome:        model.OutcomeFail,
					Duration:       500 * time.Millisecond,
					FailureMessage: "RuntimeError: Script error",
				},
			},
		},
		{
			name:          "empty directory",
			fixture:       "empty",
			expectError:   true,
			errorContains: "no XML files found",
		},
		{
			name:          "invalid XML",
			fixture:       "invalid_xml",
			expectError:   true,
			errorContains: "invalid JUnit format",
		},
		{
			name:          "missing classname",
			fixture:       "missing_classname",
			expectError:   true,
			errorContains: "missing classname or name attribute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := New()
			fixtureDir := filepath.Join("testdata", tt.fixture)

			result, err := adapter.Parse(fixtureDir)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errorContains)
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result.Tests) != len(tt.expectedTests) {
				t.Fatalf("expected %d tests, got %d", len(tt.expectedTests), len(result.Tests))
			}

			for i, expected := range tt.expectedTests {
				actual := result.Tests[i]

				if actual.TestID != expected.TestID {
					t.Errorf("test[%d].TestID: expected %q, got %q", i, expected.TestID, actual.TestID)
				}
				if actual.Outcome != expected.Outcome {
					t.Errorf("test[%d].Outcome: expected %q, got %q", i, expected.Outcome, actual.Outcome)
				}
				if actual.Duration != expected.Duration {
					t.Errorf("test[%d].Duration: expected %v, got %v", i, expected.Duration, actual.Duration)
				}
				if expected.FailureMessage != "" && actual.FailureMessage != expected.FailureMessage {
					t.Errorf("test[%d].FailureMessage: expected %q, got %q", i, expected.FailureMessage, actual.FailureMessage)
				}
			}
		})
	}
}

func TestParseDeterministicOutput(t *testing.T) {
	// Run parse multiple times and verify output is identical
	adapter := New()
	fixtureDir := filepath.Join("testdata", "multiple")

	var firstResult *model.RunResult
	for i := 0; i < 10; i++ {
		result, err := adapter.Parse(fixtureDir)
		if err != nil {
			t.Fatalf("parse failed on iteration %d: %v", i, err)
		}

		if firstResult == nil {
			firstResult = result
			continue
		}

		if len(result.Tests) != len(firstResult.Tests) {
			t.Fatalf("iteration %d: test count changed from %d to %d", i, len(firstResult.Tests), len(result.Tests))
		}

		for j, test := range result.Tests {
			if test.TestID != firstResult.Tests[j].TestID {
				t.Errorf("iteration %d: test[%d].TestID changed from %q to %q", i, j, firstResult.Tests[j].TestID, test.TestID)
			}
		}
	}
}

func TestParseRetriesFromMultipleFiles(t *testing.T) {
	// Test that retries across multiple XML files work correctly
	adapter := New()
	fixtureDir := filepath.Join("testdata", "retries_multifile")

	result, err := adapter.Parse(fixtureDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the flaky test
	var flakyTest *model.TestResult
	for i := range result.Tests {
		if result.Tests[i].TestID == "cypress/e2e/flaky.cy.js::flaky test" {
			flakyTest = &result.Tests[i]
			break
		}
	}

	if flakyTest == nil {
		t.Fatal("flaky test not found in results")
	}

	// Last occurrence (from file 002.xml) should win - it passed
	if flakyTest.Outcome != model.OutcomePass {
		t.Errorf("expected flaky test outcome to be pass (last occurrence), got %v", flakyTest.Outcome)
	}
}

// TestMain sets up the test fixtures
func TestMain(m *testing.M) {
	// Create testdata directory and fixtures
	if err := setupTestFixtures(); err != nil {
		panic("failed to setup test fixtures: " + err.Error())
	}

	os.Exit(m.Run())
}

func setupTestFixtures() error {
	baseDir := "testdata"

	fixtures := map[string]map[string]string{
		"passing": {
			"results.xml": `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="cypress/e2e/login.cy.js" tests="2" failures="0" errors="0" skipped="0" time="1.5">
    <testcase name="should display login form" classname="cypress/e2e/login.cy.js" time="0.5"/>
    <testcase name="should login successfully" classname="cypress/e2e/login.cy.js" time="1.0"/>
  </testsuite>
</testsuites>`,
		},
		"failing": {
			"results.xml": `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="cypress/e2e/checkout.cy.js" tests="2" failures="1" errors="0" skipped="0" time="2.0">
    <testcase name="should add item to cart" classname="cypress/e2e/checkout.cy.js" time="0.8"/>
    <testcase name="should complete purchase" classname="cypress/e2e/checkout.cy.js" time="1.2">
      <failure message="AssertionError: expected button to be visible" type="AssertionError">
        AssertionError: expected button to be visible
        at Context.eval (cypress/e2e/checkout.cy.js:15:10)
      </failure>
    </testcase>
  </testsuite>
</testsuites>`,
		},
		"skipped": {
			"results.xml": `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="cypress/e2e/settings.cy.js" tests="3" failures="0" errors="0" skipped="1" time="1.0">
    <testcase name="should load settings" classname="cypress/e2e/settings.cy.js" time="0.5"/>
    <testcase name="should update profile" classname="cypress/e2e/settings.cy.js" time="0.5"/>
    <testcase name="should delete account" classname="cypress/e2e/settings.cy.js" time="0">
      <skipped message="Test skipped"/>
    </testcase>
  </testsuite>
</testsuites>`,
		},
		"retries": {
			"results.xml": `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="cypress/e2e/flaky.cy.js" tests="3" failures="1" errors="0" skipped="0" time="3.0">
    <testcase name="flaky test" classname="cypress/e2e/flaky.cy.js" time="1.0">
      <failure message="Timeout waiting for element" type="TimeoutError">Timeout</failure>
    </testcase>
    <testcase name="stable test" classname="cypress/e2e/flaky.cy.js" time="0.5"/>
    <testcase name="flaky test" classname="cypress/e2e/flaky.cy.js" time="1.5"/>
  </testsuite>
</testsuites>`,
		},
		"multiple": {
			"001_spec1.xml": `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="cypress/e2e/spec1.cy.js" tests="1" failures="0" errors="0" skipped="0" time="0.5">
    <testcase name="test from spec1" classname="cypress/e2e/spec1.cy.js" time="0.5"/>
  </testsuite>
</testsuites>`,
			"002_spec2.xml": `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="cypress/e2e/spec2.cy.js" tests="1" failures="0" errors="0" skipped="0" time="0.6">
    <testcase name="test from spec2" classname="cypress/e2e/spec2.cy.js" time="0.6"/>
  </testsuite>
</testsuites>`,
		},
		"single_suite": {
			"results.xml": `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="cypress/e2e/single.cy.js" tests="1" failures="0" errors="0" skipped="0" time="0.5">
  <testcase name="single test" classname="cypress/e2e/single.cy.js" time="0.5"/>
</testsuite>`,
		},
		"error": {
			"results.xml": `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="cypress/e2e/error.cy.js" tests="1" failures="0" errors="1" skipped="0" time="0.5">
    <testcase name="error test" classname="cypress/e2e/error.cy.js" time="0.5">
      <error message="RuntimeError: Script error" type="RuntimeError">Script crashed</error>
    </testcase>
  </testsuite>
</testsuites>`,
		},
		"empty": {
			".gitkeep": "",
		},
		"invalid_xml": {
			"results.xml": `not valid xml at all {{{`,
		},
		"missing_classname": {
			"results.xml": `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="test" tests="1" failures="0" errors="0" skipped="0" time="0.5">
    <testcase name="test without classname" classname="" time="0.5"/>
  </testsuite>
</testsuites>`,
		},
		"retries_multifile": {
			"001.xml": `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="cypress/e2e/flaky.cy.js" tests="1" failures="1" errors="0" skipped="0" time="1.0">
    <testcase name="flaky test" classname="cypress/e2e/flaky.cy.js" time="1.0">
      <failure message="First attempt failed" type="Error">Failed</failure>
    </testcase>
  </testsuite>
</testsuites>`,
			"002.xml": `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="cypress/e2e/flaky.cy.js" tests="1" failures="0" errors="0" skipped="0" time="1.5">
    <testcase name="flaky test" classname="cypress/e2e/flaky.cy.js" time="1.5"/>
  </testsuite>
</testsuites>`,
		},
	}

	for dir, files := range fixtures {
		dirPath := filepath.Join(baseDir, dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return err
		}

		for filename, content := range files {
			filePath := filepath.Join(dirPath, filename)
			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				return err
			}
		}
	}

	return nil
}
