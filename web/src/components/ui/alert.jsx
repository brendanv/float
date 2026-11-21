import * as React from "react"
import { cva } from "class-variance-authority";

import { cn } from "@/lib/utils"

// Notice variants (info / warning / critical): one line on the soft
// background, led by an 8px dot, with an optional trailing AlertAction.
const noticeAlert =
  "flex flex-wrap items-center gap-x-2.5 gap-y-1 rounded-lg px-3 py-2 text-control text-foreground before:size-2 before:shrink-0 before:rounded-full before:content-[''] *:data-[slot=alert-description]:text-foreground *:[svg]:hidden"

const alertVariants = cva(
  "group/alert relative w-full",
  {
    variants: {
      variant: {
        info: cn(noticeAlert, "bg-accent before:bg-primary"),
        warning: cn(noticeAlert, "bg-warning-soft before:bg-warning"),
        critical: cn(noticeAlert, "bg-below-soft before:bg-below"),
      },
    },
    defaultVariants: {
      variant: "info",
    },
  }
)

function Alert({
  className,
  variant = "info",
  ...props
}) {
  return (
    <div
      data-slot="alert"
      data-variant={variant}
      role={variant === "critical" ? "alert" : "status"}
      className={cn(alertVariants({ variant }), className)}
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

// AlertAction trails the notice text. Links inside it take the notice's tone.
function AlertAction({
  className,
  ...props
}) {
  return (
    <div
      data-slot="alert-action"
      className={cn(
        "ml-auto flex shrink-0 items-center gap-1 font-medium [&_a]:inline-flex [&_a]:items-center [&_a]:gap-1 [&_a]:no-underline [&_a]:hover:underline [&_a]:underline-offset-3",
        "group-data-[variant=info]/alert:[&_a]:text-primary group-data-[variant=warning]/alert:[&_a]:text-foreground group-data-[variant=critical]/alert:[&_a]:text-below",
        className
      )}
      {...props} />
  );
}

// The description wraps the action onto its own line only when its text
// (capped at 14rem) and the action cannot share a line: max-w-max clamps the
// flex basis so short text does not reserve a full 14rem.
//
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
      <span data-slot="alert-description" className="min-w-0 max-w-max flex-[1_1_14rem]">{children}</span>
      {action && <AlertAction>{action}</AlertAction>}
    </Alert>
  );
}

export { Alert, AlertDescription, AlertAction, Notice }
