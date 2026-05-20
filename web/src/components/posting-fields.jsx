import { X, Plus, Tag, Scale } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Combobox } from "@/components/ui/combobox";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export function AccountInput({ value, onChange, accounts, placeholder = "Account", id, className }) {
  const options = (accounts || []).map((a) => a.fullName);
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
      className={className}
    />
  );
}

function CostFields({ cost, onChange, onRemove }) {
  const c = cost || { commodity: "", quantity: "", isTotal: false };
  return (
    <div className="ml-4 flex items-center gap-2 border-l pl-3">
      <button
        type="button"
        onClick={() => onChange({ ...c, isTotal: !c.isTotal })}
        title={c.isTotal ? "Total price (@@) — click to switch to per-unit" : "Per-unit price (@) — click to switch to total"}
        className="h-8 shrink-0 select-none rounded border border-input bg-background px-2 font-mono text-xs hover:bg-accent"
      >
        {c.isTotal ? "@@" : "@"}
      </button>
      <Input
        type="text"
        placeholder="0.00"
        value={c.quantity}
        onInput={(e) => onChange({ ...c, quantity: e.target.value })}
        className="w-20 shrink-0 sm:w-24"
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
              className="shrink-0"
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

function AssertionFields({ assertion, onChange, onRemove }) {
  const a = assertion || { commodity: "", quantity: "" };
  return (
    <div className="ml-4 flex items-center gap-2 border-l pl-3">
      <span className="h-8 shrink-0 select-none rounded border border-input bg-background px-2 font-mono text-xs leading-8">
        =
      </span>
      <Input
        type="text"
        placeholder="0.00"
        value={a.quantity}
        onInput={(e) => onChange({ ...a, quantity: e.target.value })}
        className="w-20 shrink-0 sm:w-24"
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
              className="shrink-0"
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
        return (
          <div key={i} className="flex flex-col gap-1">
            <div className="flex items-start gap-2">
              <AccountInput
                value={p.account}
                onChange={(v) => update(i, { account: v })}
                accounts={accounts}
              />
              <Input
                type="text"
                placeholder="USD"
                value={p.commodity}
                onInput={(e) => update(i, { commodity: e.target.value })}
                className="w-16 shrink-0"
              />
              <Input
                type="text"
                placeholder={isLast(i) ? "Auto" : "0.00"}
                value={p.quantity}
                onInput={(e) => update(i, { quantity: e.target.value })}
                className="w-20 shrink-0 sm:w-24"
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
                        className="shrink-0"
                      >
                        <Tag />
                      </Button>
                    }
                  />
                  <TooltipContent>Add price</TooltipContent>
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
                        className="shrink-0"
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
                className="shrink-0"
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
      <Button variant="ghost" size="sm" onClick={addRow} type="button">
        <Plus data-icon="inline-start" /> Add posting
      </Button>
    </div>
  );
}

// toPostingInput normalizes a UI posting row to a PostingInput proto payload:
// trims whitespace and drops the cost field if it has no quantity, so the
// backend doesn't try to write an `@` annotation with an empty price.
export function toPostingInput(p) {
  const out = {
    account: (p.account || "").trim(),
    commodity: (p.commodity || "").trim(),
    quantity: (p.quantity || "").trim(),
  };
  if (p.cost && p.cost.quantity && String(p.cost.quantity).trim()) {
    out.cost = {
      commodity: (p.cost.commodity || "").trim(),
      quantity: String(p.cost.quantity).trim(),
      isTotal: !!p.cost.isTotal,
    };
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
