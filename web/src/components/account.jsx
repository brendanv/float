import { cn } from "@/lib/utils";
import { accountType, splitAccount } from "@/lib/accounts.js";

const GLYPH_CLASS = {
  asset: "bg-acct-asset",
  liability: "bg-acct-liability",
  income: "bg-acct-income",
  expense: "bg-acct-expense",
  equity: "bg-acct-equity",
  other: "bg-acct-other",
};

// AccountGlyph is the 7px square that marks an account's type. Decorative:
// the account name next to it carries the meaning.
export function AccountGlyph({ type = "other", className }) {
  return (
    <span
      aria-hidden="true"
      data-slot="account-glyph"
      data-type={type}
      className={cn("inline-block size-[7px] shrink-0 rounded-[2px]", GLYPH_CLASS[type] ?? GLYPH_CLASS.other, className)}
    />
  );
}

// Account renders an account name. `variant`:
// - "leaf": last segment only (dense rows).
// - "path": parents muted, leaf in ink.
// - "full": the raw path in mono (editors, inputs).
// `title` is always the full path. `glyph` defaults on except for "full";
// `type` overrides the type inferred from the top-level segment.
export function Account({ name, variant = "leaf", glyph, type, className, ...props }) {
  const full = String(name ?? "");
  const { parents, leaf } = splitAccount(full);
  const showGlyph = glyph ?? variant !== "full";

  return (
    <span
      data-slot="account"
      title={full}
      className={cn(
        "inline-flex min-w-0 items-center gap-1.5 whitespace-nowrap",
        variant === "full" && "font-mono text-code",
        className,
      )}
      {...props}
    >
      {showGlyph && <AccountGlyph type={type ?? accountType(full)} />}
      <span className="min-w-0 truncate">
        {variant === "path" && parents && <span className="text-faint">{parents}</span>}
        {variant === "full" ? full : leaf}
      </span>
    </span>
  );
}

const COUNT_LABEL = /^(\d+|various|multiple)\s+accounts?$/i;

function FlowEnd({ value, variant }) {
  if (value == null || value === "") return null;
  if (typeof value !== "string") return value;
  if (COUNT_LABEL.test(value)) return <span className="text-faint">{value}</span>;
  return <Account name={value} variant={variant} />;
}

// Flow renders "from → to" for a transaction's accounts. Each end is an
// account path (shown as <Account>), a count string such as "3 accounts",
// or any React node.
export function Flow({ from, to, variant = "leaf", className, ...props }) {
  return (
    <span
      data-slot="flow"
      className={cn("inline-flex min-w-0 items-center gap-[7px] whitespace-nowrap text-muted-foreground", className)}
      {...props}
    >
      <FlowEnd value={from} variant={variant} />
      {from && to && <span aria-hidden="true" className="text-faint">→</span>}
      {from && to && <span className="sr-only">to</span>}
      <FlowEnd value={to} variant={variant} />
    </span>
  );
}
