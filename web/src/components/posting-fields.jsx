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
    <div ref={wrapperRef} style={{ position: "relative", flex: 1 }}>
      <input
        type="text"
        placeholder="Account"
        value={value}
        onInput={handleInput}
        onFocus={() => {
          if (value.length > 0 && filtered.length > 0) setShowSuggestions(true);
        }}
      />
      {showSuggestions && filtered.length > 0 && (
        <ul
          style={{
            position: "absolute",
            top: "100%",
            left: 0,
            right: 0,
            zIndex: 10,
            background: "var(--pico-background-color)",
            border: "1px solid var(--pico-muted-border-color)",
            borderRadius: "4px",
            maxHeight: "12rem",
            overflowY: "auto",
            listStyle: "none",
            padding: 0,
            margin: 0,
          }}
        >
          {filtered.map((a) => (
            <li
              key={a.fullName}
              onClick={() => select(a.fullName)}
              style={{
                padding: "0.5rem 0.75rem",
                cursor: "pointer",
              }}
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
    <div>
      {postings.map((p, i) => (
        <div key={i} style={{ display: "flex", gap: "0.5rem", alignItems: "start", marginBottom: "0.5rem" }}>
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
            style={{ flex: "0 0 8rem" }}
          />
          <button
            class="outline secondary"
            onClick={() => removeRow(i)}
            disabled={postings.length <= 2}
            style={{ flex: "0 0 auto", padding: "0.25rem 0.5rem" }}
            type="button"
          >
            &times;
          </button>
        </div>
      ))}
      <button class="outline" onClick={addRow} type="button" style={{ marginTop: "0.5rem" }}>
        + Add posting
      </button>
    </div>
  );
}
