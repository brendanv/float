import { useState, useEffect, useRef } from "react";
import { Search } from "lucide-react";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

export function FilterInput({ value, onChange }) {
  const [local, setLocal] = useState(value || "");
  const timerRef = useRef(null);

  useEffect(() => {
    setLocal(value || "");
  }, [value]);

  function handleInput(e) {
    const v = e.target.value;
    setLocal(v);
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => onChange(v), 300);
  }

  return (
    <div className="relative mb-4 w-full max-w-xl">
      <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-faint" />
      <Input
        type="search"
        aria-label="Filter (hledger query)"
        placeholder="Filter (hledger query)…"
        value={local}
        onInput={handleInput}
        // The value is literal hledger query syntax, so it is mono once typed.
        className={cn("pl-8", local && "font-mono text-code")}
      />
    </div>
  );
}
