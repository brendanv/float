import { cn } from "@/lib/utils";
import { formatCost, formatCostBasis, formatCurrency } from "../format.js";

const SIZE_CLASS = {
  default: "",
  figure: "text-figure font-medium",
  display: "text-display font-medium",
};

const TONE_CLASS = {
  auto: "",
  ink: "text-foreground",
  inflow: "text-inflow",
  below: "text-below",
};

// Split "$11,220.00" into "$11,220" + ".00" so the cents can shrink at
// figure/display size. Only two-decimal (fiat) output is split.
const CENTS = /^(.*\d)(\.\d{2})$/;

function flipSign(quantity) {
  const q = String(quantity ?? "").trim();
  if (q === "" || parseFloat(q) === 0) return q;
  return q.startsWith("-") ? q.slice(1) : `-${q}`;
}

// costTotal is the amount at cost: @@ is already the total, @ is per unit.
// It keeps the sign of the units.
function costTotal(quantity, cost) {
  const units = parseFloat(quantity);
  const price = parseFloat(cost.quantity);
  if (isNaN(units) || isNaN(price)) return null;
  const total = cost.isTotal ? Math.abs(price) : Math.abs(units * price);
  return units < 0 ? -total : total;
}

function Figure({ text, split }) {
  const m = split ? CENTS.exec(text) : null;
  if (!m) return text;
  return (
    <>
      {m[1]}
      <span className="ml-px align-[0.62em] text-[0.5em] tracking-normal text-faint">{m[2]}</span>
    </>
  );
}

function SingleAmount({ value, commodity, cost, costBasis, size, tone, signed, negate, whole, className, ...props }) {
  let quantity = value;
  if (value && typeof value === "object") {
    quantity = value.quantity;
    commodity ??= value.commodity;
    cost ??= value.cost;
    costBasis ??= value.costBasis;
  }
  if (negate) quantity = flipSign(quantity);

  const hasCost = !!cost?.quantity;
  const hasLot = hasCost || !!formatCostBasis(costBasis);
  // Without an @ price, a lot cost basis ({$175.00}, per unit) still gives
  // the value at cost.
  const valuation = hasCost ? cost : costBasis?.cost?.quantity ? { ...costBasis.cost, isTotal: false } : null;
  const total = valuation ? costTotal(quantity, valuation) : null;
  const primary = total !== null
    ? formatCurrency(total, valuation.commodity, { whole })
    : formatCurrency(quantity, commodity, { whole });
  const shownQuantity = total !== null ? total : parseFloat(quantity);
  const text = signed && shownQuantity > 0 && /[1-9]/.test(primary) ? `+${primary}` : primary;
  const detail = hasLot
    ? formatCurrency(quantity, commodity) + formatCostBasis(costBasis) + formatCost(cost)
    : null;
  const split = size === "figure" || size === "display";

  return (
    <span
      data-slot="amount"
      className={cn(
        "font-sans whitespace-nowrap tabular-nums",
        detail ? "inline-flex flex-col items-end" : "inline-block",
        SIZE_CLASS[size] ?? "",
        TONE_CLASS[tone] ?? "",
        className,
      )}
      {...props}
    >
      {detail ? (
        <>
          <span><Figure text={text} split={split} /></span>
          <span data-slot="amount-detail" className="font-mono text-[11.5px] leading-4 font-normal tracking-normal text-faint">
            {detail}
          </span>
        </>
      ) : (
        <Figure text={text} split={split} />
      )}
    </span>
  );
}

// Amount renders a money/commodity amount in the sans font with tabular
// figures. It formats through format.js (formatCurrency, formatCost,
// formatCostBasis) and never formats by hand.
//
// - value: a quantity (number or string), an amount object
//   ({ quantity, commodity, cost?, costBasis? }), or an array of amount
//   objects (multi-commodity balances render one per line; zeros are dropped
//   unless all are zero; an empty array renders "—").
// - commodity, cost, costBasis: used with a bare quantity. With a cost (or a
//   lot cost basis with a per-unit cost) the primary line is the value at
//   cost and a mono second line shows the lot ("10 AAPL @ $175.00",
//   "10 AAPL {$175.00}").
// - size: "default" (inherits the surrounding size) | "figure" (26px) |
//   "display" (44px). Cents shrink at figure/display.
// - tone: "auto" | "ink" leave the amount ink; "inflow" for income arriving;
//   "below" for negative/owed balances. Callers decide the tone.
// - signed: prefix "+" on positive amounts.
// - negate: flip the sign before display (liabilities shown as owed,
//   revenue shown as positive).
export function Amount({ value, size = "default", tone = "auto", className, ...props }) {
  if (Array.isArray(value)) {
    const nonZero = value.filter((a) => parseFloat(a?.quantity) !== 0);
    const shown = nonZero.length > 0 ? nonZero : value;
    if (shown.length === 0) {
      return <span data-slot="amount" className={cn("text-faint", className)}>—</span>;
    }
    if (shown.length === 1) {
      return <SingleAmount value={shown[0]} size={size} tone={tone} className={className} {...props} />;
    }
    return (
      <span data-slot="amount-list" className={cn("inline-flex flex-col items-end", className)}>
        {shown.map((a, i) => (
          <SingleAmount key={`${a.commodity}-${i}`} value={a} size={size} tone={tone} {...props} />
        ))}
      </span>
    );
  }
  return <SingleAmount value={value} size={size} tone={tone} className={className} {...props} />;
}
