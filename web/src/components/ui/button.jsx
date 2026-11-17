import { forwardRef } from "react";
import { Button as ButtonPrimitive } from "@base-ui/react/button"
import { cva } from "class-variance-authority";
import { Loader2 } from "lucide-react";

import { cn } from "@/lib/utils"

const buttonVariants = cva(
  "group/button inline-flex shrink-0 items-center justify-center rounded-md border border-transparent bg-clip-padding text-control font-medium whitespace-nowrap transition-all outline-none select-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50 active:not-aria-[haspopup]:translate-y-px disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-1 aria-invalid:ring-destructive/20 dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground [a]:hover:bg-primary/80",
        outline:
          "border-input bg-popover hover:bg-muted hover:text-foreground aria-expanded:bg-muted aria-expanded:text-foreground",
        secondary:
          "bg-secondary text-secondary-foreground hover:bg-secondary/80 aria-expanded:bg-secondary aria-expanded:text-secondary-foreground",
        ghost:
          "hover:bg-muted hover:text-foreground aria-expanded:bg-muted aria-expanded:text-foreground",
        destructive:
          "bg-destructive/10 text-destructive hover:bg-destructive/20 focus-visible:border-destructive/40 focus-visible:ring-destructive/20 dark:bg-destructive/20 dark:hover:bg-destructive/30 dark:focus-visible:ring-destructive/40",
        "destructive-ghost":
          "text-destructive hover:bg-destructive/10 hover:text-destructive aria-expanded:bg-destructive/10 aria-expanded:text-destructive focus-visible:border-destructive/40 focus-visible:ring-destructive/20",
        link: "text-primary underline-offset-4 hover:underline",
      },
      size: {
        default:
          "h-8 pointer-coarse:min-h-10 pointer-coarse:min-w-10 gap-1.5 px-3 has-data-[icon=inline-end]:pr-2.5 has-data-[icon=inline-start]:pl-2.5",
        xs: "h-6 pointer-coarse:min-h-10 pointer-coarse:min-w-10 gap-1 px-2 text-xs has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3",
        sm: "h-7 pointer-coarse:min-h-10 pointer-coarse:min-w-10 gap-1 px-2.5 has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3.5",
        lg: "h-9 pointer-coarse:min-h-11 pointer-coarse:min-w-11 gap-1.5 px-3.5 has-data-[icon=inline-end]:pr-3 has-data-[icon=inline-start]:pl-3",
        icon: "size-8 pointer-coarse:min-h-10 pointer-coarse:min-w-10",
        "icon-xs": "size-6 pointer-coarse:min-h-10 pointer-coarse:min-w-10 [&_svg:not([class*='size-'])]:size-3",
        "icon-sm": "size-7 pointer-coarse:min-h-10 pointer-coarse:min-w-10",
        "icon-lg": "size-9 pointer-coarse:min-h-11 pointer-coarse:min-w-11",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

// forwardRef is required on React 18: base-ui triggers (TooltipTrigger,
// DialogTrigger, etc.) pass a ref to their `render` element.
const Button = forwardRef(function Button({
  className,
  variant = "default",
  size = "default",
  isLoading = false,
  loadingText,
  disabled,
  children,
  ...props
}, ref) {
  return (
    <ButtonPrimitive
      ref={ref}
      data-slot="button"
      className={cn(buttonVariants({ variant, size, className }))}
      disabled={disabled || isLoading}
      {...props}
    >
      {isLoading && (
        <Loader2
          data-icon={loadingText !== undefined ? "inline-start" : undefined}
          className="size-3.5 animate-spin"
        />
      )}
      {isLoading ? loadingText : children}
    </ButtonPrimitive>
  );
});

export { Button, buttonVariants }
