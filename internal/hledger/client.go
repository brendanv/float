package hledger

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/brendanv/float/internal/slogctx"
)

const supportedVersion = "1.52"

// defaultConcurrency bounds how many hledger processes run at once when the
// caller doesn't configure server.hledger_concurrency. Each invocation parses
// the whole journal, so unbounded concurrency thrashes memory/CPU on small
// hardware under a burst of concurrent RPCs.
const defaultConcurrency = 2

// lowPriorityRetryInterval is how often a low-priority (warmer) invocation
// re-checks for a free semaphore slot instead of queuing behind interactive
// requests.
const lowPriorityRetryInterval = 25 * time.Millisecond

type lowPriorityKey struct{}

// WithLowPriority marks ctx so that hledger invocations made with it never
// queue ahead of normal (interactive) invocations for a concurrency slot.
// internal/warm uses this for background cache-warming passes.
func WithLowPriority(ctx context.Context) context.Context {
	return context.WithValue(ctx, lowPriorityKey{}, true)
}

func isLowPriority(ctx context.Context) bool {
	v, _ := ctx.Value(lowPriorityKey{}).(bool)
	return v
}

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

	semMu       sync.RWMutex
	sem         *semaphore.Weighted
	invocations atomic.Uint64
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
	c.sem = semaphore.NewWeighted(defaultConcurrency)

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

// SetConcurrency changes the number of hledger processes allowed to run at
// once. Intended to be called once at startup from server.hledger_concurrency;
// n <= 0 is ignored (keeps the current limit).
func (c *Client) SetConcurrency(n int) {
	if n <= 0 {
		return
	}
	c.semMu.Lock()
	defer c.semMu.Unlock()
	c.sem = semaphore.NewWeighted(int64(n))
}

// Invocations returns the cumulative count of hledger processes run by this
// client since construction.
func (c *Client) Invocations() uint64 {
	return c.invocations.Load()
}

// acquire reserves a concurrency slot. Normal invocations block in FIFO order
// via the semaphore; low-priority invocations (internal/warm) instead poll
// with TryAcquire so they never queue ahead of an interactive request that
// arrives while they're waiting.
func (c *Client) acquire(ctx context.Context) error {
	c.semMu.RLock()
	sem := c.sem
	c.semMu.RUnlock()

	if !isLowPriority(ctx) {
		return sem.Acquire(ctx, 1)
	}
	for {
		if sem.TryAcquire(1) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(lowPriorityRetryInterval):
		}
	}
}

func (c *Client) release() {
	c.semMu.RLock()
	sem := c.sem
	c.semMu.RUnlock()
	sem.Release(1)
}

