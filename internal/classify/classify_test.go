package classify

import (
	"testing"
	"time"

	"github.com/boyarskiy/flakehunt/internal/model"
)

func TestDetectSignature(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected model.FailureSignature
	}{
		// TIMEOUT patterns
		{
			name:     "timeout lowercase",
			message:  "Test timeout after 5000ms",
			expected: model.SignatureTimeout,
		},
		{
			name:     "timeout uppercase",
			message:  "TIMEOUT: exceeded limit",
			expected: model.SignatureTimeout,
		},
		{
			name:     "timed out",
			message:  "Request timed out waiting for response",
			expected: model.SignatureTimeout,
		},
		{
			name:     "exceeded time",
			message:  "Operation exceeded time limit",
			expected: model.SignatureTimeout,
		},

		// SELECTOR patterns
		{
			name:     "selector keyword",
			message:  "Failed to find selector '.button'",
			expected: model.SignatureSelector,
		},
		{
			name:     "element not found",
			message:  "Element not found: #submit-btn",
			expected: model.SignatureSelector,
		},
		{
			name:     "cy.get",
			message:  "cy.get() failed to find element",
			expected: model.SignatureSelector,
		},

		// NETWORK patterns
		{
			name:     "network keyword",
			message:  "Network error occurred",
			expected: model.SignatureNetwork,
		},
		{
			name:     "ECONNREFUSED",
			message:  "connect ECONNREFUSED 127.0.0.1:3000",
			expected: model.SignatureNetwork,
		},
		{
			name:     "fetch failed",
			message:  "fetch failed: unable to connect",
			expected: model.SignatureNetwork,
		},

		// DOM_DETACH patterns
		{
			name:     "detached",
			message:  "Element is detached from DOM",
			expected: model.SignatureDOMDetach,
		},
		{
			name:     "stale element",
			message:  "Stale element reference: element is no longer attached",
			expected: model.SignatureDOMDetach,
		},

		// ASSERTION patterns
		{
			name:     "expect keyword",
			message:  "expect(received).toBe(expected)",
			expected: model.SignatureAssertion,
		},
		{
			name:     "assert keyword",
			message:  "AssertionError: expected true to be false",
			expected: model.SignatureAssertion,
		},
		{
			name:     "toBe keyword",
			message:  "Expected value toBe 5, received 3",
			expected: model.SignatureAssertion,
		},
		{
			name:     "toEqual keyword",
			message:  "Expected array toEqual [1, 2, 3]",
			expected: model.SignatureAssertion,
		},

		// UNKNOWN - no match
		{
			name:     "unknown error",
			message:  "Something went wrong",
			expected: model.SignatureUnknown,
		},
		{
			name:     "empty message",
			message:  "",
			expected: model.SignatureUnknown,
		},

		// Priority test: timeout should match before assertion
		{
			name:     "timeout beats assertion",
			message:  "expect(response).timeout exceeded",
			expected: model.SignatureTimeout,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectSignature(tc.message)
			if got != tc.expected {
				t.Errorf("DetectSignature(%q) = %q, want %q", tc.message, got, tc.expected)
			}
		})
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name      string
		passCount int
		failCount int
		totalRuns int
		expected  model.Classification
	}{
		{
			name:      "flaky - passes and fails",
			passCount: 3,
			failCount: 2,
			totalRuns: 5,
			expected:  model.ClassificationFlaky,
		},
		{
			name:      "flaky - one pass one fail",
			passCount: 1,
			failCount: 1,
			totalRuns: 2,
			expected:  model.ClassificationFlaky,
		},
		{
			name:      "deterministic fail - all fail",
			passCount: 0,
			failCount: 5,
			totalRuns: 5,
			expected:  model.ClassificationDeterministicFail,
		},
		{
			name:      "stable - all pass",
			passCount: 5,
			failCount: 0,
			totalRuns: 5,
			expected:  model.ClassificationStable,
		},
		{
			name:      "stable - no runs (all skipped)",
			passCount: 0,
			failCount: 0,
			totalRuns: 0,
			expected:  model.ClassificationStable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classify(tc.passCount, tc.failCount, tc.totalRuns)
			if got != tc.expected {
				t.Errorf("classify(%d, %d, %d) = %q, want %q",
					tc.passCount, tc.failCount, tc.totalRuns, got, tc.expected)
			}
		})
	}
}

