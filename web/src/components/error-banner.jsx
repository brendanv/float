import { AlertTriangle } from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";

export function ErrorBanner({ error }) {
  if (!error) return null;
  return (
    <Alert variant="destructive" className="mb-4">
      <AlertTriangle />
      <AlertDescription>{error.message || String(error)}</AlertDescription>
    </Alert>
  );
}
