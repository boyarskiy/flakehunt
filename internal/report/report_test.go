package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boyarskiy/flakehunt/internal/model"
)

// fixtureReport creates a sample report for testing.
func fixtureReport() *model.Report {
	return &model.Report{
		Tool:         "jest",
		Target:       "src/components/Button.test.tsx",
		RunsExecuted: 10,
		FlakyCount:   2,
		StableCount:  1,
		DetFailCount: 0,
		Tests: []model.AggregatedTest{
			{
				TestID:         "src/components/Button.test.tsx::Button should render correctly",
				PassCount:      10,
				FailCount:      0,
				SkipCount:      0,
				TotalRuns:      10,
				AvgDuration:    50 * time.Millisecond,
				Classification: model.ClassificationStable,
				FlakeRate:      0,
				WastedTime:     0,
			},
			{
				TestID:         "src/components/Button.test.tsx::Button should handle click",
				PassCount:      7,
				FailCount:      3,
				SkipCount:      0,
				TotalRuns:      10,
				AvgDuration:    100 * time.Millisecond,
				Classification: model.ClassificationFlaky,
				FlakeRate:      0.3,
				WastedTime:     300 * time.Millisecond,
				FailureEvidence: []model.FailureEvidence{
					{RunIndex: 2, Excerpt: "Unable to find element with text 'Click me'", Signature: model.SignatureSelector},
					{RunIndex: 5, Excerpt: "Unable to find element with text 'Click me'", Signature: model.SignatureSelector},
					{RunIndex: 8, Excerpt: "Timeout waiting for element", Signature: model.SignatureTimeout},
				},
			},
			{
				TestID:         "src/components/Button.test.tsx::Button should submit form",
				PassCount:      5,
				FailCount:      5,
				SkipCount:      0,
				TotalRuns:      10,
				AvgDuration:    200 * time.Millisecond,
				Classification: model.ClassificationFlaky,
				FlakeRate:      0.5,
				WastedTime:     1000 * time.Millisecond,
				FailureEvidence: []model.FailureEvidence{
					{RunIndex: 1, Excerpt: "Network error: ECONNREFUSED", Signature: model.SignatureNetwork},
					{RunIndex: 3, Excerpt: "Network error: ECONNREFUSED", Signature: model.SignatureNetwork},
					{RunIndex: 4, Excerpt: "Network error: ECONNREFUSED", Signature: model.SignatureNetwork},
					{RunIndex: 7, Excerpt: "Network error: ECONNREFUSED", Signature: model.SignatureNetwork},
					{RunIndex: 9, Excerpt: "Network error: ECONNREFUSED", Signature: model.SignatureNetwork},
				},
			},
		},
		TopFlakes: []model.AggregatedTest{
			{
				TestID:         "src/components/Button.test.tsx::Button should submit form",
				PassCount:      5,
				FailCount:      5,
				SkipCount:      0,
				TotalRuns:      10,
				AvgDuration:    200 * time.Millisecond,
				Classification: model.ClassificationFlaky,
				FlakeRate:      0.5,
				WastedTime:     1000 * time.Millisecond,
				FailureEvidence: []model.FailureEvidence{
					{RunIndex: 1, Excerpt: "Network error: ECONNREFUSED", Signature: model.SignatureNetwork},
					{RunIndex: 3, Excerpt: "Network error: ECONNREFUSED", Signature: model.SignatureNetwork},
					{RunIndex: 4, Excerpt: "Network error: ECONNREFUSED", Signature: model.SignatureNetwork},
					{RunIndex: 7, Excerpt: "Network error: ECONNREFUSED", Signature: model.SignatureNetwork},
					{RunIndex: 9, Excerpt: "Network error: ECONNREFUSED", Signature: model.SignatureNetwork},
				},
			},
			{
				TestID:         "src/components/Button.test.tsx::Button should handle click",
				PassCount:      7,
				FailCount:      3,
				SkipCount:      0,
				TotalRuns:      10,
				AvgDuration:    100 * time.Millisecond,
				Classification: model.ClassificationFlaky,
				FlakeRate:      0.3,
				WastedTime:     300 * time.Millisecond,
				FailureEvidence: []model.FailureEvidence{
					{RunIndex: 2, Excerpt: "Unable to find element with text 'Click me'", Signature: model.SignatureSelector},
					{RunIndex: 5, Excerpt: "Unable to find element with text 'Click me'", Signature: model.SignatureSelector},
					{RunIndex: 8, Excerpt: "Timeout waiting for element", Signature: model.SignatureTimeout},
				},
			},
		},
		SignatureSummary: map[string]int{
			"NETWORK":  5,
			"SELECTOR": 2,
			"TIMEOUT":  1,
		},
	}
}

