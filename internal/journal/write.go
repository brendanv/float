package journal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

	if src != nil {
		// Remove the existing transaction block before appending the replacement.
		if err := removeTransactionAtLine(src.File, src.Line, fid); err != nil {
			return "", fmt.Errorf("journal: write: remove old block: %w", err)
		}
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
		var amtStr string
		if len(p.Amounts) > 0 {
			a := p.Amounts[0]
			amtStr = fmt.Sprintf("%s%.2f", a.Commodity, a.Quantity.FloatingPoint)
		}
		postings[i] = PostingInput{
			Account: p.Account,
			Amount:  amtStr,
			Comment: strings.TrimSpace(p.Comment),
		}
	}
	return postings
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
