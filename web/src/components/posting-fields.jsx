import { useState, useRef, useEffect } from "react";
import { X, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export function AccountInput({ value, onChange, accounts, placeholder = "Account" }) {
  const [showSuggestions, setShowSuggestions] = useState(false);
  const [filtered, setFiltered] = useState([]);
  const wrapperRef = useRef(null);

  useEffect(() => {
    function handleClickOutside(e) {
      if (wrapperRef.current && !wrapperRef.current.contains(e.target)) {
        setShowSuggestions(false);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

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
      setShowSuggestions(true);
    } else {
      setShowSuggestions(false);
    }
  }

  function select(fullName) {
    onChange(fullName);
    setShowSuggestions(false);
  }

  return (
    <div ref={wrapperRef} className="relative flex-1">
      <Input
        type="text"
        placeholder={placeholder}
        value={value}
        onInput={handleInput}
        onFocus={() => {
          if (accounts && value.length === 0) {
            setFiltered(accounts.slice(0, 8));
            setShowSuggestions(true);
          } else if (value.length > 0 && filtered.length > 0) {
            setShowSuggestions(true);
          }
        }}
      />
      {showSuggestions && filtered.length > 0 && (
        <ul className="absolute left-0 right-0 top-full z-50 mt-1 max-h-48 overflow-y-auto rounded-lg border bg-popover p-1 shadow-md ring-1 ring-foreground/10">
          {filtered.map((a) => (
            <li
              key={a.fullName}
              onClick={() => select(a.fullName)}
              className="cursor-pointer rounded-md px-2 py-1.5 text-sm hover:bg-accent hover:text-accent-foreground"
            >
              {a.fullName}
            </li>
          ))}
        </ul>
      )}
    </div>
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
    <div className="space-y-2">
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
        <Plus /> Add posting
      </Button>
    </div>
  );
}
