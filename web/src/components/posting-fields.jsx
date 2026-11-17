import { X, Plus, Tag, Scale, Layers } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Combobox } from "@/components/ui/combobox";
import { filterAndSortAccountNames } from "@/lib/accounts";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

// Literal hledger syntax chips (@, { }, =) in front of a posting's extra
// fields.
const SYNTAX_CHIP =
  "inline-flex h-8 pointer-coarse:min-h-10 shrink-0 select-none items-center rounded-md border border-input px-2 font-mono text-code text-muted-foreground";

// Quiet icon buttons on a posting row: ink-3 until hovered.
const ROW_ICON = "shrink-0 text-faint hover:text-foreground";

export function AccountInput({
  value,
  onChange,
  accounts,
  placeholder = "Account",
  id,
  className,
  includePrefixes = [],
  excludePrefixes = [],
  includeAccountPrefixes = includePrefixes,
  excludeAccountPrefixes = excludePrefixes,
  includeClosed = false,
  clearable = false,
  clearLabel,
}) {
  const options = filterAndSortAccountNames(accounts, {
    includePrefixes: includeAccountPrefixes,
    excludePrefixes: excludeAccountPrefixes,
    includeClosed,
  });
  return (
    <Combobox
      id={id}
      value={value}
      onChange={onChange}
      options={options}
      placeholder={placeholder}
      searchPlaceholder={`Search ${placeholder.toLowerCase()}...`}
      emptyMessage="No account found."
      allowCustomValue
      clearable={clearable}
      clearLabel={clearLabel}
      className={className}
    />
  );
}

export function editablePostings(postings) {
  return (postings || []).filter((p) => !p.generated);
}

export function fieldsFromPosting(p) {
  const a = p.amounts?.[0];
  const ba = p.balanceAssertion;
  const cb = a?.costBasis;
  let cost = a ? a.cost : undefined;
  let lotCost = cb?.cost;
  if (p.lotRole === "acquire" && lotCost?.quantity) {
    if (cost?.isTotal) lotCost = undefined;
    else cost = undefined;
  }
  return {
    account: p.account,
    // Not editable in the UI, but carried through so an update doesn't drop
    // the posting's comment (and any posting tags such as date:).
    comment: p.comment ?? "",
    commodity: a ? a.commodity : "",
    quantity: a ? a.quantity : "",
    cost,
    lot: cb
      ? {
          date: cb.date ?? "",
          label: cb.label ?? "",
          costQuantity: lotCost?.quantity ?? "",
          costCommodity: lotCost?.commodity ?? "",
        }
      : undefined,
    balanceAssertion: ba?.amount ? { commodity: ba.amount.commodity, quantity: ba.amount.quantity } : undefined,
  };
}

function CostFields({ cost, onChange, onRemove }) {
  const c = cost || { commodity: "", quantity: "", isTotal: false };
  return (
    <div className="ml-3 flex items-center gap-2 border-l border-border pl-3">
      <button
        type="button"
        onClick={() => onChange({ ...c, isTotal: !c.isTotal })}
        title={c.isTotal ? "Total price (@@) — click to switch to per-unit" : "Per-unit price (@) — click to switch to total"}
        className={cn(SYNTAX_CHIP, "text-foreground hover:bg-muted pointer-coarse:min-w-10 justify-center")}
      >
        {c.isTotal ? "@@" : "@"}
      </button>
      <Input
        type="number"
        step="any"
        placeholder="0.00"
        value={c.quantity}
        onInput={(e) => onChange({ ...c, quantity: e.target.value })}
        className="w-20 shrink-0 text-right sm:w-24"
      />
      <Input
        type="text"
        placeholder="USD"
        value={c.commodity}
        onInput={(e) => onChange({ ...c, commodity: e.target.value })}
        className="w-16 shrink-0"
      />
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={onRemove}
              type="button"
              aria-label="Remove price"
              className={ROW_ICON}
            >
              <X />
            </Button>
          }
        />
        <TooltipContent>Remove price</TooltipContent>
      </Tooltip>
    </div>
  );
}

