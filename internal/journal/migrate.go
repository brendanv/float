package journal

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	txnHeaderRe = regexp.MustCompile(`^\d{4}[/\-]\d{2}[/\-]\d{2} `)
	fidTagRe    = regexp.MustCompile(`fid:[0-9a-f]{8}`)
	codeFieldRe = regexp.MustCompile(`\([0-9a-f]{8}\)`)
	includeRe   = regexp.MustCompile(`^include\s+(.+)`)
)

// headerPrefixRe captures the date and optional status marker at the start of a
// transaction header line. Groups: 1=date+space, 2=optional status marker.
var headerPrefixRe = regexp.MustCompile(`^(\d{4}[/\-]\d{2}[/\-]\d{2} )(?:([!*]) )?`)

// MigrateFIDs scans all journal files included from main.journal in dataDir.
// It performs a two-phase migration:
//  1. Transactions with old-format "; fid:XXXX" tags → rewritten to "(XXXX)" code field
//  2. Transactions with no identifier → minted a new "(XXXX)" code
//
// Returns the count of transactions modified. Idempotent — skips transactions
// that already have a (code) field.
func MigrateFIDs(dataDir string) (int, error) {
	mainPath := filepath.Join(dataDir, "main.journal")
	mainData, err := os.ReadFile(mainPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	var includes []string
	for _, line := range strings.Split(string(mainData), "\n") {
		if m := includeRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			includes = append(includes, strings.TrimSpace(m[1]))
		}
	}

	total := 0
	for _, rel := range includes {
		abs := filepath.Join(dataDir, rel)
		n, err := migrateFile(abs)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func migrateFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	lines := strings.Split(string(data), "\n")
	count := 0
	for i, line := range lines {
		if !txnHeaderRe.MatchString(line) {
			continue
		}
		// Already has a code field — skip.
		if codeFieldRe.MatchString(line) {
			continue
		}
		// Extract the header prefix (date + optional status).
		m := headerPrefixRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		prefix := m[0] // e.g. "2026-01-15 " or "2026-01-15 * "

		if fidTagRe.MatchString(line) {
			// Old format: extract fid value, move to code field, remove from comment.
			fidMatch := fidTagRe.FindString(line) // e.g. "fid:aa001100"
			fid := fidMatch[4:]                   // strip "fid:" prefix
			rest := line[len(prefix):]             // description + comment

			// Remove the fid tag from the rest of the line.
			rest = strings.Replace(rest, fidMatch, "", 1)
			// Clean up: remove orphaned ";", commas, extra spaces.
			if semiIdx := strings.Index(rest, ";"); semiIdx >= 0 {
				comment := strings.TrimSpace(rest[semiIdx+1:])
				comment = strings.Trim(comment, ", ")
				comment = strings.Join(strings.Fields(comment), " ")
				desc := strings.TrimSpace(rest[:semiIdx])
				if comment != "" {
					rest = desc + "  ; " + comment
				} else {
					rest = desc
				}
			} else {
				rest = strings.TrimSpace(rest)
			}

			lines[i] = prefix + "(" + fid + ") " + rest
			count++
		} else {
			// No identifier at all — mint a new code.
			rest := line[len(prefix):]
			lines[i] = prefix + "(" + MintFID() + ") " + rest
			count++
		}
	}

	if count > 0 {
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			return 0, err
		}
	}
	return count, nil
}
