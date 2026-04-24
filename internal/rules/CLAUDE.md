# internal/rules

Float's transaction auto-categorization rules. Distinct from hledger CSV rules files (`.rules` in `data/rules/`) — those control CSV parsing; these control post-import categorization.

Rules are stored as JSON in `<data-dir>/rules.json`.

## Rule Fields

- `id` — 8-char hex ID (from `journal.MintFID`)
- `pattern` — case-insensitive regex matched against transaction description
- `payee` — set payee on match (empty = no change)
- `account` — set the category posting's account on match (empty = no change; only applies to 2-posting transactions)
- `tags` — map of tags to add on match
- `priority` — lower number = matched first (rules are sorted by priority before matching)
- `auto_reviewed` — if true, mark transaction `Cleared` on import

## API

- `Load(dataDir)` — reads `rules.json`; returns empty slice if missing (not an error). Returns rules sorted by priority ascending.
- `Save(dataDir, rules)` — writes `rules.json`. Must be called within `txlock.Do()`.
- `Match(rules, description)` — returns the first matching rule (by priority) or nil.
- `Preview(rules, transactions)` — returns proposed `RuleMatch` changes without writing anything. Skips transactions without a FID.
- `Apply(ctx, client, dataDir, matches)` — executes changes from a `Preview`. Must be called within `txlock.Do()`.

## ChangeSet

`Apply` uses `UpdateTransaction` for payee/account changes and `ModifyTags` for tag changes. For account changes, only 2-posting transactions with an unambiguous category posting are eligible.