// run executes hledger with args via the configured runner, bounded by the
// client's concurrency semaphore. At slog.LevelDebug it logs the command and
// duration on completion.
func (c *Client) run(ctx context.Context, args ...string) (stdout []byte, stderr []byte, err error) {
	if err := c.acquire(ctx); err != nil {
		return nil, nil, err
	}
	defer c.release()

	c.invocations.Add(1)
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

// ErrUnsafeQuery is returned when user-supplied query terms or raw query
// arguments could redirect hledger's input/output files or invoke a command
// that writes, which would bypass the txlock snapshot/check/revert protocol.
var ErrUnsafeQuery = errors.New("hledger: unsafe query argument")

// validateQueryTerms rejects tokens that hledger would parse as command-line
// flags rather than query terms. hledger accepts flags anywhere in argv, so a
// user-supplied query token like --output-file=FILE would otherwise overwrite
// arbitrary files outside the write protocol. No valid hledger query term
// starts with '-' (negation is expressed as not:).
func validateQueryTerms(query []string) error {
	for _, q := range query {
		if strings.HasPrefix(q, "-") {
			return fmt.Errorf("%w: query term %q must not start with '-'", ErrUnsafeQuery, q)
		}
	}
	return nil
}

// runQueryAllowedCommands lists the read-only hledger subcommands permitted
// through RunQuery, which executes client-supplied arguments. Commands that
// write to journal files (add, import, rewrite, ...) must go through the
// typed mutation paths so txlock can snapshot/check/revert.
var runQueryAllowedCommands = map[string]bool{
	"accounts": true, "activity": true,
	"areg": true, "aregister": true,
	"b": true, "bal": true, "balance": true,
	"bs": true, "balancesheet": true,
	"bse": true, "balancesheetequity": true,
	"cf": true, "cashflow": true,
	"check": true, "codes": true, "commodities": true,
	"descriptions": true, "files": true,
	"is": true, "incomestatement": true,
	"notes": true, "payees": true, "prices": true, "print": true,
	"r": true, "reg": true, "register": true,
	"roi": true, "stats": true, "tags": true,
}

// isBlockedRunQueryArg reports whether a RunQuery argument could redirect
// hledger's input or output to another file. -f and -o accept attached values
// (-fFILE, -oFILE), so any token starting with those prefixes is rejected.
func isBlockedRunQueryArg(a string) bool {
	if strings.HasPrefix(a, "--") {
		name, _, _ := strings.Cut(a, "=")
		return name == "--file" || name == "--output-file" || name == "--rules-file"
	}
	return strings.HasPrefix(a, "-f") || strings.HasPrefix(a, "-o")
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

// RunQuery runs hledger with the given shell-like argument string, automatically
// prepending -f <journal>. The argsStr should look like "bal --depth 2 assets"
// without the hledger binary name or the journal flag. Both single- and double-
// quoted tokens are supported. Returns raw stdout, stderr, the full command line
// for display, and the exit error (nil on success).
func (c *Client) RunQuery(ctx context.Context, argsStr string) (stdout, stderr []byte, cmdLine string, err error) {
	userArgs, splitErr := shellSplit(strings.TrimSpace(argsStr))
	if splitErr != nil {
		return nil, nil, "", splitErr
	}
	if len(userArgs) == 0 {
		return nil, nil, "", fmt.Errorf("%w: empty command", ErrUnsafeQuery)
	}
	if !runQueryAllowedCommands[userArgs[0]] {
		return nil, nil, "", fmt.Errorf("%w: %q is not an allowed read-only command", ErrUnsafeQuery, userArgs[0])
	}
	for _, a := range userArgs[1:] {
		if isBlockedRunQueryArg(a) {
			return nil, nil, "", fmt.Errorf("%w: argument %q may redirect hledger input/output", ErrUnsafeQuery, a)
		}
	}
	args := append([]string{"-f", c.journal}, userArgs...)
	cmdLine = c.bin + " " + strings.Join(args, " ")
	stdout, stderr, err = c.run(ctx, args...)
	return
}

// shellSplit splits a string into tokens using shell-like quoting rules.
// Single- and double-quoted regions are treated as single tokens; whitespace
// outside quotes acts as a delimiter. Escape sequences are not supported.
func shellSplit(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inSingle := false
	inDouble := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case (c == ' ' || c == '\t' || c == '\n') && !inSingle && !inDouble:
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unclosed quote in args")
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args, nil
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
	_, stderr, err := c.run(ctx, "check", "--infer-costs", "-f", c.journal)
	if err != nil {
		return &CheckError{Output: string(stderr)}
	}
	return nil
}

// Balances runs `hledger bal -O json -f <journal> [--depth N] [query...]`.
// depth 0 = no --depth flag.
func (c *Client) Balances(ctx context.Context, depth int, query ...string) (*BalanceReport, error) {
	if err := validateQueryTerms(query); err != nil {
		return nil, err
	}
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

// BalancesValued runs `hledger bal -O json --infer-market-prices --value=<valueSpec>
// -f <journal> [query...]`. valueSpec is the argument to --value, e.g. "now,USD"
// or "end,USD". hledger converts each commodity to the target currency using P
// directives from the journal; amounts without a matching price are left unconverted.
// depth 0 = no --depth flag.
func (c *Client) BalancesValued(ctx context.Context, valueSpec string, depth int, query ...string) (*BalanceReport, error) {
	if err := validateQueryTerms(query); err != nil {
		return nil, err
	}
	args := []string{"bal", "-O", "json", "-f", c.journal, "--infer-market-prices", "--value=" + valueSpec}
	if depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", depth))
	}
	args = append(args, query...)

	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger bal --value: %w", err))
	}

	return parseBalanceReport(stdout)
}

// BalancesCost runs `hledger bal -B -O json -f <journal> [query...]`.
// -B converts commodity amounts to their recorded purchase cost in base currency.
// Accounts that lack cost annotations are returned in the original commodity.
// depth 0 = no --depth flag.
func (c *Client) BalancesCost(ctx context.Context, depth int, query ...string) (*BalanceReport, error) {
	if err := validateQueryTerms(query); err != nil {
		return nil, err
	}
	args := []string{"bal", "-B", "-O", "json", "-f", c.journal}
	if depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", depth))
	}
	args = append(args, query...)

	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger bal -B: %w", err))
	}

	return parseBalanceReport(stdout)
}

