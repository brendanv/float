import { cn } from "@/lib/utils";

export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  className,
}) {
  return (
    <div
      data-slot="empty-state"
      className={cn(
        "flex flex-col items-center gap-2 py-8 text-center text-muted-foreground",
        className,
      )}
    >
      {Icon && <Icon className="size-8" />}
      {title && <p className="text-sm font-medium text-foreground">{title}</p>}
      {description && <p className="max-w-md text-sm">{description}</p>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}