function LotFields({ lot, onChange, onRemove }) {
  const l = lot || { date: "", label: "", costQuantity: "", costCommodity: "" };
  return (
    <div className="ml-3 flex flex-wrap items-center gap-2 border-l border-border pl-3">
      <Tooltip>
        <TooltipTrigger
          render={
            <span className={SYNTAX_CHIP}>
              {"{ }"}
            </span>
          }
        />
        <TooltipContent>On a sale, date or label selects the lot. On a buy, cost sets the basis.</TooltipContent>
      </Tooltip>
      <Input
        type="date"
        value={l.date}
        onChange={(e) => onChange({ ...l, date: e.target.value })}
        className="w-36 shrink-0"
      />
      <Input
        type="text"
        placeholder="Label"
        value={l.label}
        onInput={(e) => onChange({ ...l, label: e.target.value })}
        className="w-24 shrink-0"
      />
      <Input
        type="number"
        step="any"
        placeholder="Cost"
        value={l.costQuantity}
        onInput={(e) => onChange({ ...l, costQuantity: e.target.value })}
        className="w-20 shrink-0 text-right sm:w-24"
      />
      <Input
        type="text"
        placeholder="USD"
        value={l.costCommodity}
        onInput={(e) => onChange({ ...l, costCommodity: e.target.value })}
        className="w-16 shrink-0"
      />
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={onRemove}
              type="button"
              aria-label="Remove lot"
              className={ROW_ICON}
            >
              <X />
            </Button>
          }
        />
        <TooltipContent>Remove lot</TooltipContent>
      </Tooltip>
    </div>
  );
}

function AssertionFields({ assertion, onChange, onRemove }) {
  const a = assertion || { commodity: "", quantity: "" };
  return (
    <div className="ml-3 flex items-center gap-2 border-l border-border pl-3">
      <span className={SYNTAX_CHIP}>
        =
      </span>
      <Input
        type="number"
        step="any"
        placeholder="0.00"
        value={a.quantity}
        onInput={(e) => onChange({ ...a, quantity: e.target.value })}
        className="w-20 shrink-0 text-right sm:w-24"
      />
      <Input
        type="text"
        placeholder="USD"
        value={a.commodity}
        onInput={(e) => onChange({ ...a, commodity: e.target.value })}
        className="w-16 shrink-0"
      />
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={onRemove}
              type="button"
              aria-label="Remove balance assertion"
              className={ROW_ICON}
            >
              <X />
            </Button>
          }
        />
        <TooltipContent>Remove balance assertion</TooltipContent>
      </Tooltip>
    </div>
  );
}

