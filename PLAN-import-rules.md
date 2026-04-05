# Transaction Import & Categorization Rules

## Context

Float currently supports CSV import only via `floatctl journal import` (CLI). There is no API/web UI for importing, and no system for user-defined categorization rules that can run retroactively on existing transactions. The goal is to bring float closer to traditional budgeting apps by letting users upload CSVs, define rules like "if description matches X, set payee to Y and account to Z", and apply those rules to past transactions.

**Design decisions (confirmed with user):**
- Float rules are a **separate system** from hledger CSV rules files. hledger rules handle CSV column mapping during import; float rules handle categorization (during import as a second pass AND retroactively).
- Rule actions: **set category account**, **set payee**, **add tags**
- Retroactive scope: **all transactions** (with preview before applying)

---

## Implementation Plan

### Step 1: Rule Storage (`internal/rules/`)

New package `internal/rules/` to manage categorization rules stored as JSON in `data/rules.json`.

**File: `internal/rules/rules.go`**

```go
type Rule struct {
    ID       string            `json:"id"`       // 8-char hex (MintFID)
    Pattern  string            `json:"pattern"`  // regex matched against description
    Payee    string            `json:"payee"`    // set payee (empty = no change)
    Account  string            `json:"account"`  // set category account (empty = no change)
    Tags     map[string]string `json:"tags"`     // tags to add (empty = no change)
    Priority int               `json:"priority"` // lower = higher priority, matched first
}

func Load(dataDir string) ([]Rule, error)           // read data/rules.json
func Save(dataDir string, rules []Rule) error        // write data/rules.json
func Match(rules []Rule, description string) *Rule   // first match by priority, nil if none
```

- Rules file: `data/rules.json` — simple JSON array, sorted by priority
- Pattern matching: Go `regexp.MatchString` against the transaction description (case-insensitive)
- `Match()` iterates rules in priority order, returns first match
- `Load()` returns empty slice if file doesn't exist (not an error)
- `Save()` must be called within `txlock.Do()` since it modifies data dir files

**File: `internal/rules/apply.go`**

```go
type RuleMatch struct {
    Rule        Rule
    Transaction hledger.Transaction
    Changes     ChangeSet
}

type ChangeSet struct {
    NewPayee   *string           // nil = no change
    NewAccount *string           // nil = no change (posting account to set)
    AddTags    map[string]string // tags to add
}

// Preview checks all transactions against rules and returns proposed changes.
// Does NOT modify anything.
func Preview(rules []Rule, transactions []hledger.Transaction) []RuleMatch

// Apply executes the changes from a preview. Must be called within txlock.Do().
// Uses journal.UpdateTransaction for account/payee changes, journal.ModifyTags for tags.
func Apply(ctx context.Context, client *hledger.Client, dataDir string, matches []RuleMatch) (applied int, err error)
```

For "set category account": identify the posting that is NOT an asset/liability account (using hledger account types). For simple 2-posting transactions, change the expense/income posting's account. Skip transactions with 3+ postings or where both postings are the same type (ambiguous).

---

### Step 2: Proto Definitions

Add to `proto/float/v1/ledger.proto`:

