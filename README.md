# flakehunt

A local CLI tool that detects flaky tests by running them repeatedly and analyzing outcome variance.

## Features

- **Target mode**: Run a specific test file or pattern multiple times
- **Auto-detection**: Automatically detects Jest or Cypress from your test command
- **Flakiness detection**: Classifies tests as flaky, stable, or deterministic failures
- **Failure signatures**: Categorizes failures (TIMEOUT, SELECTOR, NETWORK, DOM_DETACH, ASSERTION)
- **Actionable reports**: Terminal summary, JSON, and Markdown output

## Installation

### From source

```bash
git clone https://github.com/boyarskiy/flakehunt.git
cd flakehunt
go build -o flakehunt ./cmd/flakehunt
mv flakehunt /usr/local/bin/
```

### Using go install

```bash
go install github.com/boyarskiy/flakehunt/cmd/flakehunt@latest
```

## Usage

```bash
flakehunt [flags] -- <test command>
```

The test tool (Jest or Cypress) is automatically detected from your command.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--runs` | required | Number of repetitions |
| `--timeout` | none | Max total runtime (e.g., "5m", "1h") |
| `--out` | `.flakehunt` | Output directory |
| `--keep-runs` | 0 | Run directories to keep (0 = all) |
| `--json` | false | Print JSON report to stdout |
| `--fail-on-flake` | true | Exit code 2 if flakes detected |
| `--target` | none | Target description for reporting |

### Examples

**Jest**
```bash
flakehunt --runs 10 -- npx jest src/utils.test.ts
```

**Cypress**
```bash
flakehunt --runs 5 -- npx cypress run --spec "cypress/e2e/login.cy.js"
```

**With timeout**
```bash
flakehunt --runs 50 --timeout 10m -- npm test
```

## Output

After running, flakehunt produces:
- Terminal summary with top flakes ranked by wasted time
- `.flakehunt/latest/report.json` - machine-readable report
- `.flakehunt/latest/report.md` - human-readable report
- `.flakehunt/latest/runs/` - individual run artifacts

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | No flakes detected |
| 1 | Tool error |
| 2 | Flaky tests detected |