// TestTerminalOutputGolden is a golden test for terminal output.
func TestTerminalOutputGolden(t *testing.T) {
	report := fixtureReport()

	var buf bytes.Buffer
	cfg := &TerminalConfig{
		Writer: &buf,
		TopN:   5,
	}

	err := RenderTerminal(cfg, report, ".flakehunt/latest")
	if err != nil {
		t.Fatalf("RenderTerminal failed: %v", err)
	}

	got := buf.String()
	goldenPath := filepath.Join("testdata", "terminal_output.golden")

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		err := os.MkdirAll(filepath.Dir(goldenPath), 0755)
		if err != nil {
			t.Fatalf("failed to create testdata dir: %v", err)
		}
		err = os.WriteFile(goldenPath, []byte(got), 0644)
		if err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Logf("Updated golden file: %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v\nRun with UPDATE_GOLDEN=1 to create it", goldenPath, err)
	}

	if got != string(want) {
		t.Errorf("terminal output mismatch.\n\nGot:\n%s\n\nWant:\n%s", got, string(want))
	}
}

// TestMarkdownOutputGolden is a golden test for markdown output.
func TestMarkdownOutputGolden(t *testing.T) {
	report := fixtureReport()

	got := RenderMarkdown(report)
	goldenPath := filepath.Join("testdata", "markdown_output.golden")

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		err := os.MkdirAll(filepath.Dir(goldenPath), 0755)
		if err != nil {
			t.Fatalf("failed to create testdata dir: %v", err)
		}
		err = os.WriteFile(goldenPath, []byte(got), 0644)
		if err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Logf("Updated golden file: %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v\nRun with UPDATE_GOLDEN=1 to create it", goldenPath, err)
	}

	if got != string(want) {
		t.Errorf("markdown output mismatch.\n\nGot:\n%s\n\nWant:\n%s", got, string(want))
	}
}

// TestJSONOutputStable tests that JSON output is stable.
func TestJSONOutputStable(t *testing.T) {
	report := fixtureReport()

	// Marshal twice
	data1, err := MarshalJSON(report)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	data2, err := MarshalJSON(report)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	if string(data1) != string(data2) {
		t.Error("JSON output is not stable across marshaling")
	}
}

// TestJSONRoundTrip tests that JSON can be unmarshaled back.
func TestJSONRoundTrip(t *testing.T) {
	report := fixtureReport()

	data, err := MarshalJSON(report)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var decoded model.Report
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Check key fields
	if decoded.Tool != report.Tool {
		t.Errorf("Tool = %s, want %s", decoded.Tool, report.Tool)
	}
	if decoded.Target != report.Target {
		t.Errorf("Target = %s, want %s", decoded.Target, report.Target)
	}
	if decoded.RunsExecuted != report.RunsExecuted {
		t.Errorf("RunsExecuted = %d, want %d", decoded.RunsExecuted, report.RunsExecuted)
	}
	if decoded.FlakyCount != report.FlakyCount {
		t.Errorf("FlakyCount = %d, want %d", decoded.FlakyCount, report.FlakyCount)
	}
}

// TestFormatDuration tests duration formatting.
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{500 * time.Millisecond, "500ms"},
		{1500 * time.Millisecond, "1.5s"},
		{65 * time.Second, "1.1m"},
		{0, "0ms"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatDuration(tt.input)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

// TestTruncateForTerminal tests excerpt truncation.
func TestTruncateForTerminal(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"this is a longer string", 10, "this is..."},
		{"multi\nline\ntext", 20, "multi line text"},
		{"", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncateForTerminal(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateForTerminal(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}

// TestEmptyReport tests handling of empty reports.
func TestEmptyReport(t *testing.T) {
	report := &model.Report{
		Tool:         "jest",
		Target:       "test.js",
		RunsExecuted: 0,
		Tests:        []model.AggregatedTest{},
		TopFlakes:    []model.AggregatedTest{},
	}

	// Should not panic on rendering
	var buf bytes.Buffer
	cfg := &TerminalConfig{Writer: &buf, TopN: 5}
	err := RenderTerminal(cfg, report, ".flakehunt/latest")
	if err != nil {
		t.Errorf("RenderTerminal failed: %v", err)
	}

	md := RenderMarkdown(report)
	if !strings.Contains(md, "No flaky tests detected") {
		t.Error("Markdown should indicate no flaky tests")
	}
}

// TestEscapeMarkdown tests markdown escaping.
func TestEscapeMarkdown(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal text", "normal text"},
		{"text|with|pipes", "text\\|with\\|pipes"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeMarkdown(tt.input)
			if result != tt.expected {
				t.Errorf("escapeMarkdown(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestSortedSignatures tests signature sorting.
func TestSortedSignatures(t *testing.T) {
	summary := map[string]int{
		"TIMEOUT":  3,
		"NETWORK":  5,
		"SELECTOR": 3,
	}

	result := sortedSignatures(summary)

	// NETWORK first (5), then SELECTOR and TIMEOUT (both 3) sorted alphabetically
	expected := []string{"NETWORK", "SELECTOR", "TIMEOUT"}
	for i, exp := range expected {
		if result[i].Name != exp {
			t.Errorf("sorted[%d].Name = %s, want %s", i, result[i].Name, exp)
		}
	}
}
