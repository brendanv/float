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
