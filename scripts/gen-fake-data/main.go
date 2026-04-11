// gen-fake-data generates a complete float data directory populated with
// realistic fake transactions, so that floatd can be started immediately.
//
// Usage:
//
//	go run ./scripts/gen-fake-data [flags]
//
// Flags:
//
//	-output-dir string   directory to write data files into (default "data")
//	-months     int      months of transaction history to generate (default 6)
//	-seed       int64    random seed for reproducible output (default: time-based)
package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func main() {
	outputDir := flag.String("output-dir", "data", "directory to write data files into")
	months := flag.Int("months", 6, "months of transaction history to generate")
	seed := flag.Int64("seed", 0, "random seed for reproducible output (0 = time-based)")
	flag.Parse()

	if *months < 1 {
		fmt.Fprintln(os.Stderr, "error: -months must be at least 1")
		os.Exit(1)
	}

	var rng *rand.Rand
	if *seed != 0 {
		rng = rand.New(rand.NewSource(*seed))
	} else {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	if err := run(*outputDir, *months, rng); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(outputDir string, numMonths int, rng *rand.Rand) error {
	rulesDir := filepath.Join(outputDir, "rules")
	for _, dir := range []string{outputDir, rulesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	staticFiles := map[string]string{
		"config.toml":              configTOML,
		"accounts.journal":         accountsJournal,
		"prices.journal":           "; float prices\n",
		"rules/chase-checking.rules": chaseCheckingRules,
		"rules/chase-savings.rules":  chaseSavingsRules,
		"rules/amex-credit.rules":    amexCreditRules,
	}
	for name, content := range staticFiles {
		path := filepath.Join(outputDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
		fmt.Printf("  wrote  %s\n", name)
	}

	// Generate monthly journal files, working backwards from current month.
	periods := monthsBefore(numMonths, time.Now())

	var mainIncludes []string
	mainIncludes = append(mainIncludes, "include accounts.journal", "include prices.journal")

	totalTxns := 0
	for i, p := range periods {
		yearDir := filepath.Join(outputDir, fmt.Sprintf("%04d", p.year))
		if err := os.MkdirAll(yearDir, 0755); err != nil {
			return err
		}

		txns := generateMonth(rng, p.year, p.month, i == 0)
		relPath := fmt.Sprintf("%04d/%02d.journal", p.year, p.month)
		absPath := filepath.Join(outputDir, relPath)

		var sb strings.Builder
		fmt.Fprintf(&sb, "; float: %04d/%02d\n\n", p.year, p.month)
		for _, t := range txns {
			sb.WriteString(t.format())
			sb.WriteByte('\n')
		}

		if err := os.WriteFile(absPath, []byte(sb.String()), 0644); err != nil {
			return fmt.Errorf("write %s: %w", relPath, err)
		}
		mainIncludes = append(mainIncludes, "include "+relPath)
		totalTxns += len(txns)
		fmt.Printf("  wrote  %s  (%d transactions)\n", relPath, len(txns))
	}

	mainContent := strings.Join(mainIncludes, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(outputDir, "main.journal"), []byte(mainContent), 0644); err != nil {
		return fmt.Errorf("write main.journal: %w", err)
	}
	fmt.Printf("  wrote  main.journal  (%d monthly files, %d total transactions)\n",
		len(periods), totalTxns)

	abs, _ := filepath.Abs(outputDir)
	fmt.Printf("\nDone. Data directory: %s\n", abs)
	fmt.Printf("Run floatd with:  floatd --data-dir %s\n", abs)
	return nil
}

// ── date helpers ──────────────────────────────────────────────────────────────

type yearMonth struct{ year, month int }

func monthsBefore(n int, ref time.Time) []yearMonth {
	result := make([]yearMonth, n)
	y, m := ref.Year(), int(ref.Month())
	for i := n - 1; i >= 0; i-- {
		result[i] = yearMonth{y, m}
		m--
		if m == 0 {
			m = 12
			y--
		}
	}
	return result
}

func lastDay(year, month int) int {
	// time.Date normalises day=0 to the last day of the previous month.
	return time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
}

func dateIn(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func randomDate(rng *rand.Rand, year, month, minDay, maxDay int) time.Time {
	last := lastDay(year, month)
	if maxDay > last {
		maxDay = last
	}
	day := minDay + rng.Intn(maxDay-minDay+1)
	return dateIn(year, month, day)
}

// ── transaction model ─────────────────────────────────────────────────────────

type posting struct {
	account string
	amount  string // empty = auto-balance
}

type transaction struct {
	date        time.Time
	description string
	fid         string
	status      string // "", "Pending", "Cleared"
	tags        map[string]string
	comment     string
	postings    []posting
}

func (t transaction) format() string {
	var b strings.Builder
	statusStr := map[string]string{"Cleared": "* ", "Pending": "! "}[t.status]
	fmt.Fprintf(&b, "%s %s(%s) %s\n", t.date.Format("2006-01-02"), statusStr, t.fid, t.description)
	if t.comment != "" {
		fmt.Fprintf(&b, "    ; %s\n", t.comment)
	}
	keys := make([]string, 0, len(t.tags))
	for k := range t.tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "    ; %s:%s\n", k, t.tags[k])
	}
	for _, p := range t.postings {
		if p.amount != "" {
			fmt.Fprintf(&b, "    %-42s%s\n", p.account, p.amount)
		} else {
			fmt.Fprintf(&b, "    %s\n", p.account)
		}
	}
	return b.String()
}

func mintFID(rng *rand.Rand) string {
	return fmt.Sprintf("%08x", rng.Uint32())
}

func fmtAmount(dollars float64) string {
	return fmt.Sprintf("$%.2f", math.Round(dollars*100)/100)
}

func randFloat(rng *rand.Rand, min, max float64) float64 {
	return min + rng.Float64()*(max-min)
}

func pick(rng *rand.Rand, choices []string) string {
	return choices[rng.Intn(len(choices))]
}

func newTxn(rng *rand.Rand, d time.Time, desc, status string, ps []posting) transaction {
	return transaction{
		date:        d,
		description: desc,
		fid:         mintFID(rng),
		status:      status,
		postings:    ps,
	}
}

// ── transaction generators ────────────────────────────────────────────────────

var gasStations = []string{
	"SHELL GAS STATION", "CHEVRON", "BP GAS STATION", "EXXON MOBIL", "CIRCLE K FUEL",
}

var groceryStores = []string{
	"WHOLE FOODS MARKET", "KROGER", "TRADER JOES", "SAFEWAY", "ALDI", "PUBLIX", "SPROUTS FARMERS MARKET",
}

var restaurants = []string{
	"CHIPOTLE MEXICAN GRILL", "MCDONALDS", "STARBUCKS COFFEE", "PANERA BREAD",
	"OLIVE GARDEN", "LOCAL PIZZA KITCHEN", "THAI ORCHID RESTAURANT", "SUSHI NORI",
	"UBER EATS ORDER", "DOORDASH ORDER", "TACO BELL", "SUBWAY", "FIVE GUYS BURGERS",
}

var shoppingPlaces = []string{
	"AMAZON.COM", "TARGET", "BEST BUY", "COSTCO WHOLESALE",
	"HOME DEPOT", "NORDSTROM", "WALMART", "WALGREENS", "CVS PHARMACY",
}

func generateMonth(rng *rand.Rand, year, month int, isFirst bool) []transaction {
	const salary = 3500.00
	const rent = 1800.00

	var txns []transaction

	// Opening balances — first month only
	if isFirst {
		txns = append(txns, newTxn(rng, dateIn(year, month, 1), "Opening balances", "Cleared", []posting{
			{"assets:checking", "$10000.00"},
			{"assets:savings", "$25000.00"},
			{"equity:opening-balances", ""},
		}))
	}

	// Two payroll deposits on the 1st and 15th
	for _, day := range []int{1, 15} {
		txns = append(txns, newTxn(rng, dateIn(year, month, day), "PAYROLL DIRECT DEPOSIT", "Cleared", []posting{
			{"assets:checking", fmtAmount(salary)},
			{"income:salary", ""},
		}))
	}

	// Rent on the 1st
	txns = append(txns, newTxn(rng, dateIn(year, month, 1), "RENT PAYMENT", "Cleared", []posting{
		{"expenses:housing:rent", fmtAmount(rent)},
		{"assets:checking", ""},
	}))

	// Internet on the 5th
	txns = append(txns, newTxn(rng, dateIn(year, month, 5), "COMCAST INTERNET", "Cleared", []posting{
		{"expenses:utilities:internet", "$65.00"},
		{"assets:checking", ""},
	}))

	// Transfer to savings on the 10th
	txns = append(txns, newTxn(rng, dateIn(year, month, 10), "TRANSFER TO SAVINGS", "Cleared", []posting{
		{"assets:savings", "$500.00"},
		{"assets:checking", ""},
	}))

	// Electric bill mid-month (12th–18th)
	txns = append(txns, newTxn(rng,
		randomDate(rng, year, month, 12, 18),
		"CITY ELECTRIC COMPANY", "Cleared", []posting{
			{"expenses:utilities:electric", fmtAmount(randFloat(rng, 85, 125))},
			{"assets:checking", ""},
		}))

	// Gas fill-ups (2–4, from checking)
	for range 2 + rng.Intn(3) {
		txns = append(txns, newTxn(rng,
			randomDate(rng, year, month, 1, 28),
			pick(rng, gasStations), "Cleared", []posting{
				{"expenses:transportation:gas", fmtAmount(randFloat(rng, 42, 70))},
				{"assets:checking", ""},
			}))
	}

	// Grocery runs (4–8, from checking)
	for range 4 + rng.Intn(5) {
		txns = append(txns, newTxn(rng,
			randomDate(rng, year, month, 1, 28),
			pick(rng, groceryStores), "Cleared", []posting{
				{"expenses:food:groceries", fmtAmount(randFloat(rng, 45, 190))},
				{"assets:checking", ""},
			}))
	}

	// Streaming subscriptions on credit card (fixed dates)
	txns = append(txns, newTxn(rng, dateIn(year, month, 15), "NETFLIX.COM", "Cleared", []posting{
		{"expenses:entertainment:streaming", "$15.99"},
		{"liabilities:credit-card", ""},
	}))
	txns = append(txns, newTxn(rng, dateIn(year, month, 20), "SPOTIFY PREMIUM", "Cleared", []posting{
		{"expenses:entertainment:streaming", "$9.99"},
		{"liabilities:credit-card", ""},
	}))

	// Restaurant meals on credit card (4–8)
	for range 4 + rng.Intn(5) {
		txns = append(txns, newTxn(rng,
			randomDate(rng, year, month, 1, 28),
			pick(rng, restaurants), "Cleared", []posting{
				{"expenses:food:restaurants", fmtAmount(randFloat(rng, 12, 85))},
				{"liabilities:credit-card", ""},
			}))
	}

	// Shopping on credit card (1–3)
	for range 1 + rng.Intn(3) {
		txns = append(txns, newTxn(rng,
			randomDate(rng, year, month, 1, 28),
			pick(rng, shoppingPlaces), "Cleared", []posting{
				{"expenses:shopping", fmtAmount(randFloat(rng, 20, 200))},
				{"liabilities:credit-card", ""},
			}))
	}

	// Credit card payment on the 25th — sum all credit-card-backed expenses.
	var ccCharged float64
	for _, t := range txns {
		onCC := false
		for _, p := range t.postings {
			if p.account == "liabilities:credit-card" {
				onCC = true
				break
			}
		}
		if !onCC {
			continue
		}
		for _, p := range t.postings {
			if strings.HasPrefix(p.account, "expenses:") && p.amount != "" {
				var v float64
				fmt.Sscanf(p.amount, "$%f", &v)
				ccCharged += v
			}
		}
	}
	if ccCharged > 0 {
		txns = append(txns, newTxn(rng, dateIn(year, month, 25), "AMEX PAYMENT", "Cleared", []posting{
			{"liabilities:credit-card", fmtAmount(ccCharged)},
			{"assets:checking", ""},
		}))
	}

	// Savings interest on the last day
	txns = append(txns, newTxn(rng,
		dateIn(year, month, lastDay(year, month)),
		"SAVINGS INTEREST CREDIT", "Cleared", []posting{
			{"assets:savings", fmtAmount(randFloat(rng, 2.5, 6.0))},
			{"income:interest", ""},
		}))

	sort.Slice(txns, func(i, j int) bool {
		return txns[i].date.Before(txns[j].date)
	})
	return txns
}

// ── static file content ───────────────────────────────────────────────────────

const configTOML = `[server]
port = 8080

[[users]]
name = "admin"
role = "admin"
passphrase_hash = "argon2id$v=19$m=65536,t=1,p=1$ZmFrZXNhbHQ$ZmFrZWhhc2g="

[[users]]
name = "viewer"
role = "viewer"
passphrase_hash = "argon2id$v=19$m=65536,t=1,p=1$ZmFrZXNhbHQ$ZmFrZWhhc2g="

[[bank_profiles]]
name = "Chase Checking"
rules_file = "rules/chase-checking.rules"

[[bank_profiles]]
name = "Chase Savings"
rules_file = "rules/chase-savings.rules"

[[bank_profiles]]
name = "Amex Credit Card"
rules_file = "rules/amex-credit.rules"
`

const accountsJournal = `account assets:checking
account assets:savings
account liabilities:credit-card
account expenses:food:groceries
account expenses:food:restaurants
account expenses:housing:rent
account expenses:transportation:gas
account expenses:utilities:electric
account expenses:utilities:internet
account expenses:entertainment:streaming
account expenses:shopping
account expenses:healthcare
account expenses:misc
account income:salary
account income:interest
account equity:opening-balances
`

const chaseCheckingRules = `skip 1
fields date, description, amount
date-format %Y-%m-%d
account1 assets:checking
currency $

if PAYROLL DIRECT DEPOSIT
  account2 income:salary

if RENT PAYMENT
  account2 expenses:housing:rent

if COMCAST INTERNET
  account2 expenses:utilities:internet

if TRANSFER TO SAVINGS
  account2 assets:savings

if CITY ELECTRIC
  account2 expenses:utilities:electric

if SHELL
  account2 expenses:transportation:gas

if CHEVRON
  account2 expenses:transportation:gas

if BP GAS
  account2 expenses:transportation:gas

if EXXON
  account2 expenses:transportation:gas

if CIRCLE K
  account2 expenses:transportation:gas

if WHOLE FOODS
  account2 expenses:food:groceries

if KROGER
  account2 expenses:food:groceries

if TRADER JOES
  account2 expenses:food:groceries

if SAFEWAY
  account2 expenses:food:groceries

if ALDI
  account2 expenses:food:groceries

if PUBLIX
  account2 expenses:food:groceries

if SPROUTS
  account2 expenses:food:groceries

if AMEX PAYMENT
  account2 liabilities:credit-card
`

const chaseSavingsRules = `skip 1
fields date, description, amount
date-format %Y-%m-%d
account1 assets:savings
currency $

if TRANSFER FROM CHECKING
  account2 assets:checking

if SAVINGS INTEREST
  account2 income:interest
`

const amexCreditRules = `skip 1
fields date, description, amount
date-format %Y-%m-%d
account1 liabilities:credit-card
currency $

if NETFLIX
  account2 expenses:entertainment:streaming

if SPOTIFY
  account2 expenses:entertainment:streaming

if CHIPOTLE
  account2 expenses:food:restaurants

if MCDONALDS
  account2 expenses:food:restaurants

if STARBUCKS
  account2 expenses:food:restaurants

if PANERA
  account2 expenses:food:restaurants

if OLIVE GARDEN
  account2 expenses:food:restaurants

if PIZZA
  account2 expenses:food:restaurants

if THAI
  account2 expenses:food:restaurants

if SUSHI
  account2 expenses:food:restaurants

if UBER EATS
  account2 expenses:food:restaurants

if DOORDASH
  account2 expenses:food:restaurants

if TACO BELL
  account2 expenses:food:restaurants

if SUBWAY
  account2 expenses:food:restaurants

if FIVE GUYS
  account2 expenses:food:restaurants

if AMAZON
  account2 expenses:shopping

if TARGET
  account2 expenses:shopping

if BEST BUY
  account2 expenses:shopping

if COSTCO
  account2 expenses:shopping

if HOME DEPOT
  account2 expenses:shopping

if NORDSTROM
  account2 expenses:shopping

if WALMART
  account2 expenses:shopping

if WALGREENS
  account2 expenses:healthcare

if CVS PHARMACY
  account2 expenses:healthcare
`