func TestAggregate(t *testing.T) {
	t.Run("empty runs", func(t *testing.T) {
		result := Aggregate([]model.RunResult{})
		if len(result) != 0 {
			t.Errorf("expected empty result, got %d tests", len(result))
		}
	})

	t.Run("single test all passing", func(t *testing.T) {
		runs := []model.RunResult{
			{
				RunIndex: 1,
				Tests: []model.TestResult{
					{TestID: "test.js::should work", Outcome: model.OutcomePass, Duration: 100 * time.Millisecond},
				},
			},
			{
				RunIndex: 2,
				Tests: []model.TestResult{
					{TestID: "test.js::should work", Outcome: model.OutcomePass, Duration: 200 * time.Millisecond},
				},
			},
		}

		result := Aggregate(runs)

		if len(result) != 1 {
			t.Fatalf("expected 1 test, got %d", len(result))
		}

		test := result[0]
		if test.TestID != "test.js::should work" {
			t.Errorf("wrong TestID: %q", test.TestID)
		}
		if test.PassCount != 2 {
			t.Errorf("PassCount = %d, want 2", test.PassCount)
		}
		if test.FailCount != 0 {
			t.Errorf("FailCount = %d, want 0", test.FailCount)
		}
		if test.Classification != model.ClassificationStable {
			t.Errorf("Classification = %q, want stable", test.Classification)
		}
		if test.FlakeRate != 0 {
			t.Errorf("FlakeRate = %f, want 0", test.FlakeRate)
		}
		expectedAvg := 150 * time.Millisecond
		if test.AvgDuration != expectedAvg {
			t.Errorf("AvgDuration = %v, want %v", test.AvgDuration, expectedAvg)
		}
	})

	t.Run("flaky test", func(t *testing.T) {
		runs := []model.RunResult{
			{
				RunIndex: 1,
				Tests: []model.TestResult{
					{TestID: "flaky.js::sometimes fails", Outcome: model.OutcomePass, Duration: 100 * time.Millisecond},
				},
			},
			{
				RunIndex: 2,
				Tests: []model.TestResult{
					{TestID: "flaky.js::sometimes fails", Outcome: model.OutcomeFail, Duration: 150 * time.Millisecond, FailureMessage: "timeout exceeded"},
				},
			},
			{
				RunIndex: 3,
				Tests: []model.TestResult{
					{TestID: "flaky.js::sometimes fails", Outcome: model.OutcomePass, Duration: 100 * time.Millisecond},
				},
			},
		}

		result := Aggregate(runs)

		if len(result) != 1 {
			t.Fatalf("expected 1 test, got %d", len(result))
		}

		test := result[0]
		if test.Classification != model.ClassificationFlaky {
			t.Errorf("Classification = %q, want flaky", test.Classification)
		}
		if test.PassCount != 2 {
			t.Errorf("PassCount = %d, want 2", test.PassCount)
		}
		if test.FailCount != 1 {
			t.Errorf("FailCount = %d, want 1", test.FailCount)
		}
		if test.TotalRuns != 3 {
			t.Errorf("TotalRuns = %d, want 3", test.TotalRuns)
		}

		// FlakeRate = 1/3
		expectedFlakeRate := 1.0 / 3.0
		if test.FlakeRate != expectedFlakeRate {
			t.Errorf("FlakeRate = %f, want %f", test.FlakeRate, expectedFlakeRate)
		}

		// Check failure evidence
		if len(test.FailureEvidence) != 1 {
			t.Fatalf("expected 1 failure evidence, got %d", len(test.FailureEvidence))
		}
		ev := test.FailureEvidence[0]
		if ev.RunIndex != 2 {
			t.Errorf("evidence RunIndex = %d, want 2", ev.RunIndex)
		}
		if ev.Signature != model.SignatureTimeout {
			t.Errorf("evidence Signature = %q, want TIMEOUT", ev.Signature)
		}
	})

	t.Run("deterministic failure", func(t *testing.T) {
		runs := []model.RunResult{
			{RunIndex: 1, Tests: []model.TestResult{{TestID: "broken.js::always fails", Outcome: model.OutcomeFail, FailureMessage: "assert failed"}}},
			{RunIndex: 2, Tests: []model.TestResult{{TestID: "broken.js::always fails", Outcome: model.OutcomeFail, FailureMessage: "assert failed"}}},
			{RunIndex: 3, Tests: []model.TestResult{{TestID: "broken.js::always fails", Outcome: model.OutcomeFail, FailureMessage: "assert failed"}}},
		}

		result := Aggregate(runs)

		if len(result) != 1 {
			t.Fatalf("expected 1 test, got %d", len(result))
		}

		test := result[0]
		if test.Classification != model.ClassificationDeterministicFail {
			t.Errorf("Classification = %q, want deterministic_fail", test.Classification)
		}
		if test.FlakeRate != 1.0 {
			t.Errorf("FlakeRate = %f, want 1.0", test.FlakeRate)
		}
		if test.WastedTime != 0 {
			t.Errorf("WastedTime = %v, want 0 (not flaky)", test.WastedTime)
		}
	})

	t.Run("skips do not count", func(t *testing.T) {
		runs := []model.RunResult{
			{RunIndex: 1, Tests: []model.TestResult{{TestID: "skip.js::test", Outcome: model.OutcomePass, Duration: 100 * time.Millisecond}}},
			{RunIndex: 2, Tests: []model.TestResult{{TestID: "skip.js::test", Outcome: model.OutcomeSkip}}},
			{RunIndex: 3, Tests: []model.TestResult{{TestID: "skip.js::test", Outcome: model.OutcomePass, Duration: 100 * time.Millisecond}}},
		}

		result := Aggregate(runs)

		test := result[0]
		if test.PassCount != 2 {
			t.Errorf("PassCount = %d, want 2", test.PassCount)
		}
		if test.SkipCount != 1 {
			t.Errorf("SkipCount = %d, want 1", test.SkipCount)
		}
		if test.TotalRuns != 2 {
			t.Errorf("TotalRuns = %d, want 2 (skips excluded)", test.TotalRuns)
		}
		if test.FlakeRate != 0 {
			t.Errorf("FlakeRate = %f, want 0", test.FlakeRate)
		}
		// Average should only include non-skip runs
		if test.AvgDuration != 100*time.Millisecond {
			t.Errorf("AvgDuration = %v, want 100ms", test.AvgDuration)
		}
	})

	t.Run("all skipped test", func(t *testing.T) {
		runs := []model.RunResult{
			{RunIndex: 1, Tests: []model.TestResult{{TestID: "allskip.js::test", Outcome: model.OutcomeSkip}}},
			{RunIndex: 2, Tests: []model.TestResult{{TestID: "allskip.js::test", Outcome: model.OutcomeSkip}}},
		}

		result := Aggregate(runs)

		test := result[0]
		if test.Classification != model.ClassificationStable {
			t.Errorf("Classification = %q, want stable (all skipped)", test.Classification)
		}
		if test.TotalRuns != 0 {
			t.Errorf("TotalRuns = %d, want 0 (all skipped)", test.TotalRuns)
		}
	})
}

