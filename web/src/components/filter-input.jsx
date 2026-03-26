import { useState, useEffect, useRef } from "preact/hooks";

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
    <input
      type="search"
      placeholder="Filter (hledger query)..."
      value={local}
      onInput={handleInput}
      style={{ marginBottom: "1rem" }}
    />
  );
}
