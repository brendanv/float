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

	// Strip non-fid tags from the inline comment on the header date line.
	lines[headerIdx] = stripNonFidTagsFromHeaderLine(lines[headerIdx], fid)

	// Build new line list: process header comment lines (headerIdx+1 to headerEnd),
	// stripping tags and dropping lines that become empty.
	newLines := make([]string, 0, len(lines))
	newLines = append(newLines, lines[:headerIdx+1]...)

	for i := headerIdx + 1; i < headerEnd; i++ {
		if commentLineRe.MatchString(lines[i]) {
			stripped := stripTagsFromCommentLine(lines[i])
			if stripped != "" {
				newLines = append(newLines, stripped)
			}
			// else: line was all tags — drop it entirely
		} else {
			// blank lines or other non-comment lines in header block
			newLines = append(newLines, lines[i])
		}
	}

	// Append new tags as a single comment line (sorted for deterministic output).
	if len(tags) > 0 {
		keys := make([]string, 0, len(tags))
		for k := range tags {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+":"+tags[k])
		}
		newLines = append(newLines, "    ; "+strings.Join(parts, ", "))
	}

	newLines = append(newLines, lines[headerEnd:]...)
	newContent := strings.Join(newLines, "\n")

	if err := os.WriteFile(sourceFile, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("journal: modify-tags: write %s: %w", sourceFile, err)
	}

	slogctx.FromContext(ctx).Info("journal: transaction tags modified", "fid", fid, "file", sourceFile)
	return nil
}

func stripNonFidTagsFromHeaderLine(line, fid string) string {
	semiIdx := strings.Index(line, ";")
	if semiIdx < 0 {
		return line
	}

	prefix := line[:semiIdx+1]
	comment := line[semiIdx+1:]

	// Strip all tags from the inline comment.
	commentStripped := anyTagRe.ReplaceAllString(comment, "")
	commentStripped = strings.ReplaceAll(commentStripped, ",", " ")
	commentStripped = strings.Join(strings.Fields(commentStripped), " ")

	if commentStripped != "" {
		return prefix + " " + commentStripped
	}
	// Comment was entirely tags — remove the entire inline comment.
	return strings.TrimRight(line[:semiIdx], " ")
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
