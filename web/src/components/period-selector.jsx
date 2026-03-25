import { monthName } from "../format.js";

export function PeriodSelector({ year, month, onChange }) {
  function prev() {
    if (month === 1) {
      onChange(year - 1, 12);
    } else {
      onChange(year, month - 1);
    }
  }

  function next() {
    if (month === 12) {
      onChange(year + 1, 1);
    } else {
      onChange(year, month + 1);
    }
  }

  return (
    <div style={{ display: "flex", alignItems: "center", gap: "1rem", marginBottom: "1rem" }}>
      <button class="outline secondary" onClick={prev} style={{ padding: "0.25rem 0.75rem" }}>
        &lsaquo;
      </button>
      <strong style={{ minWidth: "10rem", textAlign: "center" }}>
        {monthName(month)} {year}
      </strong>
      <button class="outline secondary" onClick={next} style={{ padding: "0.25rem 0.75rem" }}>
        &rsaquo;
      </button>
    </div>
  );
}
