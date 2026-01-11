// Package model defines shared data types for flakehunt.
package model

import "time"

// Outcome represents the result of a single test execution.
type Outcome string

const (
	OutcomePass Outcome = "pass"
	OutcomeFail Outcome = "fail"
	OutcomeSkip Outcome = "skip"
)

// Classification represents the flakiness classification of a test.
type Classification string

const (
	ClassificationFlaky             Classification = "flaky"
	ClassificationStable            Classification = "stable"
	ClassificationDeterministicFail Classification = "deterministic_fail"
)

// FailureSignature represents a categorized failure type.
type FailureSignature string

const (
	SignatureTimeout   FailureSignature = "TIMEOUT"
	SignatureSelector  FailureSignature = "SELECTOR"
	SignatureNetwork   FailureSignature = "NETWORK"
	SignatureDOMDetach FailureSignature = "DOM_DETACH"
	SignatureAssertion FailureSignature = "ASSERTION"
	SignatureUnknown   FailureSignature = "UNKNOWN"
)

// TestResult represents the outcome of a single test in a single run.
type TestResult struct {
	TestID         string        `json:"testId"`
	Outcome        Outcome       `json:"outcome"`
	Duration       time.Duration `json:"duration"`
	FailureMessage string        `json:"failureMessage,omitempty"`
}

// RunResult represents the parsed results of a single test run.
type RunResult struct {
	RunIndex int          `json:"runIndex"`
	Tests    []TestResult `json:"tests"`
	Error    string       `json:"error,omitempty"`
}

// FailureEvidence captures details of a specific failure occurrence.
type FailureEvidence struct {
	RunIndex  int              `json:"runIndex"`
	Excerpt   string           `json:"excerpt"`
	Signature FailureSignature `json:"signature"`
}

// AggregatedTest represents the aggregated results of a test across all runs.
type AggregatedTest struct {
	TestID          string            `json:"testId"`
	PassCount       int               `json:"passCount"`
	FailCount       int               `json:"failCount"`
	SkipCount       int               `json:"skipCount"`
	TotalRuns       int               `json:"totalRuns"`
	AvgDuration     time.Duration     `json:"avgDuration"`
	Classification  Classification    `json:"classification"`
	FlakeRate       float64           `json:"flakeRate"`
	WastedTime      time.Duration     `json:"wastedTime"`
	FailureEvidence []FailureEvidence `json:"failureEvidence,omitempty"`
}

// Report is the top-level structure for the JSON report.
type Report struct {
	Tool             string           `json:"tool"`
	Target           string           `json:"target"`
	RunsExecuted     int              `json:"runsExecuted"`
	FlakyCount       int              `json:"flakyCount"`
	StableCount      int              `json:"stableCount"`
	DetFailCount     int              `json:"deterministicFailCount"`
	Tests            []AggregatedTest `json:"tests"`
	TopFlakes        []AggregatedTest `json:"topFlakes"`
	SignatureSummary map[string]int   `json:"signatureSummary"`
}

// Tool represents a supported test tool.
type Tool string

const (
	ToolJest    Tool = "jest"
	ToolCypress Tool = "cypress"
)

// Adapter defines the interface that tool adapters must implement.
type Adapter interface {
	// BuildCommand returns the command arguments with artifact output configured.
	// runDir is the directory where artifacts should be written.
	BuildCommand(runDir string, userCmd []string) []string

	// Parse reads artifacts from runDir and returns test results.
	Parse(runDir string) (*RunResult, error)

	// ExpectedArtifact returns the path to the expected artifact for verification.
	ExpectedArtifact(runDir string) string
}
