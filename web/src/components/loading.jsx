import { Loader2 } from "lucide-react";

export function Loading() {
  return (
    <div className="flex justify-center py-8">
      <Loader2 className="size-6 animate-spin text-muted-foreground" />
    </div>
  );
}
