import { clsx } from "clsx";
import { extendTailwindMerge } from "tailwind-merge"

// The Waterline type scale (`text-label`, `text-body`, …) is defined in
// style.css. Without registering it, tailwind-merge reads `text-label` as a
// color and drops it next to `text-faint`.
const twMerge = extendTailwindMerge({
  extend: {
    theme: {
      text: ["label", "code", "control", "body", "heading", "title", "figure", "display"],
    },
  },
});

export function cn(...inputs) {
  return twMerge(clsx(inputs));
}

// ROW_ACTION hides a table row's secondary actions (Edit, Delete, …) until
// the row (`group/row`) is hovered or focused, on devices that can hover;
// touch devices always show them.
export const ROW_ACTION =
  "[@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-hover/row:opacity-100 [@media(hover:hover)]:group-focus-within/row:opacity-100 focus-visible:opacity-100 transition-opacity";
