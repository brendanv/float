import { useState } from "react";
import { X, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";

export function AccountInput({ value, onChange, accounts, placeholder = "Account" }) {
  const [open, setOpen] = useState(false);
  const [filtered, setFiltered] = useState([]);

  function handleInput(e) {
    const v = e.target.value;
    onChange(v);
    if (v.length > 0 && accounts) {
      const lower = v.toLowerCase();
      setFiltered(
        accounts
          .filter((a) => a.fullName.toLowerCase().includes(lower))
          .slice(0, 8)
      );
      setOpen(true);
    } else {
      setOpen(false);
    }
  }

  function select(fullName) {
    onChange(fullName);
    setOpen(false);
  }

  return (
    <Popover open={open && filtered.length > 0} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Input
            type="text"
            placeholder={placeholder}
            value={value}
            onInput={handleInput}
            onFocus={() => {
              if (accounts && value.length === 0) {
                setFiltered(accounts.slice(0, 8));
                setOpen(true);
              } else if (value.length > 0 && filtered.length > 0) {
                setOpen(true);
              }
            }}
            className="flex-1"
          />
        }
      />
      <PopoverContent align="start" className="w-(--anchor-width) max-h-48 overflow-y-auto p-1" onOpenAutoFocus={(e) => e.preventDefault()}>
        {filtered.map((a) => (
          <button
            key={a.fullName}
            type="button"
            onClick={() => select(a.fullName)}
            className="flex w-full rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent hover:text-accent-foreground"
          >
            {a.fullName}
          </button>
        ))}
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