**New messages:**
```protobuf
// ---- Import ----
message BankProfile {
  string name = 1;
  string rules_file = 2;  // relative path in data dir
}

message ListBankProfilesRequest {}
message ListBankProfilesResponse {
  repeated BankProfile profiles = 1;
}

message PreviewImportRequest {
  bytes csv_data = 1;        // raw CSV file contents
  string profile_name = 2;   // bank profile name from config
}

message PreviewImportResponse {
  repeated ImportCandidate candidates = 1;
}

message ImportCandidate {
  Transaction transaction = 1;   // parsed transaction
  bool is_duplicate = 2;         // true if fingerprint matches existing
  string matched_rule_id = 3;    // float rule that matched (empty if none)
}

message ImportTransactionsRequest {
  repeated int32 candidate_indices = 1;  // which candidates to import (from preview)
  bytes csv_data = 2;                     // same CSV data as preview
  string profile_name = 3;               // same profile as preview
}

message ImportTransactionsResponse {
  int32 imported_count = 1;
  repeated Transaction transactions = 2;  // the imported transactions
}

// ---- Categorization Rules ----
message TransactionRule {
  string id = 1;
  string pattern = 2;             // regex pattern
  string payee = 3;               // set payee (empty = no change)
  string account = 4;             // set category account (empty = no change)
  map<string, string> tags = 5;   // tags to add
  int32 priority = 6;
}

message ListRulesRequest {}
message ListRulesResponse {
  repeated TransactionRule rules = 1;
}

message AddRuleRequest {
  string pattern = 1;
  string payee = 2;
  string account = 3;
  map<string, string> tags = 4;
  int32 priority = 5;
}
message AddRuleResponse {
  TransactionRule rule = 1;
}

message UpdateRuleRequest {
  string id = 1;
  string pattern = 2;
  string payee = 3;
  string account = 4;
  map<string, string> tags = 5;
  int32 priority = 6;
}
message UpdateRuleResponse {
  TransactionRule rule = 1;
}

message DeleteRuleRequest {
  string id = 1;
}
message DeleteRuleResponse {}

message PreviewApplyRulesRequest {
  repeated string rule_ids = 1;  // empty = all rules
  repeated string query = 2;     // optional hledger query to filter transactions
}

message RuleApplicationPreview {
  string fid = 1;                 // transaction FID
  string description = 2;        // current description
  string matched_rule_id = 3;    // which rule matched
  string current_account = 4;    // current category account
  string new_account = 5;        // proposed new account (empty = no change)
  string current_payee = 6;
  string new_payee = 7;          // proposed new payee (empty = no change)
  map<string, string> add_tags = 8;
}

message PreviewApplyRulesResponse {
  repeated RuleApplicationPreview previews = 1;
}

message ApplyRulesRequest {
  repeated string fids = 1;       // which transactions to apply (from preview)
  repeated string rule_ids = 2;   // which rules to apply (empty = all)
  repeated string query = 3;      // same query as preview
}

message ApplyRulesResponse {
  int32 applied_count = 1;
}
```

**New RPCs in LedgerService:**
```protobuf
// Import
rpc ListBankProfiles(ListBankProfilesRequest) returns (ListBankProfilesResponse);
rpc PreviewImport(PreviewImportRequest) returns (PreviewImportResponse);
rpc ImportTransactions(ImportTransactionsRequest) returns (ImportTransactionsResponse);

// Rules
rpc ListRules(ListRulesRequest) returns (ListRulesResponse);
rpc AddRule(AddRuleRequest) returns (AddRuleResponse);
rpc UpdateRule(UpdateRuleRequest) returns (UpdateRuleResponse);
rpc DeleteRule(DeleteRuleRequest) returns (DeleteRuleResponse);
rpc PreviewApplyRules(PreviewApplyRulesRequest) returns (PreviewApplyRulesResponse);
rpc ApplyRules(ApplyRulesRequest) returns (ApplyRulesResponse);
```

---

### Step 3: Server Handlers

**File: `internal/server/ledger/handler.go`** (add methods)

**Import handlers:**

1. `ListBankProfiles` — load `config.toml`, return `BankProfiles` list
2. `PreviewImport`:
   - Write CSV bytes to a temp file
   - Load bank profile, resolve rules file path
   - Call `hledger.PrintCSV(csvFile, rulesFile)` to parse
   - Fetch existing transactions for dedup (reuse `txnFingerprint` logic from floatctl)
   - Run float categorization rules as second pass on parsed transactions
   - Return candidates with dup status and matched rule IDs
3. `ImportTransactions`:
   - Re-parse CSV (or require client to send same data)
   - Filter to selected candidate indices
   - Write each via `txlock.Do()` + `journal.AppendTransaction()`
   - Apply float rule changes (payee/account/tags) during write

**Rules handlers:**

