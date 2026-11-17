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

  // The primary line is the amount as hledger gave it; a cost (@/@@) or lot
  // basis ({…}) goes on a mono second line, as written. Converting to cost
  // is hledger's job (-B), so it isn't multiplied out here.
  const primary = formatCurrency(quantity, commodity, { whole });
  const text = signed && parseFloat(quantity) > 0 && /[1-9]/.test(primary) ? `+${primary}` : primary;
  const annotation = (formatCostBasis(costBasis) + formatCost(cost)).trim();
  const detail = annotation || null;
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
// - commodity, cost, costBasis: used with a bare quantity. With a cost or a
//   lot cost basis, the primary line stays the amount itself ("10 AAPL") and
//   a mono second line shows the annotation ("@ $175.00", "{$175.00}").
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
