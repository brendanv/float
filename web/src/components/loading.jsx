import { Loader2 } from "lucide-react";

export function Loading() {
  return (
    <div className="flex justify-center py-8" role="status" aria-label="Loading">
      <Loader2 className="size-5 animate-spin text-faint" />
    </div>
  );
}
