package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Price is a parsed P directive from prices.journal.
type Price struct {
	PID       string // 8-char hex from inline comment; empty if absent
	Date      string // "YYYY-MM-DD"
	Commodity string // commodity being priced, e.g. "AAPL"
	Quantity  string // e.g. "178.50"
	Currency  string // e.g. "USD"
}

var (
	priceLineRe = regexp.MustCompile(`^P\s+(\d{4}[-/]\d{2}[-/]\d{2})\s+(\S+)\s+(\S+)\s+(\S+)`)
	pidRe       = regexp.MustCompile(`pid:([0-9a-f]{8})`)
)

const pricesRelPath = "prices.journal"

// ListPrices reads prices.journal and returns all parsed P directives.
// Returns an empty slice (not an error) if prices.journal does not yet exist.
// Does NOT acquire txlock — safe to call concurrently with reads.
func ListPrices(dataDir string) ([]Price, error) {
	path := filepath.Join(dataDir, pricesRelPath)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("journal: read %s: %w", pricesRelPath, err)
	}

	var prices []Price
	for _, line := range strings.Split(string(data), "\n") {
		m := priceLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		p := Price{
			Date:      m[1],
			Commodity: m[2],
			Quantity:  m[3],
			Currency:  m[4],
		}
		if pm := pidRe.FindStringSubmatch(line); pm != nil {
			p.PID = pm[1]
		}
		prices = append(prices, p)
	}
	return prices, nil
}

// AppendPrice writes a new P directive to prices.journal, minting a PID.
// Ensures prices.journal exists and is included in main.journal (prepended
// before any month-file includes so prices are in scope for all transactions).
// Returns the minted PID.
// Does NOT acquire txlock — callers must wrap in txlock.Do().
func AppendPrice(dataDir, date, commodity, quantity, currency string) (string, error) {
	pid := MintFID()
	line := fmt.Sprintf("P %s %s %s %s  ; pid:%s\n", date, commodity, quantity, currency, pid)

	if err := ensurePricesFile(dataDir); err != nil {
		return "", err
	}
	if err := prependPricesInclude(dataDir); err != nil {
		return "", err
	}

	path := filepath.Join(dataDir, pricesRelPath)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("journal: open %s: %w", pricesRelPath, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(line); err != nil {
		return "", fmt.Errorf("journal: write %s: %w", pricesRelPath, err)
	}
	return pid, nil
}

// DeletePrice removes the P directive with the given PID from prices.journal.
// Returns an error if prices.journal does not exist or the PID is not found.
// Does NOT acquire txlock — callers must wrap in txlock.Do().
func DeletePrice(dataDir, pid string) error {
	path := filepath.Join(dataDir, pricesRelPath)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("journal: prices.journal not found")
	}
	if err != nil {
		return fmt.Errorf("journal: read %s: %w", pricesRelPath, err)
	}

	needle := "pid:" + pid
	lines := strings.Split(string(data), "\n")
	found := false
	newLines := lines[:0:0]
	for _, line := range lines {
		if strings.Contains(line, needle) {
			found = true
			continue
		}
		newLines = append(newLines, line)
	}
	if !found {
		return fmt.Errorf("journal: price with pid %q not found", pid)
	}

	return os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0644)
}

// ensurePricesFile creates prices.journal with a header comment if it doesn't exist.
func ensurePricesFile(dataDir string) error {
	path := filepath.Join(dataDir, pricesRelPath)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	header := "; float: commodity market prices\n"
	if err := os.WriteFile(path, []byte(header), 0644); err != nil {
		return fmt.Errorf("journal: create %s: %w", pricesRelPath, err)
	}
	return nil
}

// prependPricesInclude ensures "include prices.journal" appears in main.journal
// before any other include directives, so prices are in scope for all transactions.
func prependPricesInclude(dataDir string) error {
	mainPath := filepath.Join(dataDir, "main.journal")
	directive := "include " + pricesRelPath

	existing, err := os.ReadFile(mainPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("journal: read main.journal: %w", err)
	}

	lines := strings.Split(string(existing), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == directive {
			return nil // already present
		}
	}

	// Prepend before the first include line that is not accounts.journal
	// (accounts must always appear first so declarations are in scope for prices).
	insertAt := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "include ") && trimmed != "include "+accountsRelPath {
			insertAt = i
			break
		}
		insertAt = i + 1
	}

	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:insertAt]...)
	newLines = append(newLines, directive)
	newLines = append(newLines, lines[insertAt:]...)

	return os.WriteFile(mainPath, []byte(strings.Join(newLines, "\n")), 0644)
}
