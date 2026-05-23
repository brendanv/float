import { AlertTriangle } from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { cn } from "@/lib/utils";

export function ErrorBanner({ error, className }) {
  if (!error) return null;
  return (
    <Alert variant="destructive" className={cn(className)}>
      <AlertTriangle />
      <AlertDescription className="whitespace-pre-wrap break-words">{(error.message || String(error)).replace(/^\[[^\]]+\]\s*/, "")}</AlertDescription>
    </Alert>
  );
}
