---
name: gen-fake-data
description: Generate a fake float data directory populated with realistic transactions so floatd can be started immediately. TRIGGER when the user asks to generate fake data, seed a data directory, populate test data, create sample journal entries, or set up a development data directory for float.
---

# gen-fake-data skill

Generate a complete float data directory with fake but realistic transactions using `scripts/gen-fake-data/main.go`.

## What gets created

| File | Description |
|------|-------------|
| `config.toml` | Server on port 8080, `admin` + `viewer` users, 3 bank profiles |
| `accounts.journal` | Account declarations (assets, liabilities, expenses, income, equity) |
| `prices.journal` | Empty prices file |
| `main.journal` | Include directives for all files |
| `rules/chase-checking.rules` | CSV import rules for checking account |
| `rules/chase-savings.rules` | CSV import rules for savings account |
| `rules/amex-credit.rules` | CSV import rules for credit card |
| `YYYY/MM.journal` | One file per requested month with ~25 transactions each |

Each month includes: two payroll deposits, rent, utilities, gas, groceries, streaming subscriptions, restaurant meals, shopping (credit card), a credit card payoff, and savings interest.

## Step 1 — Determine arguments

Ask or infer from context:
- **Output directory** — where to write files. Use `./data` unless the user specifies otherwise or a data directory is already referenced in the conversation.
- **Months** — how many months of history to generate. Default 6; use a higher number (12–24) if the user wants a richer dataset.
- **Seed** — omit unless the user wants reproducible output.

## Step 2 — Run the script

```bash
go run ./scripts/gen-fake-data \
  -output-dir <OUTPUT_DIR> \
  -months <N> \
  [-seed <SEED>]
```

Example for a fresh `./data` directory with 12 months:

```bash
go run ./scripts/gen-fake-data -output-dir ./data -months 12
```

## Step 3 — Validate with hledger

Always confirm the journal is well-formed before reporting success:

```bash
hledger -f <OUTPUT_DIR>/main.journal check
```

If `check` exits non-zero, read the error and fix the underlying issue before continuing.

## Step 4 — Report to the user

Tell the user:
- The output directory (absolute path)
- How many months and total transactions were generated
- That floatd can now be started with:
  ```
  floatd --data-dir <OUTPUT_DIR>
  ```
- The two pre-created user accounts (`admin` / `viewer`) — note the passphrase hashes are placeholder values since auth is not yet implemented
