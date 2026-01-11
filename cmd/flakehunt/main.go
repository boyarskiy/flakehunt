// Package main is the entry point for the flakehunt CLI.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/boyarskiy/flakehunt/internal/adapters/cypress"
	"github.com/boyarskiy/flakehunt/internal/adapters/jest"
	"github.com/boyarskiy/flakehunt/internal/classify"
	"github.com/boyarskiy/flakehunt/internal/model"
	"github.com/boyarskiy/flakehunt/internal/report"
	"github.com/boyarskiy/flakehunt/internal/runner"
)

const (
	// Exit codes per spec
	exitSuccess    = 0
	exitError      = 1
	exitFlakeFound = 2
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	// Handle help and version before flag parsing
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			printUsage()
			return exitSuccess
		case "-v", "--version", "version":
			fmt.Println("flakehunt v1.0.0")
			return exitSuccess
		}
	}

	// Define flags
	fs := flag.NewFlagSet("flakehunt", flag.ContinueOnError)
	cfg := &cliConfig{}
	fs.IntVar(&cfg.runs, "runs", 0, "Number of repetitions (required)")
	fs.DurationVar(&cfg.timeout, "timeout", 0, "Max total runtime (e.g., \"5m\", \"1h\")")
	fs.StringVar(&cfg.outDir, "out", ".flakehunt", "Output directory")
	fs.IntVar(&cfg.keepRuns, "keep-runs", 0, "Number of run directories to keep (0 = keep all)")
	fs.BoolVar(&cfg.jsonOutput, "json", false, "Print report JSON to stdout")
	fs.BoolVar(&cfg.failOnFlake, "fail-on-flake", true, "Exit with code 2 if flakes detected")
	fs.StringVar(&cfg.target, "target", "", "Target description (for reporting)")

	// Find the -- separator
	cmdIdx := findSeparator(args)
	if cmdIdx == -1 {
		fmt.Fprintln(os.Stderr, "Error: test command required after --")
		fmt.Fprintln(os.Stderr, "Usage: flakehunt [flags] -- <test command>")
		return exitError
	}

	// Parse flags (everything before --)
	if err := fs.Parse(args[:cmdIdx]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitError
	}

	// Get user command (everything after --)
	userCmd := args[cmdIdx+1:]
	if len(userCmd) == 0 {
		fmt.Fprintln(os.Stderr, "Error: test command required after --")
		return exitError
	}

	// Validate required flags
	if cfg.runs <= 0 {
		fmt.Fprintln(os.Stderr, "Error: --runs is required and must be a positive integer")
		return exitError
	}

	// Auto-detect tool from command
	tool, adapter, err := detectTool(userCmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitError
	}

	return execute(cfg, tool, adapter, userCmd)
}

// detectTool analyzes the command to determine which test tool is being used.
func detectTool(cmd []string) (model.Tool, model.Adapter, error) {
	cmdStr := strings.ToLower(strings.Join(cmd, " "))

	if strings.Contains(cmdStr, "jest") {
		return model.ToolJest, jest.New(), nil
	}
	if strings.Contains(cmdStr, "cypress") {
		return model.ToolCypress, cypress.New(), nil
	}

	return "", nil, fmt.Errorf("could not detect test tool from command %q. Ensure command contains 'jest' or 'cypress'", strings.Join(cmd, " "))
}

// cliConfig holds the parsed CLI flags.
type cliConfig struct {
	runs        int
	timeout     time.Duration
	outDir      string
	keepRuns    int
	jsonOutput  bool
	failOnFlake bool
	target      string
}

func execute(cfg *cliConfig, tool model.Tool, adapter model.Adapter, userCmd []string) int {
	// Set up context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nInterrupted, stopping...")
		cancel()
	}()

	// Build runner config
	runnerCfg := &runner.Config{
		Runs:     cfg.runs,
		Timeout:  cfg.timeout,
		OutDir:   cfg.outDir,
		KeepRuns: cfg.keepRuns,
		Tool:     tool,
		Command:  userCmd,
		Adapter:  adapter,
	}

	// Execute runs
	fmt.Printf("Running %d iterations with %s...\n\n", cfg.runs, tool)
	result, err := runner.Run(ctx, runnerCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitError
	}

	// Convert runner results to model.RunResult for aggregation
	var runResults []model.RunResult
	for _, rr := range result.RunResults {
		if rr != nil {
			runResults = append(runResults, *rr)
		}
	}

	// Aggregate and classify
	aggregatedTests := classify.Aggregate(runResults)

	// Build report
	target := cfg.target
	if target == "" {
		target = fmt.Sprintf("%v", userCmd)
	}

	rpt := buildReport(string(tool), target, result.RunsExecuted, aggregatedTests)

	// Write reports
	if err := report.WriteJSON(result.LatestDir, rpt); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write JSON report: %v\n", err)
	}

	if err := report.WriteMarkdown(result.LatestDir, rpt); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write Markdown report: %v\n", err)
	}

	// Render terminal output
	termCfg := report.DefaultTerminalConfig(os.Stdout)
	if err := report.RenderTerminal(termCfg, rpt, result.LatestDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to render terminal output: %v\n", err)
	}

	// Print JSON to stdout if requested
	if cfg.jsonOutput {
		data, err := report.MarshalJSON(rpt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to marshal JSON: %v\n", err)
		} else {
			fmt.Println(string(data))
		}
	}

	// Determine exit code
	if cfg.failOnFlake && rpt.FlakyCount > 0 {
		return exitFlakeFound
	}

	return exitSuccess
}

func buildReport(tool, target string, runsExecuted int, tests []model.AggregatedTest) *model.Report {
	flakyCount := 0
	stableCount := 0
	detFailCount := 0

	for _, t := range tests {
		switch t.Classification {
		case model.ClassificationFlaky:
			flakyCount++
		case model.ClassificationStable:
			stableCount++
		case model.ClassificationDeterministicFail:
			detFailCount++
		}
	}

	// Get top flakes
	topFlakes := classify.TopFlakes(tests, 10)

	// Get signature summary
	signatureSummary := classify.SignatureSummary(tests)

	return &model.Report{
		Tool:             tool,
		Target:           target,
		RunsExecuted:     runsExecuted,
		FlakyCount:       flakyCount,
		StableCount:      stableCount,
		DetFailCount:     detFailCount,
		Tests:            tests,
		TopFlakes:        topFlakes,
		SignatureSummary: signatureSummary,
	}
}

func findSeparator(args []string) int {
	for i, arg := range args {
		if arg == "--" {
			return i
		}
	}
	return -1
}

func printUsage() {
	fmt.Println(`flakehunt - Detect flaky tests by running them repeatedly

Usage:
  flakehunt [flags] -- <test command>

The test tool (Jest or Cypress) is auto-detected from the command.

Flags:
  --runs <n>        Number of repetitions (required)
  --timeout <dur>   Max total runtime (e.g., "5m", "1h")
  --out <path>      Output directory (default: .flakehunt)
  --keep-runs <n>   Number of run directories to keep (0 = keep all)
  --json            Print report JSON to stdout
  --fail-on-flake   Exit with code 2 if flakes detected (default: true)
  --target <desc>   Target description for reporting

Examples:
  flakehunt --runs 10 -- npx jest src/utils.test.ts
  flakehunt --runs 5 -- npx cypress run --spec "cypress/e2e/login.cy.js"
  flakehunt --runs 20 --timeout 5m -- npm test

Exit codes:
  0  No flakes detected
  1  Tool error
  2  Flaky tests detected (when --fail-on-flake is true)`)
}
