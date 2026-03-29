import { useState, useRef, useEffect } from "preact/hooks";

function AccountInput({ value, onChange, accounts }) {
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
    <div ref={wrapperRef} class="relative flex-1">
      <input
        type="text"
        placeholder="Account"
        value={value}
        onInput={handleInput}
        onFocus={() => {
          if (value.length > 0 && filtered.length > 0) setShowSuggestions(true);
        }}
        class="input input-bordered input-sm w-full"
      />
      {showSuggestions && filtered.length > 0 && (
        <ul class="absolute top-full left-0 right-0 z-10 bg-base-100 border border-base-300 rounded-box shadow-lg max-h-48 overflow-y-auto p-1">
          {filtered.map((a) => (
            <li
              key={a.fullName}
              onClick={() => select(a.fullName)}
              class="px-3 py-2 text-sm cursor-pointer rounded hover:bg-base-200"
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
    <div class="space-y-2">
      {postings.map((p, i) => (
        <div key={i} class="flex gap-2 items-start">
          <AccountInput
            value={p.account}
            onChange={(v) => update(i, "account", v)}
            accounts={accounts}
          />
          <input
            type="text"
            placeholder={i === postings.length - 1 ? "Auto-balance" : "Amount"}
            value={p.amount}
            onInput={(e) => update(i, "amount", e.target.value)}
            class="input input-bordered input-sm w-24 sm:w-32 shrink-0"
          />
          <button
            class="btn btn-ghost btn-sm shrink-0"
            onClick={() => removeRow(i)}
            disabled={postings.length <= 2}
            type="button"
          >
            &times;
          </button>
        </div>
      ))}
      <button class="btn btn-ghost btn-sm" onClick={addRow} type="button">
        + Add posting
      </button>
    </div>
  );
}