func TestAggregateMultipleTests(t *testing.T) {
	runs := []model.RunResult{
		{
			RunIndex: 1,
			Tests: []model.TestResult{
				{TestID: "a.js::test1", Outcome: model.OutcomePass, Duration: 100 * time.Millisecond},
				{TestID: "b.js::test2", Outcome: model.OutcomeFail, Duration: 200 * time.Millisecond, FailureMessage: "timeout"},
				{TestID: "c.js::test3", Outcome: model.OutcomeFail, Duration: 50 * time.Millisecond, FailureMessage: "assert failed"},
			},
		},
		{
			RunIndex: 2,
			Tests: []model.TestResult{
				{TestID: "a.js::test1", Outcome: model.OutcomePass, Duration: 100 * time.Millisecond},
				{TestID: "b.js::test2", Outcome: model.OutcomePass, Duration: 200 * time.Millisecond},
				{TestID: "c.js::test3", Outcome: model.OutcomeFail, Duration: 50 * time.Millisecond, FailureMessage: "assert failed"},
			},
		},
	}

	result := Aggregate(runs)

	if len(result) != 3 {
		t.Fatalf("expected 3 tests, got %d", len(result))
	}

	// Results should be sorted by wastedTime descending
	// b.js::test2 is flaky with 50% flake rate and 200ms avg duration = highest wasted time
	// a.js::test1 and c.js::test3 are stable/deterministic = 0 wasted time, sorted by other criteria

	if result[0].TestID != "b.js::test2" {
		t.Errorf("first test should be b.js::test2 (highest wasted time), got %q", result[0].TestID)
	}
	if result[0].Classification != model.ClassificationFlaky {
		t.Errorf("b.js::test2 should be flaky, got %q", result[0].Classification)
	}
}

