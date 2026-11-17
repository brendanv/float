import * as React from "react"

import { cn } from "@/lib/utils"

// Card is a genuine object (form, dialog-like panel, notice slab): hairline
// ring, paper fill, inner padding. Page sections are not cards; use Section
// from components/page.jsx. `variant="raised"` is the only variant; `size`
// is "default" or "sm".
function Card({
  className,
  variant = "raised",
  size = "default",
  ...props
}) {
  return (
    <div
      data-slot="card"
      data-variant={variant}
      data-size={size}
      className={cn(
        "group/card flex flex-col overflow-hidden rounded-lg bg-card text-body text-card-foreground ring-1 ring-border has-data-[slot=card-footer]:pb-0 has-[>img:first-child]:pt-0 *:[img:first-child]:rounded-none *:[img:last-child]:rounded-none",
        "gap-4 py-4 data-[size=sm]:gap-2 data-[size=sm]:py-3",
        className
      )}
      {...props} />
  );
}

function CardHeader({
  className,
  ...props
}) {
  return (
    <div
      data-slot="card-header"
      className={cn(
        "group/card-header @container/card-header grid auto-rows-min items-start gap-x-3 gap-y-0.5 has-data-[slot=card-action]:grid-cols-[1fr_auto] has-data-[slot=card-description]:grid-rows-[auto_auto]",
        "px-4 group-data-[size=sm]/card:px-3 [.border-b]:pb-4 group-data-[size=sm]/card:[.border-b]:pb-3",
        className
      )}
      {...props} />
  );
}

function CardTitle({
  className,
  ...props
}) {
  return (
    <div
      data-slot="card-title"
      className={cn(
        "font-heading text-heading font-semibold",
        className
      )}
      {...props} />
  );
}

function CardDescription({
  className,
  ...props
}) {
  return (
    <div
      data-slot="card-description"
      className={cn("text-control text-muted-foreground", className)}
      {...props} />
  );
}

function CardAction({
  className,
  ...props
}) {
  return (
    <div
      data-slot="card-action"
      className={cn(
        "col-start-2 row-span-2 row-start-1 self-start justify-self-end",
        className
      )}
      {...props} />
  );
}

function CardContent({
  className,
  ...props
}) {
  return (
    <div
      data-slot="card-content"
      className={cn("px-4 group-data-[size=sm]/card:px-3", className)}
      {...props} />
  );
}

function CardFooter({
  className,
  ...props
}) {
  return (
    <div
      data-slot="card-footer"
      className={cn("flex items-center border-t p-4 group-data-[size=sm]/card:p-3", className)}
      {...props} />
  );
}

export {
  Card,
  CardHeader,
  CardFooter,
  CardTitle,
  CardAction,
  CardDescription,
  CardContent,
}
