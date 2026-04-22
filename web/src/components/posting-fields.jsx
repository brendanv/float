import { useState } from "react";
import { X, Plus, Check, ChevronsUpDown } from "lucide-react";
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

export function PostingFields({ postings, onChange, accounts }) {
  function update(index, field, value) {
    const next = postings.map((p, i) =>
      i === index ? { ...p, [field]: value } : p
    );
    onChange(next);
  }

  function addRow() {
    onChange([...postings, { account: "", amount: "" }]);
  }

  function removeRow(index) {
    if (postings.length <= 2) return;
    onChange(postings.filter((_, i) => i !== index));
  }

  return (
    <div className="flex flex-col gap-2">
      {postings.map((p, i) => (
        <div key={i} className="flex items-start gap-2">
          <AccountInput
            value={p.account}
            onChange={(v) => update(i, "account", v)}
            accounts={accounts}
          />
          <Input
            type="text"
            placeholder={i === postings.length - 1 ? "Auto-balance" : "Amount"}
            value={p.amount}
            onInput={(e) => update(i, "amount", e.target.value)}
            className="w-24 shrink-0 sm:w-32"
          />
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
      ))}
      <Button variant="ghost" size="sm" onClick={addRow} type="button">
        <Plus data-icon="inline-start" /> Add posting
      </Button>
    </div>
  );
}
