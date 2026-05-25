package journal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
)

// SourceLocation identifies where an existing transaction lives in the journal.
type SourceLocation struct {
	File string // absolute path to the journal file
	Line int    // 1-indexed header line number
}

// WriteTransaction writes a transaction to the journal.
// If src is nil, it appends as a new transaction (minting a FID if t.FID is empty).
// If src is non-nil, it replaces the existing transaction block at src, removing
// the old block and appending the new text to the correct month file for t.Date
// (which may differ from src.File if the date has moved to a different month).
// Stamps float-updated-at in t.FloatMeta on every write.
// Callers must wrap this in txlock.Do().
func WriteTransaction(ctx context.Context, client *hledger.Client, dataDir string, t TransactionInput, src *SourceLocation) (string, error) {
	fid := t.FID
	if fid == "" {
		fid = MintFID()
	}

	// Stamp the last-updated timestamp on every write.
	if t.FloatMeta == nil {
		t.FloatMeta = make(map[string]string)
	}
	t.FloatMeta[hledger.HiddenMetaPrefix+"updated-at"] = time.Now().UTC().Format(time.RFC3339)

	text, err := FormatViaHledger(ctx, client, t, fid)
	if err != nil {
		return "", err
	}

	year, month := t.Date.Year(), int(t.Date.Month())
	relPath, created, err := EnsureMonthFile(dataDir, year, month)
	if err != nil {
		return "", err
	}
	if created {
		mainPath := filepath.Join(dataDir, "main.journal")
		if err := UpdateMainIncludes(mainPath, relPath); err != nil {
			return "", err
		}
	}
	absPath := filepath.Join(dataDir, relPath)

	if src != nil {
		if absPath == src.File {
			// Same file: replace the block in-place so that the transaction's
			// position among other transactions is unchanged. This is required
			// when the transaction carries a balance assertion — moving it to
			// the end of the file would change the running balance at that
			// point and cause hledger to reject the journal.
			if err := replaceTransactionAtLine(src.File, src.Line, fid, text); err != nil {
				return "", fmt.Errorf("journal: write: replace block: %w", err)
			}
			slogctx.FromContext(ctx).Info("journal: transaction replaced", "fid", fid, "old_file", src.File, "new_path", relPath)
			return fid, nil
		}
		// Date moved to a different month file: remove from the old file,
		// then fall through to append to the new file below.
		if err := removeTransactionAtLine(src.File, src.Line, fid); err != nil {
			return "", fmt.Errorf("journal: write: remove old block: %w", err)
		}
	}

	f, err := os.OpenFile(absPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("journal: write: open %s: %w", absPath, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.WriteString(text); err != nil {
		return "", fmt.Errorf("journal: write: write %s: %w", absPath, err)
	}

	if src == nil {
		slogctx.FromContext(ctx).Info("journal: transaction appended", "fid", fid, "path", relPath)
	} else {
		slogctx.FromContext(ctx).Info("journal: transaction replaced", "fid", fid, "old_file", src.File, "new_path", relPath)
	}
	return fid, nil
}

// InputFromTransaction builds a TransactionInput from a looked-up hledger.Transaction,
// preserving all fields. Callers override specific fields before calling WriteTransaction.
func InputFromTransaction(t hledger.Transaction) (TransactionInput, error) {
	date, err := time.Parse("2006-01-02", t.Date)
	if err != nil {
		return TransactionInput{}, fmt.Errorf("journal: parse date %q: %w", t.Date, err)
	}
	return TransactionInput{
		Date:        date,
		Description: t.Description,
		Comment:     freeTextComment(t.Comment),
		Tags:        userTagsFromTransaction(t),
		Postings:    postingsFromTransaction(t),
		FID:         t.FID,
		Status:      t.Status,
		FloatMeta:   t.FloatMeta,
	}, nil
}

// postingsFromTransaction converts hledger postings to []PostingInput.
func postingsFromTransaction(t hledger.Transaction) []PostingInput {
	postings := make([]PostingInput, len(t.Postings))
	for i, p := range t.Postings {
		pi := PostingInput{
			Account: p.Account,
			Comment: strings.TrimSpace(p.Comment),
		}
		if len(p.Amounts) > 0 {
			a := p.Amounts[0]
			pi.Commodity = a.Commodity
			pi.Quantity = fmt.Sprintf("%.*f", a.Quantity.DecimalPlaces, a.Quantity.FloatingPoint)
			cost, _ := a.ParseCost()
			if cost != nil {
				pi.Cost = &CostInput{
					Commodity: cost.Contents.Commodity,
					Quantity:  fmt.Sprintf("%.*f", cost.Contents.Quantity.DecimalPlaces, cost.Contents.Quantity.FloatingPoint),
					IsTotal:   cost.Tag == "TotalCost",
				}
			}
		}
		pi.BalanceAssertion = balanceAssertionInputFromHledger(p.BalanceAssertion)
		postings[i] = pi
	}
	return postings
}

// balanceAssertionInputFromHledger converts a parsed hledger balance assertion
// to the journal package's input form, preserving Inclusive/Total flags.
// Returns nil when the source assertion is nil.
func balanceAssertionInputFromHledger(ba *hledger.BalanceAssertion) *BalanceAssertionInput {
	if ba == nil {
		return nil
	}
	return &BalanceAssertionInput{
		Commodity: ba.Amount.Commodity,
		Quantity:  fmt.Sprintf("%.*f", ba.Amount.Quantity.DecimalPlaces, ba.Amount.Quantity.FloatingPoint),
		Inclusive: ba.Inclusive,
		Total:     ba.Total,
	}
}

// userTagsFromTransaction extracts user-visible (non-float-) tags from t.Tags into a map.
func userTagsFromTransaction(t hledger.Transaction) map[string]string {
	tags := make(map[string]string, len(t.Tags))
	for _, tag := range t.Tags {
		if !strings.HasPrefix(tag[0], hledger.HiddenMetaPrefix) {
			tags[tag[0]] = tag[1]
		}
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}

// freeTextComment extracts the free-text portion from a parsed hledger Transaction.Comment.
// hledger includes tag:value lines verbatim in the comment string; this strips them so the
// result contains only human-written text.
func freeTextComment(comment string) string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(comment), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip lines whose entire content is one or more tag:value patterns.
		stripped := anyTagRe.ReplaceAllString(line, "")
		stripped = strings.TrimSpace(strings.ReplaceAll(stripped, ",", " "))
		if stripped != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// replaceTransactionAtLine replaces the transaction block starting at headerLine
// (1-indexed) in path with newText. The replacement is done in-place, preserving
// the transaction's file position and thus the validity of any balance assertions.
func replaceTransactionAtLine(path string, headerLine int, fid, newText string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("journal: replace: read %s: %w", path, err)
	}

	lines := strings.Split(string(data), "\n")
	headerIdx := headerLine - 1

	if headerIdx < 0 || headerIdx >= len(lines) {
		return fmt.Errorf("journal: replace: source line %d out of range in %s", headerLine, path)
	}

	if !txnHeaderRe.MatchString(lines[headerIdx]) || !strings.Contains(lines[headerIdx], "("+fid+")") {
		return fmt.Errorf("journal: replace: line %d in %s does not match expected transaction header for fid %q", headerLine, path, fid)
	}

	endIdx := headerIdx + 1
	for endIdx < len(lines) && strings.TrimSpace(lines[endIdx]) != "" {
		endIdx++
	}
	if endIdx < len(lines) && strings.TrimSpace(lines[endIdx]) == "" {
		endIdx++
	}

	// Build replacement lines: strip trailing newlines, split, then re-add
	// exactly one blank separator line to match the block structure we removed.
	replacementLines := append(strings.Split(strings.TrimRight(newText, "\n"), "\n"), "")

	newLines := make([]string, 0, len(lines)-(endIdx-headerIdx)+len(replacementLines))
	newLines = append(newLines, lines[:headerIdx]...)
	newLines = append(newLines, replacementLines...)
	newLines = append(newLines, lines[endIdx:]...)

	if err := os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0644); err != nil {
		return fmt.Errorf("journal: replace: write %s: %w", path, err)
	}
	return nil
}

// removeTransactionAtLine removes the transaction block starting at headerLine
// (1-indexed) from path. The fid is used as a sanity check on the header.
func removeTransactionAtLine(path string, headerLine int, fid string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("journal: remove: read %s: %w", path, err)
	}

	lines := strings.Split(string(data), "\n")
	headerIdx := headerLine - 1 // convert to 0-indexed

	if headerIdx < 0 || headerIdx >= len(lines) {
		return fmt.Errorf("journal: remove: source line %d out of range in %s", headerLine, path)
	}

	// Sanity check: the line should be a transaction header containing the fid.
	if !txnHeaderRe.MatchString(lines[headerIdx]) || !strings.Contains(lines[headerIdx], "("+fid+")") {
		return fmt.Errorf("journal: remove: line %d in %s does not match expected transaction header for fid %q", headerLine, path, fid)
	}

	// Walk forward to find the end of the transaction block (non-blank lines).
	endIdx := headerIdx + 1
	for endIdx < len(lines) && strings.TrimSpace(lines[endIdx]) != "" {
		endIdx++
	}
	// Include one trailing blank line if present.
	if endIdx < len(lines) && strings.TrimSpace(lines[endIdx]) == "" {
		endIdx++
	}

	// Reconstruct file without the removed block.
	newLines := append(lines[:headerIdx:headerIdx], lines[endIdx:]...)
	newContent := strings.Join(newLines, "\n")

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("journal: remove: write %s: %w", path, err)
	}
	return nil
}

