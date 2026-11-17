import { Alert, AlertDescription } from "@/components/ui/alert";
import { cn } from "@/lib/utils";

// ErrorBanner shows an RPC/validation error as a critical Notice. Messages
// can span lines (hledger output), so the dot aligns with the first line.
export function ErrorBanner({ error, className }) {
  if (!error) return null;
  return (
    <Alert variant="critical" className={cn("items-start before:mt-1.5", className)}>
      <AlertDescription className="flex-1 whitespace-pre-wrap break-words">{(error.message || String(error)).replace(/^\[[^\]]+\]\s*/, "")}</AlertDescription>
    </Alert>
  );
}
