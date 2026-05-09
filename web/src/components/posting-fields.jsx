import { useState } from "react";
import { X, Plus, ChevronsUpDown, Tag } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

export function AccountInput({ value, onChange, accounts, placeholder = "Account" }) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");

  const exactMatch = (accounts || []).some(
    (a) => a.fullName.toLowerCase() === search.toLowerCase()
  );

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <button
            type="button"
            role="combobox"
            aria-expanded={open}
            className={cn(
              "flex h-8 w-full items-center justify-between rounded-none border border-input bg-background px-3 text-xs outline-offset-0 outline-none hover:bg-background focus-visible:outline-[3px]",
              !value && "text-muted-foreground",
            )}
          >
            <span className="truncate">{value || placeholder}</span>
            <ChevronsUpDown className="ml-1 size-3.5 shrink-0 opacity-50" />
          </button>
        }
      />
      <PopoverContent align="start" className="w-(--anchor-width) p-0">
        <Command>
          <CommandInput
            placeholder={`Search ${placeholder.toLowerCase()}...`}
            value={search}
            onValueChange={setSearch}
          />
          <CommandList>
            <CommandEmpty>
              {search ? (
                <button
                  type="button"
                  className="w-full px-2 py-1.5 text-left text-xs hover:bg-accent hover:text-accent-foreground"
                  onClick={() => {
                    onChange(search);
                    setSearch("");
                    setOpen(false);
                  }}
                >
                  Use "<span className="font-medium">{search}</span>"
                </button>
              ) : (
                "No account found."
              )}
            </CommandEmpty>
            <CommandGroup>
              {!exactMatch && search && (
                <CommandItem
                  value={`__use__${search}`}
                  onSelect={() => {
                    onChange(search);
                    setSearch("");
                    setOpen(false);
                  }}
                  className="text-muted-foreground"
                >
                  Use "<span className="font-medium text-foreground">{search}</span>"
                </CommandItem>
              )}
              {(accounts || []).map((a) => (
                <CommandItem
                  key={a.fullName}
                  value={a.fullName}
                  onSelect={() => {
                    onChange(a.fullName === value ? "" : a.fullName);
                    setSearch("");
                    setOpen(false);
                  }}
                  data-checked={value === a.fullName ? "true" : undefined}
                >
                  {a.fullName}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
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
  return out;
}
