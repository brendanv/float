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
}

// AccountHierarchy derives parent and depth metadata for every interned
// account path.
func AccountHierarchy(d *Dict) []AccountMeta {
	paths := d.Values()
	out := make([]AccountMeta, len(paths))
	for i, p := range paths {
		meta := AccountMeta{Path: p, Parent: -1, Depth: int32(strings.Count(p, ":") + 1)}
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