// PostingFields edits a transaction's posting rows. Balancing and amount
// inference are left to hledger on save.
export function PostingFields({ postings, onChange, accounts }) {
  function update(index, patch) {
    const next = postings.map((p, i) => (i === index ? { ...p, ...patch } : p));
    onChange(next);
  }

  function updateCost(index, cost) {
    update(index, { cost });
  }

  function removeCost(index) {
    const next = postings.map((p, i) => {
      if (i !== index) return p;
      const { cost: _, ...rest } = p;
      return rest;
    });
    onChange(next);
  }

  function updateLot(index, lot) {
    update(index, { lot });
  }

  function removeLot(index) {
    const next = postings.map((p, i) => {
      if (i !== index) return p;
      const { lot: _, ...rest } = p;
      return rest;
    });
    onChange(next);
  }

  function updateAssertion(index, ba) {
    update(index, { balanceAssertion: ba });
  }

  function removeAssertion(index) {
    const next = postings.map((p, i) => {
      if (i !== index) return p;
      const { balanceAssertion: _, ...rest } = p;
      return rest;
    });
    onChange(next);
  }

  function addRow() {
    onChange([...postings, { account: "", commodity: "", quantity: "" }]);
  }

  function removeRow(index) {
    if (postings.length <= 2) return;
    onChange(postings.filter((_, i) => i !== index));
  }

  const isLast = (i) => i === postings.length - 1;

  return (
    <div className="flex flex-col gap-2">
      {postings.map((p, i) => {
        const hasCost = !!p.cost;
        const hasAssertion = !!p.balanceAssertion;
        const hasLot = !!p.lot;
        return (
          <div key={i} className="flex flex-col gap-1">
            {/* Below sm the account gets its own line (with the remove button)
                and the amount fields wrap underneath; on wider screens it is
                one row. `order` keeps a single DOM tree for both layouts. */}
            <div className="flex flex-wrap items-start gap-2 sm:flex-nowrap">
              <AccountInput
                value={p.account}
                onChange={(v) => update(i, { account: v })}
                accounts={accounts}
                // The raw account path is mono once there is one.
                className={cn("basis-[calc(100%-2.5rem)] sm:flex-1", p.account && "font-mono text-code")}
              />
              <Input
                type="text"
                placeholder="USD"
                value={p.commodity}
                onInput={(e) => update(i, { commodity: e.target.value })}
                className="order-3 w-16 shrink-0 sm:order-none"
              />
              <Input
                type="number"
                step="any"
                placeholder={isLast(i) ? "Auto" : "0.00"}
                value={p.quantity}
                onInput={(e) => update(i, { quantity: e.target.value })}
                className="order-3 w-auto min-w-0 flex-1 text-right sm:order-none sm:w-28 sm:flex-none sm:shrink-0"
              />
              {!hasCost && (
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => updateCost(i, { commodity: "", quantity: "", isTotal: false })}
                        type="button"
                        aria-label="Add price"
                        className={cn(ROW_ICON, "order-3 sm:order-none")}
                      >
                        <Tag />
                      </Button>
                    }
                  />
                  <TooltipContent>Add price</TooltipContent>
                </Tooltip>
              )}
              {!hasLot && (
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => updateLot(i, { date: "", label: "", costQuantity: "", costCommodity: "" })}
                        type="button"
                        aria-label="Add lot / cost basis"
                        className={cn(ROW_ICON, "order-3 sm:order-none")}
                      >
                        <Layers />
                      </Button>
                    }
                  />
                  <TooltipContent>Add lot / cost basis</TooltipContent>
                </Tooltip>
              )}
              {!hasAssertion && (
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => updateAssertion(i, { commodity: "", quantity: "" })}
                        type="button"
                        aria-label="Add balance assertion"
                        className={cn(ROW_ICON, "order-3 sm:order-none")}
                      >
                        <Scale />
                      </Button>
                    }
                  />
                  <TooltipContent>Add balance assertion</TooltipContent>
                </Tooltip>
              )}
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => removeRow(i)}
                disabled={postings.length <= 2}
                type="button"
                aria-label="Remove posting"
                className={cn(ROW_ICON, "order-2 size-8 sm:order-none sm:size-7")}
              >
                <X />
              </Button>
            </div>
            {hasCost && (
              <CostFields
                cost={p.cost}
                onChange={(c) => updateCost(i, c)}
                onRemove={() => removeCost(i)}
              />
            )}
            {hasLot && (
              <LotFields
                lot={p.lot}
                onChange={(l) => updateLot(i, l)}
                onRemove={() => removeLot(i)}
              />
            )}
            {hasAssertion && (
              <AssertionFields
                assertion={p.balanceAssertion}
                onChange={(a) => updateAssertion(i, a)}
                onRemove={() => removeAssertion(i)}
              />
            )}
          </div>
        );
      })}
      <div>
        <Button variant="ghost" size="sm" onClick={addRow} type="button" className="-ml-2 text-muted-foreground">
          <Plus data-icon="inline-start" /> Add posting
        </Button>
      </div>
    </div>
  );
}

// toPostingInput normalizes a UI posting row to a PostingInput proto payload:
// trims whitespace and drops the cost field if it has no quantity, so the
// backend doesn't try to write an `@` annotation with an empty price. The
// posting comment is passed through untouched: UpdateTransaction replaces all
// postings, so omitting it would delete it.
export function toPostingInput(p) {
  const out = {
    account: (p.account || "").trim(),
    commodity: (p.commodity || "").trim(),
    quantity: (p.quantity || "").trim(),
  };
  if (p.comment) out.comment = p.comment;
  if (p.cost && p.cost.quantity && String(p.cost.quantity).trim()) {
    out.cost = {
      commodity: (p.cost.commodity || "").trim(),
      quantity: String(p.cost.quantity).trim(),
      isTotal: !!p.cost.isTotal,
    };
  }
  if (p.lot) {
    const date = (p.lot.date || "").trim();
    const label = (p.lot.label || "").trim();
    const costQuantity = String(p.lot.costQuantity ?? "").trim();
    if (date || label || costQuantity) {
      const costBasis = { date, label };
      if (costQuantity) {
        costBasis.cost = {
          commodity: (p.lot.costCommodity || "").trim(),
          quantity: costQuantity,
        };
      }
      out.costBasis = costBasis;
    }
  }
  if (p.balanceAssertion?.quantity && String(p.balanceAssertion.quantity).trim()) {
    out.balanceAssertion = {
      amount: {
        commodity: (p.balanceAssertion.commodity || "").trim(),
        quantity: String(p.balanceAssertion.quantity).trim(),
      },
    };
  }
  return out;
}