// PortfolioTimeseries runs `hledger bs --monthly --historical --layout=bare
// --infer-market-prices --value=end,USD -O json -f <journal> [accounts...] [date:begin..]`
// for the given investment account names (not prefixes — pass leaf account names that hold
// non-currency commodities so that pure-cash accounts are excluded from the total).
// accounts must be non-empty; begin is an optional "YYYY-MM-DD" string.
func (c *Client) PortfolioTimeseries(ctx context.Context, accounts []string, begin string) (*BalanceSheetTimeseries, error) {
	args := []string{
		"bs", "-O", "json", "-f", c.journal,
		"--monthly", "--historical", "--layout=bare",
		"--infer-market-prices", "--value=end,USD",
	}
	args = append(args, accounts...)
	if begin != "" {
		args = append(args, "date:"+begin+"..")
	}

	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger bs (portfolio): %w", err))
	}

	return parseBalanceSheetTimeseries(stdout)
}

// PortfolioCostBasisTimeseries runs `hledger bs --monthly --historical --layout=bare
// --cost -O json -f <journal> [accounts...] [date:begin..]`
// using --cost to convert each posting at its transaction cost annotation (@@),
// giving the historical cost basis of the portfolio.
// accounts must be non-empty; begin is an optional "YYYY-MM-DD" string.
func (c *Client) PortfolioCostBasisTimeseries(ctx context.Context, accounts []string, begin string) (*BalanceSheetTimeseries, error) {
	args := []string{
		"bs", "-O", "json", "-f", c.journal,
		"--monthly", "--historical", "--layout=bare",
		"--cost",
	}
	args = append(args, accounts...)
	if begin != "" {
		args = append(args, "date:"+begin+"..")
	}

	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger bs (cost basis): %w", err))
	}

	return parseBalanceSheetTimeseries(stdout)
}

// BalanceSheetTimeseries runs `hledger bs --monthly --historical --layout=bare
// --infer-market-prices --value=end,USD -O json -f <journal> [date:begin..end]`
// and returns per-period asset and liability totals.
// begin and end are optional "YYYY-MM-DD" strings; pass "" to omit.
func (c *Client) BalanceSheetTimeseries(ctx context.Context, begin, end string) (*BalanceSheetTimeseries, error) {
	args := []string{
		"bs", "-O", "json", "-f", c.journal,
		"--monthly", "--historical", "--layout=bare",
		"--infer-market-prices", "--value=end,USD",
	}
	if begin != "" || end != "" {
		args = append(args, "date:"+begin+".."+end)
	}

	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger bs: %w", err))
	}

	return parseBalanceSheetTimeseries(stdout)
}

// IncomeStatementTimeseries runs `hledger is --monthly --tree -O json -f <journal>
// [date:begin..end]` and returns per-account revenue and expense amounts
// grouped by monthly period. --tree includes parent account rows with
// aggregated amounts.
// begin and end are optional "YYYY-MM-DD" strings; pass "" to omit.
func (c *Client) IncomeStatementTimeseries(ctx context.Context, begin, end string) (*IncomeStatementTimeseries, error) {
	args := []string{"is", "-O", "json", "-f", c.journal, "--monthly", "--tree"}
	if begin != "" || end != "" {
		args = append(args, "date:"+begin+".."+end)
	}

	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger is: %w", err))
	}

	return parseIncomeStatementTimeseries(stdout)
}

// Register runs `hledger reg -O json -f <journal> [query...]`.
// Returns flat RegisterRows (one per posting).
func (c *Client) Register(ctx context.Context, query ...string) ([]RegisterRow, error) {
	if err := validateQueryTerms(query); err != nil {
		return nil, err
	}
	args := []string{"reg", "-O", "json", "-f", c.journal}
	args = append(args, query...)

	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger reg: %w", err))
	}

	return parseRegisterRows(stdout)
}

