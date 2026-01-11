// Package report implements reporting for flakehunt.
package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/boyarskiy/flakehunt/internal/model"
)

// TerminalConfig holds configuration for terminal output.
type TerminalConfig struct {
	Writer  io.Writer
	TopN    int  // Number of top flakes to show (default: 5)
	Verbose bool // Show additional details
}

// DefaultTerminalConfig returns the default terminal configuration.
func DefaultTerminalConfig(w io.Writer) *TerminalConfig {
	return &TerminalConfig{
		Writer: w,
		TopN:   5,
	}
}

// RenderTerminal writes the terminal summary to the configured writer.
func RenderTerminal(cfg *TerminalConfig, report *model.Report, artifactPath string) error {
	if cfg.Writer == nil {
		return fmt.Errorf("writer is required")
	}
	if report == nil {
		return fmt.Errorf("report is required")
	}

	w := cfg.Writer
	topN := cfg.TopN
	if topN <= 0 {
		topN = 5
	}

	// Header
	fmt.Fprintln(w)
	fmt.Fprintln(w, "=== Flakehunt Report ===")
	fmt.Fprintln(w)

	// Tool and target
	fmt.Fprintf(w, "Tool:   %s\n", report.Tool)
	fmt.Fprintf(w, "Target: %s\n", report.Target)
	fmt.Fprintln(w)

	// Runs executed
	fmt.Fprintf(w, "Runs Executed: %d\n", report.RunsExecuted)
	fmt.Fprintln(w)

	// Counts
	fmt.Fprintln(w, "Test Counts:")
	fmt.Fprintf(w, "  Flaky:              %d\n", report.FlakyCount)
	fmt.Fprintf(w, "  Deterministic Fail: %d\n", report.DetFailCount)
	fmt.Fprintf(w, "  Stable:             %d\n", report.StableCount)
	fmt.Fprintln(w)

	// Top flakes
	if len(report.TopFlakes) > 0 {
		fmt.Fprintln(w, "Top Flakes (by wasted time):")
		displayed := topN
		if len(report.TopFlakes) < displayed {
			displayed = len(report.TopFlakes)
		}
		for i := 0; i < displayed; i++ {
			flake := report.TopFlakes[i]
			fmt.Fprintf(w, "  %d. %s\n", i+1, flake.TestID)
			fmt.Fprintf(w, "     Flake Rate: %.1f%% (%d/%d failed)\n",
				flake.FlakeRate*100, flake.FailCount, flake.TotalRuns)
			fmt.Fprintf(w, "     Wasted Time: %s\n", formatDuration(flake.WastedTime))

			// Show failure evidence
			if len(flake.FailureEvidence) > 0 {
				runIndices := make([]int, 0, len(flake.FailureEvidence))
				for _, ev := range flake.FailureEvidence {
					runIndices = append(runIndices, ev.RunIndex)
				}
				sort.Ints(runIndices)
				fmt.Fprintf(w, "     Failed Runs: %s\n", formatRunIndices(runIndices))

				// Show first excerpt (truncated)
				excerpt := flake.FailureEvidence[0].Excerpt
				if excerpt != "" {
					truncated := truncateForTerminal(excerpt, 80)
					fmt.Fprintf(w, "     Excerpt: %s\n", truncated)
				}
			}
			fmt.Fprintln(w)
		}
	} else if report.FlakyCount == 0 {
		fmt.Fprintln(w, "No flaky tests detected.")
		fmt.Fprintln(w)
	}

	// Signature summary
	if len(report.SignatureSummary) > 0 {
		fmt.Fprintln(w, "Failure Signatures:")
		// Sort signatures for deterministic output
		signatures := sortedSignatures(report.SignatureSummary)
		for _, sig := range signatures {
			fmt.Fprintf(w, "  %s: %d\n", sig.Name, sig.Count)
		}
		fmt.Fprintln(w)
	}

	// Artifact path
	fmt.Fprintf(w, "Artifacts: %s\n", artifactPath)
	fmt.Fprintln(w)

	return nil
}

// signatureCount holds a signature name and its count.
type signatureCount struct {
	Name  string
	Count int
}

// sortedSignatures returns signatures sorted by count (descending), then name (ascending).
func sortedSignatures(summary map[string]int) []signatureCount {
	result := make([]signatureCount, 0, len(summary))
	for name, count := range summary {
		result = append(result, signatureCount{Name: name, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Name < result[j].Name
	})
	return result
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	ms := d.Milliseconds()
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	secs := float64(ms) / 1000.0
	if secs < 60 {
		return fmt.Sprintf("%.1fs", secs)
	}
	mins := secs / 60.0
	return fmt.Sprintf("%.1fm", mins)
}

// formatRunIndices formats a slice of run indices as a comma-separated string.
func formatRunIndices(indices []int) string {
	if len(indices) == 0 {
		return ""
	}
	strs := make([]string, len(indices))
	for i, idx := range indices {
		strs[i] = fmt.Sprintf("%d", idx)
	}
	return strings.Join(strs, ", ")
}

// truncateForTerminal truncates an excerpt to maxLen characters.
func truncateForTerminal(s string, maxLen int) string {
	// Replace newlines with spaces for single-line display
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	// Collapse multiple spaces
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	s = strings.TrimSpace(s)

	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
