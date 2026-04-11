package ui

// filterPreset represents a named quick filter preset for the transactions panel.
type filterPreset struct {
	label  string
	tokens []string // hledger query tokens appended to the base query
}

// txFilterPresets are the available quick filter presets for the transaction
// review panel on the home tab. Index 0 (Unreviewed) is the default, surfacing
// transactions that still need attention.
var txFilterPresets = []filterPreset{
	{label: "Unreviewed", tokens: []string{"not:status:*"}},
	{label: "All", tokens: nil},
	{label: "Reviewed", tokens: []string{"status:*"}},
	{label: "No payee", tokens: []string{"not:payee:.+"}},
}
