// Package classify provides flakiness classification and aggregation logic.
package classify

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/boyarskiy/flakehunt/internal/model"
)

// maxExcerptLen is the maximum length for failure excerpts.
const maxExcerptLen = 200

// signaturePatterns maps failure signatures to their detection patterns.
// Patterns are checked in order; first match wins.
var signaturePatterns = []struct {
	signature model.FailureSignature
	patterns  []*regexp.Regexp
}{
	{
		signature: model.SignatureTimeout,
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)timeout`),
			regexp.MustCompile(`(?i)timed?\s*out`),
			regexp.MustCompile(`(?i)exceeded\s*time`),
		},
	},
	{
		signature: model.SignatureSelector,
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)selector`),
			regexp.MustCompile(`(?i)element\s*not\s*found`),
			regexp.MustCompile(`(?i)cy\.get`),
		},
	},
	{
		signature: model.SignatureNetwork,
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)network`),
			regexp.MustCompile(`(?i)ECONNREFUSED`),
			regexp.MustCompile(`(?i)fetch\s*failed`),
		},
	},
	{
		signature: model.SignatureDOMDetach,
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)detached`),
			regexp.MustCompile(`(?i)stale\s*element`),
		},
	},
	{
		signature: model.SignatureAssertion,
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\bexpect\b`),
			regexp.MustCompile(`(?i)\bassert`),
			regexp.MustCompile(`(?i)\btoBe\b`),
			regexp.MustCompile(`(?i)\btoEqual\b`),
		},
	},
}

// DetectSignature analyzes a failure message and returns the appropriate signature.
// Patterns are checked in a deterministic order; first match wins.
// Returns SignatureUnknown if no patterns match.
func DetectSignature(failureMessage string) model.FailureSignature {
	if failureMessage == "" {
		return model.SignatureUnknown
	}

	for _, sp := range signaturePatterns {
		for _, pattern := range sp.patterns {
			if pattern.MatchString(failureMessage) {
				return sp.signature
			}
		}
	}

	return model.SignatureUnknown
}

// truncateExcerpt truncates a string to maxExcerptLen, adding ellipsis if truncated.
func truncateExcerpt(s string) string {
	// Normalize whitespace
	s = strings.Join(strings.Fields(s), " ")

	if len(s) <= maxExcerptLen {
		return s
	}
	return s[:maxExcerptLen-3] + "..."
}

// testAggregator accumulates results for a single test across runs.
type testAggregator struct {
	testID          string
	passCount       int
	failCount       int
	skipCount       int
	totalDuration   time.Duration
	durationCount   int // count of runs with valid duration (excludes skips)
	failureEvidence []model.FailureEvidence
}

// Aggregate merges per-test outcomes across runs and returns classified, ranked results.
// The returned slice is sorted by wastedTime descending, with tie-breakers applied.
func Aggregate(runs []model.RunResult) []model.AggregatedTest {
	if len(runs) == 0 {
		return []model.AggregatedTest{}
	}

	// Map testID -> aggregator
	aggregators := make(map[string]*testAggregator)

	// Process each run
	for _, run := range runs {
		for _, test := range run.Tests {
			agg, exists := aggregators[test.TestID]
			if !exists {
				agg = &testAggregator{
					testID:          test.TestID,
					failureEvidence: []model.FailureEvidence{},
				}
				aggregators[test.TestID] = agg
			}

			switch test.Outcome {
			case model.OutcomePass:
				agg.passCount++
				agg.totalDuration += test.Duration
				agg.durationCount++
			case model.OutcomeFail:
				agg.failCount++
				agg.totalDuration += test.Duration
				agg.durationCount++

				// Collect failure evidence
				evidence := model.FailureEvidence{
					RunIndex:  run.RunIndex,
					Excerpt:   truncateExcerpt(test.FailureMessage),
					Signature: DetectSignature(test.FailureMessage),
				}
				agg.failureEvidence = append(agg.failureEvidence, evidence)
			case model.OutcomeSkip:
				agg.skipCount++
				// Skips do not count toward duration average
			}
		}
	}

	// Convert aggregators to AggregatedTest slice
	results := make([]model.AggregatedTest, 0, len(aggregators))
	for _, agg := range aggregators {
		at := buildAggregatedTest(agg, len(runs))
		results = append(results, at)
	}

	// Sort results deterministically
	sortAggregatedTests(results)

	return results
}

// buildAggregatedTest constructs an AggregatedTest from an aggregator.
func buildAggregatedTest(agg *testAggregator, numRuns int) model.AggregatedTest {
	// TotalRuns excludes skips for flake rate calculation
	totalRuns := agg.passCount + agg.failCount

	// Calculate average duration
	var avgDuration time.Duration
	if agg.durationCount > 0 {
		avgDuration = agg.totalDuration / time.Duration(agg.durationCount)
	}

	// Determine classification
	classification := classify(agg.passCount, agg.failCount, totalRuns)

	// Calculate flake rate
	var flakeRate float64
	if totalRuns > 0 {
		flakeRate = float64(agg.failCount) / float64(totalRuns)
	}

	// Calculate wasted time: flakeRate * avgDuration * runs
	// Only meaningful for flaky tests
	var wastedTime time.Duration
	if classification == model.ClassificationFlaky && totalRuns > 0 {
		wastedTime = time.Duration(flakeRate * float64(avgDuration) * float64(numRuns))
	}

	// Sort failure evidence by run index for deterministic output
	sortedEvidence := make([]model.FailureEvidence, len(agg.failureEvidence))
	copy(sortedEvidence, agg.failureEvidence)
	sort.Slice(sortedEvidence, func(i, j int) bool {
		return sortedEvidence[i].RunIndex < sortedEvidence[j].RunIndex
	})

	return model.AggregatedTest{
		TestID:          agg.testID,
		PassCount:       agg.passCount,
		FailCount:       agg.failCount,
		SkipCount:       agg.skipCount,
		TotalRuns:       totalRuns,
		AvgDuration:     avgDuration,
		Classification:  classification,
		FlakeRate:       flakeRate,
		WastedTime:      wastedTime,
		FailureEvidence: sortedEvidence,
	}
}

// classify determines the classification based on pass and fail counts.
func classify(passCount, failCount, totalRuns int) model.Classification {
	if totalRuns == 0 {
		// All skipped or no runs
		return model.ClassificationStable
	}

	if passCount >= 1 && failCount >= 1 {
		return model.ClassificationFlaky
	}

	if failCount == totalRuns {
		return model.ClassificationDeterministicFail
	}

	// passes == totalRuns (stable)
	return model.ClassificationStable
}

// sortAggregatedTests sorts tests by wastedTime descending, with tie-breakers.
// Tie-breakers: FlakeRate desc -> FailCount desc -> TestID asc
func sortAggregatedTests(tests []model.AggregatedTest) {
	sort.Slice(tests, func(i, j int) bool {
		// Primary: wastedTime descending
		if tests[i].WastedTime != tests[j].WastedTime {
			return tests[i].WastedTime > tests[j].WastedTime
		}

		// Tie-breaker 1: FlakeRate descending
		if tests[i].FlakeRate != tests[j].FlakeRate {
			return tests[i].FlakeRate > tests[j].FlakeRate
		}

		// Tie-breaker 2: FailCount descending
		if tests[i].FailCount != tests[j].FailCount {
			return tests[i].FailCount > tests[j].FailCount
		}

		// Tie-breaker 3: TestID ascending (lexicographic)
		return tests[i].TestID < tests[j].TestID
	})
}

// FilterByClassification returns tests matching the given classification.
// The returned slice maintains the original sort order.
func FilterByClassification(tests []model.AggregatedTest, class model.Classification) []model.AggregatedTest {
	result := make([]model.AggregatedTest, 0)
	for _, t := range tests {
		if t.Classification == class {
			result = append(result, t)
		}
	}
	return result
}

// SignatureSummary counts occurrences of each failure signature across all tests.
// Returns a map suitable for the report.
func SignatureSummary(tests []model.AggregatedTest) map[string]int {
	summary := make(map[string]int)
	for _, t := range tests {
		for _, ev := range t.FailureEvidence {
			summary[string(ev.Signature)]++
		}
	}
	return summary
}

// TopFlakes returns up to n flaky tests, sorted by wasted time.
func TopFlakes(tests []model.AggregatedTest, n int) []model.AggregatedTest {
	flaky := FilterByClassification(tests, model.ClassificationFlaky)
	if len(flaky) <= n {
		return flaky
	}
	return flaky[:n]
}