func TestSortingDeterminism(t *testing.T) {
	// Create tests with same wasted time to verify tie-breakers
	runs := []model.RunResult{
		{
			RunIndex: 1,
			Tests: []model.TestResult{
				{TestID: "z.js::test", Outcome: model.OutcomePass, Duration: 100 * time.Millisecond},
				{TestID: "a.js::test", Outcome: model.OutcomePass, Duration: 100 * time.Millisecond},
				{TestID: "m.js::test", Outcome: model.OutcomePass, Duration: 100 * time.Millisecond},
			},
		},
	}

	// Run multiple times to verify deterministic ordering
	for i := 0; i < 10; i++ {
		result := Aggregate(runs)

		if len(result) != 3 {
			t.Fatalf("expected 3 tests, got %d", len(result))
		}

		// All have 0 wasted time and 0 flake rate, so should be sorted by TestID ascending
		expectedOrder := []string{"a.js::test", "m.js::test", "z.js::test"}
		for j, expected := range expectedOrder {
			if result[j].TestID != expected {
				t.Errorf("iteration %d: position %d = %q, want %q", i, j, result[j].TestID, expected)
			}
		}
	}
}

func TestWastedTimeCalculation(t *testing.T) {
	runs := []model.RunResult{
		{RunIndex: 1, Tests: []model.TestResult{{TestID: "test", Outcome: model.OutcomePass, Duration: 100 * time.Millisecond}}},
		{RunIndex: 2, Tests: []model.TestResult{{TestID: "test", Outcome: model.OutcomeFail, Duration: 100 * time.Millisecond, FailureMessage: "err"}}},
	}

	result := Aggregate(runs)
	test := result[0]

	// flakeRate = 0.5, avgDuration = 100ms, runs = 2
	// wastedTime = 0.5 * 100ms * 2 = 100ms
	expectedWasted := 100 * time.Millisecond
	if test.WastedTime != expectedWasted {
		t.Errorf("WastedTime = %v, want %v", test.WastedTime, expectedWasted)
	}
}

