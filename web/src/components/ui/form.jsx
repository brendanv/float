import * as React from "react";
import { cn } from "@/lib/utils";
import { Label } from "@/components/ui/label";

// Form: vertical stack of field rows and action rows. Use as the root of every
// form so spacing between fields/sections is consistent.
function Form({ className, ...props }) {
  return (
    <form
      data-slot="form"
      className={cn("flex flex-col gap-4", className)}
      {...props}
    />
  );
}

// FormSection groups related fields with consistent spacing — typically a
// single field or a FormRow + optional description.
function FormSection({ className, ...props }) {
  return (
    <div
      data-slot="form-section"
      className={cn("flex flex-col gap-1.5", className)}
      {...props}
    />
  );
}

// FormField renders Label + control + optional description + optional error
// with consistent spacing. Pass the control as children. `hint` renders
// right-aligned next to the label (e.g. "(optional)").
function FormField({
  label,
  htmlFor,
  description,
  hint,
  error,
  children,
  className,
}) {
  return (
    <div
      data-slot="form-field"
      className={cn("flex min-w-0 flex-col gap-1.5", className)}
    >
      {label && (
        <div className="flex items-baseline justify-between gap-2 min-w-0">
          <Label htmlFor={htmlFor} className="text-xs">
            {label}
          </Label>
          {hint && (
            <span className="text-[10px] text-muted-foreground shrink-0">
              {hint}
            </span>
          )}
        </div>
      )}
      {children}
      {description && (
        <p className="text-[11px] leading-snug text-muted-foreground">
          {description}
        </p>
      )}
      {error && (
        <p className="text-[11px] leading-snug text-destructive">{error}</p>
      )}
    </div>
  );
}

// FormRow lays out multiple FormFields in a responsive grid. By default the
// grid is single-column on mobile and `cols` columns on sm+. Pass `cols={3}`
// or `cols={4}` to override.
function FormRow({ cols = 2, className, children, ...props }) {
  const gridClass =
    cols === 4
      ? "sm:grid-cols-4"
      : cols === 3
      ? "sm:grid-cols-3"
      : cols === 2
      ? "sm:grid-cols-2"
      : "";
  return (
    <div
      data-slot="form-row"
      data-cols={cols}
      className={cn("grid grid-cols-1 gap-3", gridClass, className)}
      {...props}
    >
      {children}
    </div>
  );
}

// FormActions is a right-aligned button row that stacks on mobile.
function FormActions({ className, align = "end", ...props }) {
  const alignClass =
    align === "between"
      ? "sm:justify-between"
      : align === "start"
      ? "sm:justify-start"
      : "sm:justify-end";
  return (
    <div
      data-slot="form-actions"
      className={cn(
        "flex flex-col-reverse gap-2 pt-1 sm:flex-row sm:items-center",
        alignClass,
        className
      )}
      {...props}
    />
  );
}

export { Form, FormSection, FormField, FormRow, FormActions };
