import { Amount } from "@/components/amount.jsx";
import { Section } from "@/components/page.jsx";
import { SERIES_COLORS, foldToOther } from "@/lib/chart-colors";

// AllocationChart is one stacked horizontal bar (categorical colors, largest
// first, folded into "Other" past the palette size) over a legend list that
// always shows every holding. With mixed currencies the values can't share
// one bar, so only the list is shown, with percentages within each currency.
export function AllocationChart({ holdings, currencyTotals }) {
  const valued = holdings.filter((h) => h.currentValue);
  const mixedCurrencies = currencyTotals.length > 1;

  if (valued.length === 0) {
    return (
      <Section title="Allocation">
        <p className="text-control text-muted-foreground">
          No valued holdings available.
        </p>
      </Section>
    );
  }

  const items = valued.map((h) => ({
    symbol: h.symbol,
    value: parseFloat(h.currentValue.quantity),
    amount: h.currentValue,
    portfolioPct: h.portfolioPct,
  }));
  const segments = foldToOther(
    [...items].sort((a, b) => b.value - a.value),
    { nameKey: "symbol", valueKey: "value" },
  ).map((d, i) => ({ ...d, color: SERIES_COLORS[i] }));
  const colorBySymbol = Object.fromEntries(segments.map((d) => [d.symbol, d.color]));
  const total = segments.reduce((sum, d) => sum + Math.max(d.value, 0), 0);
  const pctOf = (v) => (total > 0 ? (Math.max(v, 0) / total) * 100 : 0);
  const commodity = items[0].amount.commodity;

  return (
    <Section title="Allocation">
      {mixedCurrencies ? (
        <p className="text-[12.5px] text-faint">
          Holdings span multiple currencies — percentages are within each
          currency's total.
        </p>
      ) : (
        <div
          data-slot="allocation-bar"
          role="img"
          aria-label={`Allocation: ${segments.map((d) => `${d.symbol} ${pctOf(d.value).toFixed(1)}%`).join(", ")}`}
          className="flex h-2.5 w-full gap-px overflow-hidden rounded-[2px]"
        >
          {segments.map((d) => (
            <div
              key={d.symbol}
              className="h-full min-w-px"
              style={{ width: `${pctOf(d.value)}%`, backgroundColor: d.color }}
              title={`${d.symbol}: ${pctOf(d.value).toFixed(1)}%`}
            />
          ))}
        </div>
      )}
      <ul data-slot="allocation-legend" className="flex flex-col">
        {[...items].sort((a, b) => b.value - a.value).map((item) => (
          <li key={item.symbol} className="flex items-center gap-2 py-1">
            {!mixedCurrencies && (
              <span
                aria-hidden="true"
                className="size-2 shrink-0 rounded-[2px]"
                style={{ backgroundColor: colorBySymbol[item.symbol] ?? colorBySymbol.Other }}
              />
            )}
            <span className="min-w-0 flex-1 truncate font-mono text-code font-medium">
              {item.symbol}
            </span>
            <Amount value={item.amount} />
            <span className="w-12 text-right text-[12px] tabular-nums text-faint">
              {item.portfolioPct > 0 ? `${item.portfolioPct.toFixed(1)}%` : ""}
            </span>
          </li>
        ))}
      </ul>
      {!mixedCurrencies && segments.some((d) => d.isOther) && (
        <p className="text-[12.5px] text-faint">
          Smaller holdings are grouped as Other in the bar
          (<Amount value={segments.at(-1).value} commodity={commodity} />).
        </p>
      )}
    </Section>
  );
}
