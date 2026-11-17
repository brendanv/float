import * as React from "react"
import { cva } from "class-variance-authority";

import { cn } from "@/lib/utils"

const blockAlert =
  "grid gap-0.5 rounded-lg border px-2.5 py-2 text-left text-control has-data-[slot=alert-action]:relative has-data-[slot=alert-action]:pr-18 has-[>svg]:grid-cols-[auto_1fr] has-[>svg]:gap-x-2 *:[svg]:row-span-2 *:[svg]:translate-y-0 *:[svg]:text-current *:[svg:not([class*='size-'])]:size-4"

// Notice variants (info / warning / critical): one line on the soft
// background, led by an 8px dot, with an optional trailing AlertAction.
const noticeAlert =
  "flex flex-wrap items-center gap-x-2.5 gap-y-1 rounded-lg px-3 py-2 text-control text-foreground before:size-2 before:shrink-0 before:rounded-full before:content-[''] *:data-[slot=alert-description]:text-foreground *:[svg]:hidden"

const alertVariants = cva(
  "group/alert relative w-full",
  {
    variants: {
      variant: {
        default: cn(blockAlert, "bg-card text-card-foreground"),
        destructive: cn(
          blockAlert,
          "bg-card text-destructive *:data-[slot=alert-description]:text-destructive/90 *:[svg]:text-current"
        ),
        info: cn(noticeAlert, "bg-accent before:bg-primary"),
        warning: cn(noticeAlert, "bg-warning-soft before:bg-warning"),
        critical: cn(noticeAlert, "bg-below-soft before:bg-below"),
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
)

const NOTICE_VARIANTS = new Set(["info", "warning", "critical"])

function Alert({
  className,
  variant,
  ...props
}) {
  const notice = NOTICE_VARIANTS.has(variant)
  return (
    <div
      data-slot="alert"
      data-variant={variant ?? "default"}
      role={notice && variant !== "critical" ? "status" : "alert"}
      className={cn(alertVariants({ variant }), className)}
      {...props} />
  );
}

function AlertTitle({
  className,
  ...props
}) {
  return (
    <div
      data-slot="alert-title"
      className={cn(
        "font-medium group-has-[>svg]/alert:col-start-2 [&_a]:underline [&_a]:underline-offset-3 [&_a]:hover:text-foreground",
        className
      )}
      {...props} />
  );
}

function AlertDescription({
  className,
  ...props
}) {
  return (
    <div
      data-slot="alert-description"
      className={cn(
        "min-w-0 text-control text-balance text-muted-foreground md:text-pretty [&_a]:underline [&_a]:underline-offset-3 [&_a]:hover:text-foreground [&_p:not(:last-child)]:mb-2",
        className
      )}
      {...props} />
  );
}

// AlertAction sits top-right on block alerts and trails the text on notices.
// Links inside a notice action take the notice's tone.
function AlertAction({
  className,
  ...props
}) {
  return (
    <div
      data-slot="alert-action"
      className={cn(
        "absolute top-[calc(--spacing(1.25))] right-[calc(--spacing(1.25))]",
        "group-data-[variant=info]/alert:static group-data-[variant=warning]/alert:static group-data-[variant=critical]/alert:static",
        "group-data-[variant=info]/alert:ml-auto group-data-[variant=warning]/alert:ml-auto group-data-[variant=critical]/alert:ml-auto",
        "flex shrink-0 items-center gap-1 font-medium [&_a]:inline-flex [&_a]:items-center [&_a]:gap-1 [&_a]:no-underline [&_a]:hover:underline [&_a]:underline-offset-3",
        "group-data-[variant=info]/alert:[&_a]:text-primary group-data-[variant=warning]/alert:[&_a]:text-foreground group-data-[variant=critical]/alert:[&_a]:text-below",
        className
      )}
      {...props} />
  );
}

// Notice is the one-line Waterline notice: <Notice variant="warning"
// action={<Link to="/prices">Update prices</Link>}>2 holdings…</Notice>.
// `action` renders in an AlertAction (a link or a small button).
function Notice({
  variant = "info",
  action,
  children,
  className,
  ...props
}) {
  return (
    <Alert variant={variant} className={className} {...props}>
      <span data-slot="alert-description" className="min-w-0 flex-1">{children}</span>
      {action && <AlertAction>{action}</AlertAction>}
    </Alert>
  );
}

export { Alert, AlertTitle, AlertDescription, AlertAction, Notice }
