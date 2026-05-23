import { cn } from "@/lib/utils";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export function Page({ className, ...props }) {
  return (
    <div
      data-slot="page"
      className={cn("flex flex-col gap-6", className)}
      {...props}
    />
  );
}

export function PageCard({
  title,
  description,
  action,
  children,
  className,
  contentClassName,
  headerClassName,
  size,
  ...props
}) {
  const hasHeader = title || description || action;

  return (
    <Card data-slot="page-card" size={size} className={className} {...props}>
      {hasHeader && (
        <CardHeader className={headerClassName}>
          {title && <CardTitle>{title}</CardTitle>}
          {description && <CardDescription>{description}</CardDescription>}
          {action && <CardAction>{action}</CardAction>}
        </CardHeader>
      )}
      <CardContent className={contentClassName}>{children}</CardContent>
    </Card>
  );
}

export function DashboardGrid({ className, ...props }) {
  return (
    <div
      data-slot="dashboard-grid"
      className={cn("grid grid-cols-12 gap-6", className)}
      {...props}
    />
  );
}

export function MetricCard({
  title,
  value,
  description,
  footer,
  valueClassName,
  className,
}) {
  return (
    <Card className={cn("h-full justify-between", className)}>
      <CardHeader>
        <CardDescription>{title}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className={cn("font-mono text-2xl font-semibold tabular-nums", valueClassName)}>
          {value}
        </div>
        {description && (
          <p className="mt-1 text-xs text-muted-foreground">{description}</p>
        )}
      </CardContent>
      {footer && <CardFooter className="text-xs text-muted-foreground">{footer}</CardFooter>}
    </Card>
  );
}
