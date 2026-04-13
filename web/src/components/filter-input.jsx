import { useState, useEffect, useRef } from "react";

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
    <label class="input input-bordered flex items-center gap-2 mb-4 w-full">
      <svg class="h-4 w-4 opacity-50 shrink-0" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="11" cy="11" r="8" /><path d="m21 21-4.35-4.35" />
      </svg>
      <input
        type="search"
        placeholder="Filter (hledger query)..."
        value={local}
        onInput={handleInput}
        class="grow"
      />
    </label>
  );
}
