import { cn } from "@/lib/utils";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
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
