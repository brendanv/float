package journal

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
)

// postingLineRe matches the start of a posting line.
// Posting lines start with 1+ spaces followed by a non-space, non-semicolon character.
var postingLineRe = regexp.MustCompile(`^\s+[^\s;]`)

// anyTagRe matches a tag:value pattern in a comment string, including empty values (e.g. "tag:").
var anyTagRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_-]*:[^\s,;]*`)

// commentLineRe matches a line that is a hledger comment (starts with optional whitespace then ;).
var commentLineRe = regexp.MustCompile(`^\s*;`)

// floatMetaTagRe matches a float- hidden-meta tag pattern within a comment string.
var floatMetaTagRe = regexp.MustCompile(`float-[A-Za-z0-9_-]+:[^\s,;]*`)

// isFloatMetaLine reports whether the comment line contains only float- hidden meta tags
// and no other text content. Used to identify lines that should be preserved by ModifyTags
// and removed by ModifyFloatMeta.
func isFloatMetaLine(line string) bool {
	semiIdx := strings.Index(line, ";")
	if semiIdx < 0 {
		return false
	}
	comment := line[semiIdx+1:]
	stripped := floatMetaTagRe.ReplaceAllString(comment, "")
	stripped = strings.ReplaceAll(stripped, ",", " ")
	stripped = strings.Join(strings.Fields(stripped), " ")
	return stripped == ""
}

// ModifyTags replaces all transaction-level tags (except fid) on the transaction
// identified by fid. tags is the complete desired set of non-fid tags.
// Callers must wrap in txlock.Do().
func ModifyTags(ctx context.Context, client *hledger.Client, dataDir, fid string, tags map[string]string) error {
	txns, err := client.Transactions(ctx, "code:"+fid)
	if err != nil {
		return fmt.Errorf("journal: modify-tags: lookup fid %q: %w", fid, err)
	}
	switch len(txns) {
	case 0:
		return fmt.Errorf("journal: modify-tags: no transaction found with fid %q", fid)
	case 1:
		// expected
	default:
		return fmt.Errorf("journal: modify-tags: fid %q matched %d transactions (corrupt journal — run audit)", fid, len(txns))
	}

	txn := txns[0]
	sourceFile := txn.SourcePos[0].File
	headerLine := txn.SourcePos[0].Line // 1-indexed

	data, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("journal: modify-tags: read %s: %w", sourceFile, err)
	}

	lines := strings.Split(string(data), "\n")
	headerIdx := headerLine - 1 // 0-indexed

	if headerIdx < 0 || headerIdx >= len(lines) {
		return fmt.Errorf("journal: modify-tags: source line %d out of range in %s", headerLine, sourceFile)
	}

	// Sanity check: the header line should contain the fid.
	if !txnHeaderRe.MatchString(lines[headerIdx]) || !strings.Contains(lines[headerIdx], "("+fid+")") {
		return fmt.Errorf("journal: modify-tags: line %d in %s does not match expected transaction header for fid %q", headerLine, sourceFile, fid)
	}

	// Find where header comments end and postings begin.
	headerEnd := headerIdx + 1
	for headerEnd < len(lines) && !postingLineRe.MatchString(lines[headerEnd]) {
		headerEnd++
	}

	// Strip the entire inline comment from the header line.
	// Any free text found there becomes the first comment line below the header.
	cleanHeader, headerFreeText := stripHeaderInlineComment(lines[headerIdx])

	// Walk the comment block, separating free-text lines from hidden-meta lines.
	// User-tag-only lines are dropped (they will be replaced by the new tags below).
	var freeTextLines []string
	var floatMetaLines []string
	if headerFreeText != "" {
		freeTextLines = append(freeTextLines, "    ; "+headerFreeText)
	}
	for i := headerIdx + 1; i < headerEnd; i++ {
		if !commentLineRe.MatchString(lines[i]) {
			// blank or non-comment line in the header block — drop it
			continue
		}
		if isFloatMetaLine(lines[i]) {
			floatMetaLines = append(floatMetaLines, lines[i])
		} else {
			stripped := stripTagsFromCommentLine(lines[i])
			if stripped != "" {
				freeTextLines = append(freeTextLines, stripped)
			}
			// else: line was all user-tags — drop it (will be replaced below)
		}
	}

	// Reconstruct: header → free-text comments → user tags → hidden meta → postings.
	newLines := make([]string, 0, len(lines))
	newLines = append(newLines, lines[:headerIdx]...)
	newLines = append(newLines, cleanHeader)
	newLines = append(newLines, freeTextLines...)
	if len(tags) > 0 {
		keys := make([]string, 0, len(tags))
		for k := range tags {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			newLines = append(newLines, "    ; "+k+":"+tags[k])
		}
	}
	newLines = append(newLines, floatMetaLines...)
	newLines = append(newLines, lines[headerEnd:]...)
	newContent := strings.Join(newLines, "\n")

	if err := os.WriteFile(sourceFile, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("journal: modify-tags: write %s: %w", sourceFile, err)
	}

	slogctx.FromContext(ctx).Info("journal: transaction tags modified", "fid", fid, "file", sourceFile)
	return nil
}

// ModifyFloatMeta replaces all hidden-meta tags (those with hledger.HiddenMetaPrefix) on the
// transaction identified by fid. meta is the complete desired set; keys must include the
// "float-" prefix. User-visible tags and free-text comments are preserved unchanged.
// Callers must wrap in txlock.Do().
func ModifyFloatMeta(ctx context.Context, client *hledger.Client, dataDir, fid string, meta map[string]string) error {
	txns, err := client.Transactions(ctx, "code:"+fid)
	if err != nil {
		return fmt.Errorf("journal: modify-hidden-meta: lookup fid %q: %w", fid, err)
	}
	switch len(txns) {
	case 0:
		return fmt.Errorf("journal: modify-hidden-meta: no transaction found with fid %q", fid)
	case 1:
		// expected
	default:
		return fmt.Errorf("journal: modify-hidden-meta: fid %q matched %d transactions (corrupt journal — run audit)", fid, len(txns))
	}

	txn := txns[0]
	sourceFile := txn.SourcePos[0].File
	headerLine := txn.SourcePos[0].Line // 1-indexed

	data, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("journal: modify-hidden-meta: read %s: %w", sourceFile, err)
	}

	lines := strings.Split(string(data), "\n")
	headerIdx := headerLine - 1 // 0-indexed

	if headerIdx < 0 || headerIdx >= len(lines) {
		return fmt.Errorf("journal: modify-hidden-meta: source line %d out of range in %s", headerLine, sourceFile)
	}

	if !txnHeaderRe.MatchString(lines[headerIdx]) || !strings.Contains(lines[headerIdx], "("+fid+")") {
		return fmt.Errorf("journal: modify-hidden-meta: line %d in %s does not match expected transaction header for fid %q", headerLine, sourceFile, fid)
	}

	// Find where header comments end and postings begin.
	headerEnd := headerIdx + 1
	for headerEnd < len(lines) && !postingLineRe.MatchString(lines[headerEnd]) {
		headerEnd++
	}

	// Strip the entire inline comment from the header line.
	// Any free text found there becomes the first comment line below the header.
	cleanHeader, headerFreeText := stripHeaderInlineComment(lines[headerIdx])

	// Build new line list: drop hidden meta lines; preserve everything else.
	// If the header had inline free text, prepend it as the first comment line.
	newLines := make([]string, 0, len(lines))
	newLines = append(newLines, lines[:headerIdx]...)
	newLines = append(newLines, cleanHeader)
	if headerFreeText != "" {
		newLines = append(newLines, "    ; "+headerFreeText)
	}

	for i := headerIdx + 1; i < headerEnd; i++ {
		if commentLineRe.MatchString(lines[i]) && isFloatMetaLine(lines[i]) {
			// Drop existing hidden meta lines — they will be replaced below.
			continue
		}
		newLines = append(newLines, lines[i])
	}

	// Append new hidden meta tags, one per line (sorted for deterministic output).
	if len(meta) > 0 {
		keys := make([]string, 0, len(meta))
		for k := range meta {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			newLines = append(newLines, "    ; "+k+":"+meta[k])
		}
	}

	newLines = append(newLines, lines[headerEnd:]...)
	newContent := strings.Join(newLines, "\n")

	if err := os.WriteFile(sourceFile, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("journal: modify-hidden-meta: write %s: %w", sourceFile, err)
	}

	slogctx.FromContext(ctx).Info("journal: transaction hidden meta modified", "fid", fid, "file", sourceFile)
	return nil
}

// freeTextComment extracts the free-text portion from a parsed hledger Transaction.Comment.
// Hledger includes tag:value lines verbatim in the comment string; this strips them so the
// result contains only human-written text suitable for use in TransactionInput.Comment.
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

// stripHeaderInlineComment removes the entire inline comment from a transaction header line.
// It returns the clean header (no "; ..." suffix) and any free-text content that was in the
// inline comment (tags are dropped; free text is returned so callers can prepend it as a
// separate comment line). If there is no inline comment, freeText is "".
func stripHeaderInlineComment(line string) (cleanLine, freeText string) {
	semiIdx := strings.Index(line, ";")
	if semiIdx < 0 {
		return line, ""
	}

	comment := line[semiIdx+1:]

	// Strip tags; any remaining text is the free-text portion.
	commentStripped := anyTagRe.ReplaceAllString(comment, "")
	commentStripped = strings.ReplaceAll(commentStripped, ",", " ")
	commentStripped = strings.Join(strings.Fields(commentStripped), " ")

	return strings.TrimRight(line[:semiIdx], " "), commentStripped
}

// stripTagsFromCommentLine removes all tag:value patterns from a comment line.
// Returns empty string if the entire comment body was tags (safe to drop the line).
func stripTagsFromCommentLine(line string) string {
	semiIdx := strings.Index(line, ";")
	if semiIdx < 0 {
		return line
	}

	prefix := line[:semiIdx+1]
	comment := line[semiIdx+1:]

	stripped := anyTagRe.ReplaceAllString(comment, "")
	stripped = strings.ReplaceAll(stripped, ",", " ")
	stripped = strings.Join(strings.Fields(stripped), " ")

	if stripped == "" {
		return ""
	}
	return prefix + " " + stripped
}
