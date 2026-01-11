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

## When to Use Flakehunt

**Good use cases:**

- Confirming a suspected flaky test before investigating
- Quick local check before pushing a new test
- Reproducing a flake that occurred in CI
- Measuring flake rate of a known problematic test
- Generating evidence for prioritizing test fixes

**Not ideal for:**

- Discovering unknown flakes across your entire test suite
- Detecting flakes that only occur in CI environments
- Replacing CI-based flake detection systems

## Limitations

Flakehunt runs tests locally, which differs from CI in important ways:

| Factor | Local | CI |
|--------|-------|-----|
| Resources | Fast, dedicated | Shared, constrained |
| Parallelism | Usually sequential | Multiple jobs competing |
| Network | Low latency | Variable latency |
| State | Clean environment | Accumulated test artifacts |
| Timing | Consistent | Variable under load |

**What this means:**

- A test that flakes 5% of the time in CI might pass 100% locally
- Environment-dependent flakes (network, database, timing) may not reproduce
- Low-frequency flakes need many runs (50-100+) to detect reliably

## Tips for Better Detection

**1. Simulate CI parallelism**

```bash
# Run Jest with limited workers like CI does
flakehunt --runs 30 -- npx jest --maxWorkers=2 path/to/test
```

**2. Increase run count for rare flakes**

```bash
# Low-frequency flakes need more iterations
flakehunt --runs 100 --timeout 30m -- npx jest path/to/test
```

**3. Use timeout to bound exploration**

```bash
# Run as many iterations as possible in 5 minutes
flakehunt --runs 1000 --timeout 5m -- npx jest path/to/test
```

## Comparison with CI-Based Detection

| Approach | Pros | Cons |
|----------|------|------|
| **Flakehunt (local)** | Fast feedback, no CI cost, easy setup | May miss environment-dependent flakes |
| **CI retry detection** | Catches real flakes, zero setup | Only detects after failures occur |
| **CI test analytics** | Historical data, high accuracy | Requires CI integration, costs money |

**Recommended workflow:**

1. Use CI test analytics (CircleCI Insights, Datadog, BuildPulse) to identify flaky tests
2. Use flakehunt locally to reproduce and investigate specific flakes
3. Use flakehunt to verify fixes before pushing
