package report

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/boyarskiy/flakehunt/internal/model"
)

// WriteMarkdown writes the report as Markdown to the specified output directory.
// The file is written to <outDir>/report.md
func WriteMarkdown(outDir string, report *model.Report) error {
	if report == nil {
		return fmt.Errorf("report is required")
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", outDir, err)
	}

	path := filepath.Join(outDir, "report.md")

	content := RenderMarkdown(report)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write report to %s: %w", path, err)
	}

	return nil
}

// RenderMarkdown renders the report as a Markdown string.
func RenderMarkdown(report *model.Report) string {
	if report == nil {
		return ""
	}

	var sb strings.Builder

	// Header
	sb.WriteString("# Flakehunt Report\n\n")

	// Summary section
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Tool | %s |\n", report.Tool))
	sb.WriteString(fmt.Sprintf("| Target | %s |\n", escapeMarkdown(report.Target)))
	sb.WriteString(fmt.Sprintf("| Runs Executed | %d |\n", report.RunsExecuted))
	sb.WriteString(fmt.Sprintf("| Flaky Tests | %d |\n", report.FlakyCount))
	sb.WriteString(fmt.Sprintf("| Deterministic Failures | %d |\n", report.DetFailCount))
	sb.WriteString(fmt.Sprintf("| Stable Tests | %d |\n", report.StableCount))
	sb.WriteString("\n")

	// Top Flakes section
	if len(report.TopFlakes) > 0 {
		sb.WriteString("## Top Flakes\n\n")
		sb.WriteString("Ranked by wasted time (flakeRate x avgDuration x runs).\n\n")

		for i, flake := range report.TopFlakes {
			sb.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, escapeMarkdown(flake.TestID)))

			sb.WriteString("| Metric | Value |\n")
			sb.WriteString("|--------|-------|\n")
			sb.WriteString(fmt.Sprintf("| Flake Rate | %.1f%% |\n", flake.FlakeRate*100))
			sb.WriteString(fmt.Sprintf("| Pass Count | %d |\n", flake.PassCount))
			sb.WriteString(fmt.Sprintf("| Fail Count | %d |\n", flake.FailCount))
			sb.WriteString(fmt.Sprintf("| Skip Count | %d |\n", flake.SkipCount))
			sb.WriteString(fmt.Sprintf("| Average Duration | %s |\n", formatDuration(flake.AvgDuration)))
			sb.WriteString(fmt.Sprintf("| Wasted Time | %s |\n", formatDuration(flake.WastedTime)))
			sb.WriteString("\n")

			// Failure evidence
			if len(flake.FailureEvidence) > 0 {
				sb.WriteString("**Failed Runs:**\n\n")

				// Sort evidence by run index
				sortedEvidence := make([]model.FailureEvidence, len(flake.FailureEvidence))
				copy(sortedEvidence, flake.FailureEvidence)
				sort.Slice(sortedEvidence, func(i, j int) bool {
					return sortedEvidence[i].RunIndex < sortedEvidence[j].RunIndex
				})

				for _, ev := range sortedEvidence {
					sb.WriteString(fmt.Sprintf("- **Run %d** [%s]\n", ev.RunIndex, ev.Signature))
					if ev.Excerpt != "" {
						// Indent excerpt
						excerpt := truncateForTerminal(ev.Excerpt, 200)
						sb.WriteString(fmt.Sprintf("  ```\n  %s\n  ```\n", excerpt))
					}
				}
				sb.WriteString("\n")
			}
		}
	} else {
		sb.WriteString("## Top Flakes\n\n")
		sb.WriteString("No flaky tests detected.\n\n")
	}

	// Failure Signatures section
	if len(report.SignatureSummary) > 0 {
		sb.WriteString("## Failure Signatures\n\n")
		sb.WriteString("| Signature | Count |\n")
		sb.WriteString("|-----------|-------|\n")

		signatures := sortedSignatures(report.SignatureSummary)
		for _, sig := range signatures {
			sb.WriteString(fmt.Sprintf("| %s | %d |\n", sig.Name, sig.Count))
		}
		sb.WriteString("\n")
	}

	// All Tests section
	if len(report.Tests) > 0 {
		sb.WriteString("## All Tests\n\n")
		sb.WriteString("| Test ID | Classification | Flake Rate | Pass | Fail | Skip |\n")
		sb.WriteString("|---------|----------------|------------|------|------|------|\n")

		// Sort tests by classification (flaky first, then det_fail, then stable), then by TestID
		sortedTests := make([]model.AggregatedTest, len(report.Tests))
		copy(sortedTests, report.Tests)
		sort.Slice(sortedTests, func(i, j int) bool {
			orderI := classificationOrder(sortedTests[i].Classification)
			orderJ := classificationOrder(sortedTests[j].Classification)
			if orderI != orderJ {
				return orderI < orderJ
			}
			return sortedTests[i].TestID < sortedTests[j].TestID
		})

		for _, test := range sortedTests {
			flakeRateStr := "-"
			if test.Classification == model.ClassificationFlaky {
				flakeRateStr = fmt.Sprintf("%.1f%%", test.FlakeRate*100)
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %d | %d | %d |\n",
				escapeMarkdown(test.TestID),
				test.Classification,
				flakeRateStr,
				test.PassCount,
				test.FailCount,
				test.SkipCount,
			))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// classificationOrder returns a sort order for classifications.
func classificationOrder(c model.Classification) int {
	switch c {
	case model.ClassificationFlaky:
		return 0
	case model.ClassificationDeterministicFail:
		return 1
	case model.ClassificationStable:
		return 2
	default:
		return 3
	}
}

// escapeMarkdown escapes special Markdown characters in a string.
func escapeMarkdown(s string) string {
	// Escape pipe characters which break tables
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}
