import { cn } from "@/lib/utils";

export function Stats15({ title, items = [] }) {
  if (!items.length) return null;
  return (
    <div className="w-full">
      {title && (
        <h3 className="text-sm font-medium text-foreground">{title}</h3>
      )}
      <ul role="list" className={cn("divide-y divide-border text-sm", title && "mt-2")}>
        {items.map((item, index) => (
          <li key={index} className="flex items-center justify-between py-3">
            <span className="text-muted-foreground">{item.label}</span>
            <span className="flex items-center gap-3 tabular-nums">
              <span className="text-right font-medium text-foreground">
                {item.value}
              </span>
              <span className="h-5 w-px bg-border" aria-hidden="true" />
              <span
                className={cn(
                  "rounded px-1.5 py-1 text-center text-xs font-semibold w-15",
                  item.positive === true
                    ? "bg-emerald-50 text-emerald-600 dark:bg-emerald-400/10 dark:text-emerald-400"
                    : item.positive === false
                    ? "bg-red-50 text-red-600 dark:bg-red-400/10 dark:text-red-400"
                    : "bg-muted text-muted-foreground"
                )}>
                {item.percentage}
              </span>
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}
