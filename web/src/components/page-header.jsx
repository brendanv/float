import { cn } from "@/lib/utils";

// PageHeader is the page title (22px) with a right-aligned actions slot.
// Pass actions as `actions` or as children. `description` is a muted line
// under the title; leave it out when it would only repeat the title.
export function PageHeader({ title, description, actions, children, className }) {
  const slot = actions ?? children;
  return (
    <div
      data-slot="page-header"
      className={cn("flex flex-wrap items-center gap-x-3 gap-y-2", className)}
    >
      <div className="flex min-w-0 flex-col gap-0.5">
        <h1 className="text-title font-semibold">{title}</h1>
        {description && (
          <p className="text-control text-muted-foreground">{description}</p>
        )}
      </div>
      {slot && (
        <div data-slot="page-header-actions" className="ml-auto flex flex-wrap items-center gap-2">
          {slot}
        </div>
      )}
    </div>
  );
}
