import { cn } from "@/lib/utils";

// Segmented is a small exclusive toggle ("1Y | 2Y | 5Y | All"): a labelled
// group of buttons with aria-pressed on the selected one.
// options: [{ value, label }]; value: the selected value.
export function Segmented({ options, value, onChange, label, className }) {
  return (
    <div
      role="group"
      aria-label={label}
      data-slot="segmented"
      className={cn("inline-flex gap-0.5 rounded-md border border-input bg-background p-0.5", className)}
    >
      {options.map((opt) => {
        const selected = opt.value === value;
        return (
          <button
            key={opt.value}
            type="button"
            aria-pressed={selected}
            onClick={() => onChange(opt.value)}
            className={cn(
              "touch-target h-6 min-w-9 rounded-[4px] px-2.5 text-[12.5px] text-muted-foreground outline-none transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring",
              selected && "bg-muted font-medium text-foreground ring-1 ring-border ring-inset",
            )}
          >
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}
