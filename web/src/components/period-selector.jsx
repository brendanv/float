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
    <div class="flex items-center mb-4">
      <div class="join">
        <button class="btn btn-ghost btn-sm join-item" onClick={prev}>
          &lsaquo;
        </button>
        <button class="btn btn-ghost btn-sm join-item pointer-events-none min-w-32 sm:min-w-40 font-semibold">
          {monthName(month)} {year}
        </button>
        <button class="btn btn-ghost btn-sm join-item" onClick={next}>
          &rsaquo;
        </button>
      </div>
    </div>
  );
}
