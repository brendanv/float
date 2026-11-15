// Categorical series colors (CSS variables from style.css, so they follow
// the light/dark theme). Assign in this order and never cycle: callers with
// more categories than slots fold the remainder into an "Other" bucket.
export const SERIES_COLORS = [1, 2, 3, 4, 5, 6, 7, 8].map((i) => `var(--chart-${i})`);

// foldToOther keeps the first `SERIES_COLORS.length - 1` items and sums the
// rest into one `{ [nameKey]: "Other" }` item when there are more items than
// colors. Items must already be sorted by importance.
export function foldToOther(items, { nameKey = "name", valueKey = "amount" } = {}) {
  if (items.length <= SERIES_COLORS.length) return items;
  const keep = SERIES_COLORS.length - 1;
  const rest = items.slice(keep).reduce((sum, item) => sum + item[valueKey], 0);
  return [...items.slice(0, keep), { [nameKey]: "Other", [valueKey]: rest, isOther: true }];
}
