// Package txfilter translates the bounded hledger query token vocabulary
// produced by the web UI (date, acct, tag, payee, desc, status, code, and
// their not: negations) into an in-memory predicate over already-parsed
// []hledger.Transaction. It exists so that filtering a transaction list by
// UI-driven query changes (date range, account, tag, status, payee, search)
// costs a slice scan instead of a new full-journal hledger invocation.
//
// Parse is strict: any token outside this vocabulary returns ok=false, and
// the caller must fall back to an hledger query — this package never guesses
// at hledger's broader query grammar (depth:, amt:, cur:, real:, etc.).
package txfilter

import (
	"regexp"
	"strings"
	"time"

	"github.com/brendanv/float/internal/hledger"
)

type predicate func(t *hledger.Transaction) bool

// Filter is a parsed, in-memory equivalent of an hledger query token list.
type Filter struct {
	preds []predicate
}

// Parse translates tokens into a Filter. ok is false if any token uses
// syntax or a keyword outside the supported vocabulary; callers should fall
// back to passing the tokens straight through to hledger in that case.
func Parse(tokens []string) (*Filter, bool) {
	f := &Filter{preds: make([]predicate, 0, len(tokens))}
	for _, tok := range tokens {
		negate := false
		rest := tok
		if r, ok := strings.CutPrefix(rest, "not:"); ok {
			negate = true
			rest = r
		}
		pred, ok := parseToken(rest)
		if !ok {
			return nil, false
		}
		if negate {
			inner := pred
			pred = func(t *hledger.Transaction) bool { return !inner(t) }
		}
		f.preds = append(f.preds, pred)
	}
	return f, true
}

// Match reports whether t satisfies every token in the filter.
func (f *Filter) Match(t *hledger.Transaction) bool {
	for _, p := range f.preds {
		if !p(t) {
			return false
		}
	}
	return true
}

// Filter returns the subset of txns matching f, preserving order.
func (f *Filter) Filter(txns []hledger.Transaction) []hledger.Transaction {
	out := make([]hledger.Transaction, 0, len(txns))
	for i := range txns {
		if f.Match(&txns[i]) {
			out = append(out, txns[i])
		}
	}
	return out
}

func parseToken(tok string) (predicate, bool) {
	idx := strings.IndexByte(tok, ':')
	if idx < 0 {
		return nil, false
	}
	keyword, value := tok[:idx], tok[idx+1:]
	switch keyword {
	case "date":
		return parseDate(value)
	case "acct":
		return parseRegexToken(value, matchAccount)
	case "payee":
		return parseRegexToken(value, matchPayee)
	case "desc":
		return parseRegexToken(value, matchDesc)
	case "tag":
		return parseTag(value)
	case "status":
		return parseStatus(value)
	case "code":
		return parseCode(value)
	default:
		return nil, false
	}
}

func compileInfixRegex(pattern string) (*regexp.Regexp, bool) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, false
	}
	return re, true
}

func parseRegexToken(value string, match func(t *hledger.Transaction, re *regexp.Regexp) bool) (predicate, bool) {
	re, ok := compileInfixRegex(value)
	if !ok {
		return nil, false
	}
	return func(t *hledger.Transaction) bool { return match(t, re) }, true
}

func matchAccount(t *hledger.Transaction, re *regexp.Regexp) bool {
	for _, p := range t.Postings {
		if re.MatchString(p.Account) {
			return true
		}
	}
	return false
}

// matchPayee mirrors hledger: matches the payee part of the description, or
// the whole description when there is no "|" separator (Payee is nil).
func matchPayee(t *hledger.Transaction, re *regexp.Regexp) bool {
	if t.Payee != nil {
		return re.MatchString(*t.Payee)
	}
	return re.MatchString(t.Description)
}

func matchDesc(t *hledger.Transaction, re *regexp.Regexp) bool {
	return re.MatchString(t.Description)
}

// parseDate parses "A..B" (either side optional) or a single exact date, in
// hledger's accepted "YYYY/MM/DD" or "YYYY-MM-DD" forms. The end of a range
// is exclusive, matching hledger's date: query semantics for `print`.
func parseDate(value string) (predicate, bool) {
	if strings.Contains(value, "..") {
		parts := strings.SplitN(value, "..", 2)
		begin, hasBegin, ok := parseOptionalDate(parts[0])
		if !ok {
			return nil, false
		}
		end, hasEnd, ok := parseOptionalDate(parts[1])
		if !ok {
			return nil, false
		}
		return func(t *hledger.Transaction) bool {
			d, ok := parseHledgerDate(t.Date)
			if !ok {
				return false
			}
			if hasBegin && d.Before(begin) {
				return false
			}
			if hasEnd && !d.Before(end) {
				return false
			}
			return true
		}, true
	}

	day, ok := parseHledgerDate(value)
	if !ok {
		return nil, false
	}
	next := day.AddDate(0, 0, 1)
	return func(t *hledger.Transaction) bool {
		d, ok := parseHledgerDate(t.Date)
		if !ok {
			return false
		}
		return !d.Before(day) && d.Before(next)
	}, true
}

func parseOptionalDate(s string) (time.Time, bool, bool) {
	if s == "" {
		return time.Time{}, false, true
	}
	d, ok := parseHledgerDate(s)
	return d, true, ok
}

func parseHledgerDate(s string) (time.Time, bool) {
	s = strings.ReplaceAll(s, "/", "-")
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false
	}
	return d, true
}

// parseTag handles "tag:K" (key present, any value) and "tag:K=RE" (key
// present, value matches RE as a case-insensitive infix regex). Key matching
// is case-insensitive, matching hledger. Checks transaction-level tags and
// every posting's tags, since hledger's tag: query matches either.
func parseTag(value string) (predicate, bool) {
	key := value
	hasValueMatch := false
	var re *regexp.Regexp
	if i := strings.IndexByte(value, '='); i >= 0 {
		key = value[:i]
		var ok bool
		re, ok = compileInfixRegex(value[i+1:])
		if !ok {
			return nil, false
		}
		hasValueMatch = true
	}
	return func(t *hledger.Transaction) bool {
		if tagMatch(t.Tags, key, re, hasValueMatch) {
			return true
		}
		for _, p := range t.Postings {
			if tagMatch(p.Tags, key, re, hasValueMatch) {
				return true
			}
		}
		return false
	}, true
}

func tagMatch(tags [][2]string, key string, re *regexp.Regexp, hasValueMatch bool) bool {
	for _, kv := range tags {
		if !strings.EqualFold(kv[0], key) {
			continue
		}
		if !hasValueMatch || re.MatchString(kv[1]) {
			return true
		}
	}
	return false
}

// parseStatus supports the three hledger transaction statuses: "" (Unmarked),
// "!" (Pending), "*" (Cleared).
func parseStatus(value string) (predicate, bool) {
	var want string
	switch value {
	case "":
		want = "Unmarked"
	case "!":
		want = "Pending"
	case "*":
		want = "Cleared"
	default:
		return nil, false
	}
	return func(t *hledger.Transaction) bool { return t.Status == want }, true
}

// parseCode matches the FID (transaction code) exactly.
func parseCode(value string) (predicate, bool) {
	return func(t *hledger.Transaction) bool { return t.FID == value }, true
}