// Aregister runs `hledger areg -O json -f <journal> <account> [query...]`.
// Returns one row per transaction touching the focused account, with a signed
// change amount and running balance pre-computed by hledger.
func (c *Client) Aregister(ctx context.Context, account string, query ...string) ([]AregisterRow, error) {
	// account is a positional query term too — validate it alongside query.
	if err := validateQueryTerms(append([]string{account}, query...)); err != nil {
		return nil, err
	}
	args := []string{"areg", "-O", "json", "-f", c.journal, account}
	args = append(args, query...)

	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger areg: %w", err))
	}

	return parseAregisterRows(stdout)
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

// UnusedAccounts runs `hledger accounts --unused -f <journal>`.
// Returns declared account names that have no postings referencing them.
func (c *Client) UnusedAccounts(ctx context.Context) ([]string, error) {
	args := []string{"accounts", "--unused", "-f", c.journal}
	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger accounts --unused: %w", err))
	}
	var names []string
	for _, line := range strings.Split(string(stdout), "\n") {
		if n := strings.TrimSpace(line); n != "" {
			names = append(names, n)
		}
	}
	return names, nil
}

// UndeclaredAccounts runs `hledger accounts --undeclared -f <journal>`.
// Returns account names used in transactions but not declared with an `account` directive.
// Returns an empty slice if all accounts are declared.
func (c *Client) UndeclaredAccounts(ctx context.Context) ([]string, error) {
	args := []string{"accounts", "--undeclared", "-f", c.journal}
	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger accounts --undeclared: %w", err))
	}
	var names []string
	for _, line := range strings.Split(string(stdout), "\n") {
		if n := strings.TrimSpace(line); n != "" {
			names = append(names, n)
		}
	}
	return names, nil
}

// Tags runs `hledger tags -f <journal>` and returns the list of tag names in use,
// excluding the internal "fid" tag.
func (c *Client) Tags(ctx context.Context) ([]string, error) {
	args := []string{"tags", "-f", c.journal}

	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger tags: %w", err))
	}

	return parseTags(stdout), nil
}

// Payees runs `hledger payees -f <journal> desc:.*[|].*` and returns the list
// of unique payees. The desc:.*[|].* filter restricts output to transactions
// whose description contains a "|" separator, so bare descriptions without an
// explicit payee don't appear in the payees list.
func (c *Client) Payees(ctx context.Context) ([]string, error) {
	args := []string{"payees", "-f", c.journal, `desc:.*[|].*`}

	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger payees: %w", err))
	}

	return parsePayees(stdout), nil
}

// PrintText runs `hledger print -f <journalFile> -I` and returns the plain-text
// output. Used to normalize/canonicalize transaction text before appending to
// real journal files. Balance assertions are ignored (-I) because the temp file
// contains only the draft transaction, not the full journal history; the full
// journal check happens at txlock commit time.
func (c *Client) PrintText(ctx context.Context, journalFile string) (string, error) {
	printArgs := []string{"print", "--infer-costs", "-f", journalFile, "-I"}
	stdout, stderr, err := c.run(ctx, printArgs...)
	if err != nil {
		return "", cmdError(c.bin, printArgs, stderr, fmt.Errorf("hledger print: %w", err))
	}
	return string(stdout), nil
}

// PrintJournal runs `hledger print -f <journal>` and returns the flattened
// journal in hledger's plain-text journal format, with all `include`d files
// (accounts.journal, prices.journal, monthly transaction files) inlined into
// a single document.
func (c *Client) PrintJournal(ctx context.Context) (string, error) {
	args := []string{"print", "-f", c.journal}
	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return "", cmdError(c.bin, args, stderr, fmt.Errorf("hledger print: %w", err))
	}
	return string(stdout), nil
}

// Transactions runs `hledger print -O json -f <journal> [query...]`.
// Returns parsed transactions.
func (c *Client) Transactions(ctx context.Context, query ...string) ([]Transaction, error) {
	if err := validateQueryTerms(query); err != nil {
		return nil, err
	}
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
	args := []string{"print", "--infer-costs", "-O", "json", "--rules-file", rulesFile, "-f", csvFile}

	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger print csv: %w", err))
	}

	return parseTransactions(stdout)
}
