package cube

import "strings"

// Dict interns strings to dense uint32 ids in first-seen order. Ids index
// Values, which ships in the cube header so the client can decode a column of
// ids back into names.
type Dict struct {
	values []string
	index  map[string]uint32
}

// NewDict returns an empty Dict.
func NewDict() *Dict {
	return &Dict{index: make(map[string]uint32)}
}

// Intern returns the id for s, assigning a new one if this is its first sighting.
func (d *Dict) Intern(s string) uint32 {
	if id, ok := d.index[s]; ok {
		return id
	}
	id := uint32(len(d.values))
	d.values = append(d.values, s)
	d.index[s] = id
	return id
}

// ID returns the id for s and whether it has been interned.
func (d *Dict) ID(s string) (uint32, bool) {
	id, ok := d.index[s]
	return id, ok
}

// Values returns the interned strings, indexed by id. The result aliases the
// Dict's storage and must not be mutated.
func (d *Dict) Values() []string { return d.values }

// Len returns the number of interned strings.
func (d *Dict) Len() int { return len(d.values) }

// AccountMeta describes one account's position in the account hierarchy.
type AccountMeta struct {
	Path string `json:"path"`
	// Parent is the id of the nearest ancestor that is itself in the dict, or
	// -1 when no ancestor was interned. Parents are not synthesized: an account
	// tier with no postings of its own has no id, and the client reaches it by
	// prefix matching instead.
	Parent int32 `json:"parent"`
	// Depth is the number of colon-separated components, so "assets:checking"
	// has depth 2.
	Depth int32 `json:"depth"`
	// Type is hledger's account type letter (A, L, E, R, X, C, V), inherited
	// from the nearest ancestor that declares one. Carrying it lets the client
	// reproduce hledger's `type:` queries instead of guessing from the top-level
	// account name, which is only a default and can be overridden by an account
	// directive.
	Type string `json:"type,omitempty"`
}

// AccountHierarchy derives parent, depth, and type metadata for every interned
// account path. types maps account paths to hledger type letters; an account
// absent from it inherits the type of its nearest declared ancestor.
func AccountHierarchy(d *Dict, types map[string]string) []AccountMeta {
	paths := d.Values()
	out := make([]AccountMeta, len(paths))
	for i, p := range paths {
		meta := AccountMeta{
			Path:   p,
			Parent: -1,
			Depth:  int32(strings.Count(p, ":") + 1),
			Type:   inheritedType(p, types),
		}
		// Walk up the path, longest prefix first, stopping at the nearest
		// ancestor that was actually interned.
		for rest := p; ; {
			cut := strings.LastIndex(rest, ":")
			if cut < 0 {
				break
			}
			rest = rest[:cut]
			if id, ok := d.ID(rest); ok {
				meta.Parent = int32(id)
				break
			}
		}
		out[i] = meta
	}
	return out
}

// inheritedType returns the account's own type letter, or the nearest declared
// ancestor's. hledger applies types down the tree, so a leaf with no directive
// of its own still belongs to its parent's type.
func inheritedType(path string, types map[string]string) string {
	if t, ok := types[path]; ok && t != "" {
		return t
	}
	for rest := path; ; {
		cut := strings.LastIndex(rest, ":")
		if cut < 0 {
			return ""
		}
		rest = rest[:cut]
		if t, ok := types[rest]; ok && t != "" {
			return t
		}
	}
}

// TypeMatches reports whether an account's hledger type letter satisfies a
// `type:` query letter.
//
// hledger has two subtypes: Cash (C) is a kind of Asset, and Conversion (V) is
// a kind of Equity, so `type:A` matches cash accounts and `type:E` matches
// conversion accounts, while the reverse is not true. This matters more than it
// looks: hledger types a plainly-named "assets:checking" as C by its own
// default inference, so an exact-match comparison would report zero assets on
// an ordinary ledger.
func TypeMatches(accountType, want string) bool {
	if accountType == want {
		return true
	}
	switch want {
	case "A":
		return accountType == "C"
	case "E":
		return accountType == "V"
	}
	return false
}
