package ui

// filterPreset represents a named quick filter preset for the transactions panel.
type filterPreset struct {
	label  string
	tokens []string // hledger query tokens appended to the base query
}

// txFilterPresets are the available quick filter presets, matching the web UI.
// Index 0 ("All") is the default with no extra filtering.
var txFilterPresets = []filterPreset{
	{label: "All", tokens: nil},
	{label: "Reviewed", tokens: []string{"status:*"}},
	{label: "Unreviewed", tokens: []string{"not:status:*"}},
	{label: "No payee", tokens: []string{"not:payee:.+"}},
}
