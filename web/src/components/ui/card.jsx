import * as React from "react"

import { cn } from "@/lib/utils"

// Card variants (Waterline):
// - "raised": a genuine object (form, dialog-like panel, notice slab). Hairline
//   ring, paper fill, inner padding.
// - "section": a page section sitting directly on the page background. No
//   ring, fill or outer padding; the header is a section heading over a
//   hairline. PageCard uses this by default.
// Variant and size reach the slots through context so their padding is
// picked in JS rather than by competing group-data selectors.
const CardContext = React.createContext({ variant: "raised", size: "default" })

function useCard() {
  return React.useContext(CardContext)
}

function Card({
  className,
  variant = "raised",
  size = "default",
  ...props
}) {
  const ctx = React.useMemo(() => ({ variant, size }), [variant, size])
  const section = variant === "section"
  const sm = size === "sm"
  return (
    <CardContext.Provider value={ctx}>
      <div
        data-slot="card"
        data-variant={variant}
        data-size={size}
        className={cn(
          "group/card flex flex-col text-body text-card-foreground",
          section
            ? sm ? "gap-2" : "gap-3"
            : cn(
                "overflow-hidden rounded-lg bg-card ring-1 ring-border has-data-[slot=card-footer]:pb-0 has-[>img:first-child]:pt-0 *:[img:first-child]:rounded-none *:[img:last-child]:rounded-none",
                sm ? "gap-2 py-3" : "gap-4 py-4"
              ),
          className
        )}
        {...props} />
    </CardContext.Provider>
  );
}

function CardHeader({
  className,
  ...props
}) {
  const { variant, size } = useCard()
  const section = variant === "section"
  return (
    <div
      data-slot="card-header"
      className={cn(
        "group/card-header @container/card-header grid auto-rows-min items-start gap-x-3 gap-y-0.5 has-data-[slot=card-action]:grid-cols-[1fr_auto] has-data-[slot=card-description]:grid-rows-[auto_auto]",
        section
          ? "min-h-8 content-end border-b border-border pb-2"
          : cn(size === "sm" ? "px-3 [.border-b]:pb-3" : "px-4 [.border-b]:pb-4"),
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
  const { variant } = useCard()
  return (
    <div
      data-slot="card-action"
      className={cn(
        "col-start-2 row-span-2 row-start-1 justify-self-end",
        variant === "section" ? "self-end" : "self-start",
        className
      )}
      {...props} />
  );
}

function CardContent({
  className,
  ...props
}) {
  const { variant, size } = useCard()
  return (
    <div
      data-slot="card-content"
      className={cn(
        variant === "section" ? "min-w-0" : size === "sm" ? "px-3" : "px-4",
        className
      )}
      {...props} />
  );
}

function CardFooter({
  className,
  ...props
}) {
  const { variant, size } = useCard()
  return (
    <div
      data-slot="card-footer"
      className={cn(
        "flex items-center border-t",
        variant === "section" ? "pt-3" : size === "sm" ? "p-3" : "p-4",
        className
      )}
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