func TestTruncateExcerpt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short message unchanged",
			input:    "Short error",
			expected: "Short error",
		},
		{
			name:     "whitespace normalized",
			input:    "Error  with   multiple    spaces",
			expected: "Error with multiple spaces",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateExcerpt(tc.input)
			if got != tc.expected {
				t.Errorf("truncateExcerpt() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestFilterByClassification(t *testing.T) {
	tests := []model.AggregatedTest{
		{TestID: "flaky1", Classification: model.ClassificationFlaky},
		{TestID: "stable1", Classification: model.ClassificationStable},
		{TestID: "flaky2", Classification: model.ClassificationFlaky},
		{TestID: "fail1", Classification: model.ClassificationDeterministicFail},
	}

	flaky := FilterByClassification(tests, model.ClassificationFlaky)
	if len(flaky) != 2 {
		t.Errorf("expected 2 flaky tests, got %d", len(flaky))
	}

	stable := FilterByClassification(tests, model.ClassificationStable)
	if len(stable) != 1 {
		t.Errorf("expected 1 stable test, got %d", len(stable))
	}

	failing := FilterByClassification(tests, model.ClassificationDeterministicFail)
	if len(failing) != 1 {
		t.Errorf("expected 1 deterministic fail test, got %d", len(failing))
	}
}

func TestSignatureSummary(t *testing.T) {
	tests := []model.AggregatedTest{
		{
			TestID: "test1",
			FailureEvidence: []model.FailureEvidence{
				{Signature: model.SignatureTimeout},
				{Signature: model.SignatureTimeout},
			},
		},
		{
			TestID: "test2",
			FailureEvidence: []model.FailureEvidence{
				{Signature: model.SignatureNetwork},
			},
		},
		{
			TestID: "test3",
			FailureEvidence: []model.FailureEvidence{
				{Signature: model.SignatureTimeout},
			},
		},
	}

	summary := SignatureSummary(tests)

	if summary["TIMEOUT"] != 3 {
		t.Errorf("TIMEOUT count = %d, want 3", summary["TIMEOUT"])
	}
	if summary["NETWORK"] != 1 {
		t.Errorf("NETWORK count = %d, want 1", summary["NETWORK"])
	}
}

func TestTopFlakes(t *testing.T) {
	tests := []model.AggregatedTest{
		{TestID: "flaky1", Classification: model.ClassificationFlaky, WastedTime: 300 * time.Millisecond},
		{TestID: "stable1", Classification: model.ClassificationStable},
		{TestID: "flaky2", Classification: model.ClassificationFlaky, WastedTime: 200 * time.Millisecond},
		{TestID: "flaky3", Classification: model.ClassificationFlaky, WastedTime: 100 * time.Millisecond},
	}

	top2 := TopFlakes(tests, 2)
	if len(top2) != 2 {
		t.Fatalf("expected 2 top flakes, got %d", len(top2))
	}

	allFlakes := TopFlakes(tests, 10)
	if len(allFlakes) != 3 {
		t.Errorf("expected 3 flakes (all available), got %d", len(allFlakes))
	}
}

func TestFailureEvidenceSortedByRunIndex(t *testing.T) {
	runs := []model.RunResult{
		{RunIndex: 3, Tests: []model.TestResult{{TestID: "test", Outcome: model.OutcomeFail, FailureMessage: "err3"}}},
		{RunIndex: 1, Tests: []model.TestResult{{TestID: "test", Outcome: model.OutcomeFail, FailureMessage: "err1"}}},
		{RunIndex: 2, Tests: []model.TestResult{{TestID: "test", Outcome: model.OutcomePass}}},
	}

	result := Aggregate(runs)
	test := result[0]

	if len(test.FailureEvidence) != 2 {
		t.Fatalf("expected 2 failure evidences, got %d", len(test.FailureEvidence))
	}

	// Evidence should be sorted by RunIndex ascending
	if test.FailureEvidence[0].RunIndex != 1 {
		t.Errorf("first evidence RunIndex = %d, want 1", test.FailureEvidence[0].RunIndex)
	}
	if test.FailureEvidence[1].RunIndex != 3 {
		t.Errorf("second evidence RunIndex = %d, want 3", test.FailureEvidence[1].RunIndex)
	}
}
