package hledger

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/brendanv/float/internal/slogctx"
)

const supportedVersion = "1.52"

// CommandRunner executes a command and returns its stdout, stderr, and error.
// Inject a stub via NewWithRunner for testing.
type CommandRunner func(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)

func execCommandRunner(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	return stdoutBuf.Bytes(), stderrBuf.Bytes(), err
}

type Client struct {
	bin     string
	journal string
	runner  CommandRunner
}

// New validates the binary exists and the version matches supportedVersion.
// Uses the real exec-based runner.
func New(bin, journal string) (*Client, error) {
	resolvedBin, err := exec.LookPath(bin)
	if err != nil {
		return nil, fmt.Errorf("hledger binary not found at %q: %w", bin, err)
	}
	return newClient(resolvedBin, journal, execCommandRunner)
}

// NewWithRunner creates a Client using a custom CommandRunner instead of exec.
// bin is passed as-is to the runner (no LookPath). Useful for testing.
func NewWithRunner(bin, journal string, runner CommandRunner) (*Client, error) {
	return newClient(bin, journal, runner)
}

func newClient(bin, journal string, runner CommandRunner) (*Client, error) {
	c := &Client{bin: bin, journal: journal, runner: runner}

	stdout, _, err := c.run(context.Background(), "--version")
	if err != nil {
		return nil, fmt.Errorf("hledger --version failed: %w", err)
	}

	got, err := parseVersion(string(stdout))
	if err != nil {
		return nil, err
	}

	if got != supportedVersion {
		return nil, fmt.Errorf("unsupported hledger version %q, need %q", got, supportedVersion)
	}

	return c, nil
}

// parseVersion extracts version from "hledger 1.52, linux-x86_64\n".
func parseVersion(output string) (string, error) {
	output = strings.TrimSpace(output)
	parts := strings.Split(output, " ")
	if len(parts) < 2 {
		return "", fmt.Errorf("parseVersion: unexpected output %q", output)
	}
	version := strings.TrimSuffix(parts[1], ",")
	return version, nil
}

// run executes hledger with args via the configured runner.
// At slog.LevelDebug it logs the command and duration on completion.
func (c *Client) run(ctx context.Context, args ...string) (stdout []byte, stderr []byte, err error) {
	start := time.Now()
	stdout, stderr, err = c.runner(ctx, c.bin, args...)
	slogctx.FromContext(ctx).Debug("hledger",
		"args", args,
		"duration_ms", time.Since(start).Milliseconds(),
		slog.Bool("ok", err == nil),
	)
	return
}

// cmdError wraps a runner error with the full command line and any stderr output,
// so callers can see exactly what hledger invocation failed and why.
func cmdError(bin string, args []string, stderr []byte, err error) error {
	cmd := bin
	if len(args) > 0 {
		cmd += " " + strings.Join(args, " ")
	}
	se := strings.TrimSpace(string(stderr))
	if se != "" {
		return fmt.Errorf("%w\ncommand: %s\nstderr: %s", err, cmd, se)
	}
	return fmt.Errorf("%w\ncommand: %s", err, cmd)
}

// RunRaw executes hledger with arbitrary args and returns stdout, stderr, and
// any error. The full command line is included in the returned cmdLine string
// for display purposes. This is an escape hatch for debugging — prefer the
// typed methods (Balances, Register, etc.) for production code.
func (c *Client) RunRaw(ctx context.Context, args ...string) (stdout, stderr []byte, cmdLine string, err error) {
	cmdLine = c.bin
	if len(args) > 0 {
		cmdLine += " " + strings.Join(args, " ")
	}
	stdout, stderr, err = c.run(ctx, args...)
	return
}

// Version returns the hledger version string.
func (c *Client) Version(ctx context.Context) (string, error) {
	stdout, _, err := c.run(ctx, "--version")
	if err != nil {
		return "", err
	}
	return parseVersion(string(stdout))
}

// Check runs `hledger check -f <journal>`.
// Returns nil on exit 0. Returns *CheckError (with full stderr) on exit non-0.
func (c *Client) Check(ctx context.Context) error {
	_, stderr, err := c.run(ctx, "check", "-f", c.journal)
	if err != nil {
		return &CheckError{Output: string(stderr)}
	}
	return nil
}

// Balances runs `hledger bal -O json -f <journal> [--depth N] [query...]`.
// depth 0 = no --depth flag.
func (c *Client) Balances(ctx context.Context, depth int, query ...string) (*BalanceReport, error) {
	args := []string{"bal", "-O", "json", "-f", c.journal}
	if depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", depth))
	}
	args = append(args, query...)

	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger bal: %w", err))
	}

	return parseBalanceReport(stdout)
}

// Register runs `hledger reg -O json -f <journal> [query...]`.
// Returns flat RegisterRows (one per posting).
func (c *Client) Register(ctx context.Context, query ...string) ([]RegisterRow, error) {
	args := []string{"reg", "-O", "json", "-f", c.journal}
	args = append(args, query...)

	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger reg: %w", err))
	}

	return parseRegisterRows(stdout)
}

// Accounts runs `hledger accounts --types [--tree] -f <journal>`.
// tree=true: returns populated tree. tree=false: flat list with no children.
func (c *Client) Accounts(ctx context.Context, tree bool) ([]*AccountNode, error) {
	args := []string{"accounts", "--types", "-f", c.journal}
	if tree {
		args = append(args, "--tree")
	}

	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger accounts: %w", err))
	}

	if tree {
		return parseAccountsTree(string(stdout))
	}
	return parseAccountsFlat(string(stdout))
}

// PrintText runs `hledger print -f <journalFile>` and returns the plain-text
// output. Used to normalize/canonicalize transaction text before appending to
// real journal files.
func (c *Client) PrintText(ctx context.Context, journalFile string) (string, error) {
	printArgs := []string{"print", "-f", journalFile}
	stdout, stderr, err := c.run(ctx, printArgs...)
	if err != nil {
		return "", cmdError(c.bin, printArgs, stderr, fmt.Errorf("hledger print: %w", err))
	}
	return string(stdout), nil
}

// Transactions runs `hledger print -O json -f <journal> [query...]`.
// Returns parsed transactions.
func (c *Client) Transactions(ctx context.Context, query ...string) ([]Transaction, error) {
	args := []string{"print", "-O", "json", "-f", c.journal}
	args = append(args, query...)

	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger print: %w", err))
	}

	return parseTransactions(stdout)
}

// PrintCSV runs `hledger print -O json --rules-file <rulesFile> -f <csvFile>`.
// Used for import preview — no journal file is needed/written.
func (c *Client) PrintCSV(ctx context.Context, csvFile, rulesFile string) ([]Transaction, error) {
	args := []string{"print", "-O", "json", "--rules-file", rulesFile, "-f", csvFile}

	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger print csv: %w", err))
	}

	return parseTransactions(stdout)
}