4. `ListRules` — `rules.Load(dataDir)`
5. `AddRule` — load, append (mint ID), save within txlock (since it modifies data dir)
6. `UpdateRule` — load, find by ID, update, save within txlock
7. `DeleteRule` — load, remove by ID, save within txlock
8. `PreviewApplyRules` — load rules + query transactions, run `rules.Preview()`, return matches
9. `ApplyRules` — load rules + transactions, filter to requested FIDs, apply changes via `rules.Apply()` within txlock

**Move shared helpers from floatctl:**
- `txnFingerprint()` → `internal/import/dedup.go` or `internal/rules/dedup.go`
- `hledgerTxnToInput()` → `internal/journal/convert.go` (near existing `InputFromTransaction`)

---

### Step 4: Web UI

**New page: `web/src/pages/import.jsx`**
- File upload input + bank profile dropdown
- "Preview" button → calls `PreviewImport`
- Table showing parsed transactions with [NEW]/[DUP] badges
- Checkboxes to select which to import
- Shows which float rule matched each transaction
- "Import Selected" button → calls `ImportTransactions`

**New page: `web/src/pages/rules.jsx`**
- List existing rules (sortable by priority)
- Add/edit/delete rule forms (pattern, payee, account, tags, priority)
- "Test pattern" input: type a description, shows which rule matches
- "Apply Rules" button → calls `PreviewApplyRules`, shows preview dialog, then `ApplyRules`

**Router update: `web/src/app.jsx`**
- Add `/import` and `/rules` routes
- Add nav links

---

### Step 5: floatctl Updates

Move `txnFingerprint` and `hledgerTxnToInput` from `cmd/floatctl/journal.go` to shared internal packages so they're reusable by the server handler.

Add new floatctl commands:
- `floatctl rules list <data-dir>` — list rules as JSON
- `floatctl rules add <data-dir> --pattern "..." --payee "..." --account "..." --priority N`
- `floatctl rules delete <data-dir> <rule-id>`
- `floatctl rules apply <data-dir> [--rule-id ...] [query...]` — preview + apply

---

## File Summary

| File | Action |
|------|--------|
| `internal/rules/rules.go` | **New** — Rule type, Load/Save/Match |
| `internal/rules/apply.go` | **New** — Preview/Apply logic |
| `internal/rules/rules_test.go` | **New** — Unit tests |
| `proto/float/v1/ledger.proto` | **Modify** — Add import + rule messages and RPCs |
| `internal/server/ledger/handler.go` | **Modify** — Add 9 new RPC handlers |
| `internal/journal/convert.go` | **New** — Move `hledgerTxnToInput` + `txnFingerprint` here |
| `cmd/floatctl/journal.go` | **Modify** — Use shared convert functions |
| `cmd/floatctl/rules.go` | **New** — floatctl rules subcommands |
| `web/src/pages/import.jsx` | **New** — Import page |
| `web/src/pages/rules.jsx` | **New** — Rules management page |
| `web/src/app.jsx` | **Modify** — Add routes + nav |
| `internal/config/config.go` | **No change** — BankProfile already exists |

---

## Implementation Order

1. `internal/rules/` package (rules.go + apply.go + tests) — foundation, no dependencies
2. `internal/journal/convert.go` — extract shared helpers from floatctl
3. Proto definitions + `mise run proto-gen` — defines the API contract
4. Server handlers — import RPCs first, then rules RPCs
5. `cmd/floatctl/rules.go` — CLI for rules management
6. Web UI pages — import page, then rules page
7. Integration tests — end-to-end import + rule application

---

## Verification

1. **Unit tests**: `internal/rules/` — pattern matching, priority ordering, Preview/Apply logic
2. **Integration tests**: Import flow with test CSV + rules file (reuse `testdata/import.csv` and `testdata/import.rules`)
3. **Manual test via floatctl**:
   - `floatctl rules add <data-dir> --pattern "AMAZON" --payee "Amazon" --account "expenses:shopping"`
   - `floatctl journal import <data-dir> <csv> --profile "..."` — verify rules applied during import
   - `floatctl rules apply <data-dir>` — verify retroactive application
4. **Manual test via API**: `buf curl` for each new RPC
5. **Web UI**: Upload CSV, preview, import; create rules, apply retroactively
6. **Run `mise run check`** — lint + test pass
