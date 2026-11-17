import { Link } from "@tanstack/react-router";
import { cn } from "@/lib/utils";

// SectionTabs links the routes grouped under one nav item (Reports,
// Investments, Import, Settings). `items` is [{ to, label }]; the item whose
// `to` equals `currentPath` is marked current.
export function SectionTabs({ items, currentPath, label = "Section", className }) {
  if (!items?.length) return null;
  return (
    <nav
      data-slot="section-tabs"
      aria-label={label}
      className={cn(
        "-mx-4 [scrollbar-width:none] flex gap-5 overflow-x-auto border-b border-border px-4 md:mx-0 md:px-0",
        className,
      )}
    >
      {items.map((item) => {
        const current = item.to === currentPath;
        return (
          <Link
            key={item.to}
            to={item.to}
            aria-current={current ? "page" : undefined}
            className={cn(
              "shrink-0 rounded-sm pb-2.5 text-body font-medium whitespace-nowrap text-faint outline-none transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring",
              current && "text-foreground shadow-[inset_0_-2px_0_var(--primary)]",
            )}
          >
            {item.label}
          </Link>
        );
      })}
    </nav>
  );
}