// anyTagRe matches a tag:value pattern in a comment string.
var anyTagRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_-]*:[^\s,;]*`)

// BatchReplacement is a single in-place transaction replacement within a journal file.
type BatchReplacement struct {
	HeaderLine int    // 1-indexed line number of the transaction header
	FID        string // sanity-check: the FID expected at this line
	NewText    string // formatted replacement text (ends with "\n\n")
}

// DeleteSpec identifies a transaction to remove by its source location.
type DeleteSpec struct {
	HeaderLine int    // 1-indexed line number of the transaction header
	FID        string // sanity-check: the FID expected at this line
}

// batchRemoveFromFile removes multiple transaction blocks from path in one
// read+write cycle. Removals are applied in descending HeaderLine order so
// earlier removals cannot shift the line numbers of subsequent targets.
func batchRemoveFromFile(path string, specs []DeleteSpec) error {
	if len(specs) == 0 {
		return nil
	}

	sorted := make([]DeleteSpec, len(specs))
	copy(sorted, specs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].HeaderLine > sorted[j].HeaderLine
	})

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("journal: batch remove: read %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")

	for _, spec := range sorted {
		headerIdx := spec.HeaderLine - 1
		if headerIdx < 0 || headerIdx >= len(lines) {
			return fmt.Errorf("journal: batch remove: source line %d out of range in %s", spec.HeaderLine, path)
		}
		if !txnHeaderRe.MatchString(lines[headerIdx]) || !strings.Contains(lines[headerIdx], "("+spec.FID+")") {
			return fmt.Errorf("journal: batch remove: line %d in %s does not match expected transaction header for fid %q", spec.HeaderLine, path, spec.FID)
		}

		endIdx := headerIdx + 1
		for endIdx < len(lines) && strings.TrimSpace(lines[endIdx]) != "" {
			endIdx++
		}
		if endIdx < len(lines) && strings.TrimSpace(lines[endIdx]) == "" {
			endIdx++
		}

		lines = append(lines[:headerIdx:headerIdx], lines[endIdx:]...)
	}

	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return fmt.Errorf("journal: batch remove: write %s: %w", path, err)
	}
	return nil
}

// BatchReplaceTransactions applies multiple in-place transaction replacements
// to a single journal file in one read+write cycle. Replacements are applied
// in descending HeaderLine order so earlier replacements cannot shift the line
// numbers of the transactions that follow them in the file.
func BatchReplaceTransactions(path string, replacements []BatchReplacement) error {
	if len(replacements) == 0 {
		return nil
	}

	// Work on a copy sorted descending so each edit doesn't shift remaining positions.
	sorted := make([]BatchReplacement, len(replacements))
	copy(sorted, replacements)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].HeaderLine > sorted[j].HeaderLine
	})

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("journal: batch replace: read %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")

	for _, r := range sorted {
		headerIdx := r.HeaderLine - 1
		if headerIdx < 0 || headerIdx >= len(lines) {
			return fmt.Errorf("journal: batch replace: source line %d out of range in %s", r.HeaderLine, path)
		}
		if !txnHeaderRe.MatchString(lines[headerIdx]) || !strings.Contains(lines[headerIdx], "("+r.FID+")") {
			return fmt.Errorf("journal: batch replace: line %d in %s does not match expected transaction header for fid %q", r.HeaderLine, path, r.FID)
		}

		endIdx := headerIdx + 1
		for endIdx < len(lines) && strings.TrimSpace(lines[endIdx]) != "" {
			endIdx++
		}
		if endIdx < len(lines) && strings.TrimSpace(lines[endIdx]) == "" {
			endIdx++
		}

		replacementLines := append(strings.Split(strings.TrimRight(r.NewText, "\n"), "\n"), "")
		newLines := make([]string, 0, len(lines)-(endIdx-headerIdx)+len(replacementLines))
		newLines = append(newLines, lines[:headerIdx]...)
		newLines = append(newLines, replacementLines...)
		newLines = append(newLines, lines[endIdx:]...)
		lines = newLines
	}

	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return fmt.Errorf("journal: batch replace: write %s: %w", path, err)
	}
	return nil
}
