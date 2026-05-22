import { ArrowUp, ArrowDown, ArrowUpDown } from "lucide-react";
import { cn } from "@/lib/utils";

export function TableSortHeader({ column, children, align = "left", className }) {
  const sorted = column.getIsSorted();
  const Icon = sorted === "asc" ? ArrowUp : sorted === "desc" ? ArrowDown : ArrowUpDown;
  return (
    <button
      type="button"
      onClick={() => column.toggleSorting(sorted === "asc")}
      className={cn(
        "inline-flex cursor-pointer select-none items-center gap-1 rounded px-1 -mx-1 py-0.5 transition-colors hover:text-foreground",
        align === "right" && "ml-auto flex-row-reverse",
        sorted ? "text-foreground" : "text-muted-foreground",
        className,
      )}
    >
      {children}
      <Icon className={cn("size-3 shrink-0", !sorted && "opacity-40")} />
    </button>
  );
}
