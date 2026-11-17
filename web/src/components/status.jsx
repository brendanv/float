import { useState } from "react";
import { Check } from "lucide-react";
import { ledgerClient } from "../client.js";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

// Mark draws the status glyph. Reviewed (Cleared) has no mark: attention is
// earned. Unmarked ("") is an accent dot; Pending ("!") is an amber ring.
function Mark({ status, error }) {
  if (error) return <span aria-hidden="true" className="size-2 rounded-full bg-below" />;
  if (status === "Cleared") return null;
  if (status === "Pending") {
    return <span aria-hidden="true" className="size-2 rounded-full border-[1.5px] border-warning" />;
  }
  return <span aria-hidden="true" className="size-2 rounded-full bg-primary" />;
}

function statusLabel(status) {
  if (status === "Cleared") return "Reviewed";
  if (status === "Pending") return "Pending";
  return "Unreviewed";
}

// StatusMark shows a transaction's hledger status ("Cleared", "Pending" or
// "" for unmarked). Without `fid` it is a static mark. With `fid` it is a
// button that cycles the status (Cleared → Pending, anything else →
// Cleared) through UpdateTransactionStatus, then calls `onStatusChange`. The reviewed state
// stays blank until hover/focus, when a faint check shows the action.
export function StatusMark({ status, fid, onStatusChange, className }) {
  const [updating, setUpdating] = useState(false);
  const [error, setError] = useState(null);
  const isReviewed = status === "Cleared";

  if (!fid) {
    return (
      <span
        data-slot="status-mark"
        data-status={status || "Unmarked"}
        role="img"
        aria-label={statusLabel(status)}
        title={statusLabel(status)}
        className={cn("inline-flex size-5 items-center justify-center", className)}
      >
        <Mark status={status} />
      </span>
    );
  }

  async function handleClick(e) {
    e.stopPropagation();
    if (updating) return;
    const newStatus = isReviewed ? "Pending" : "Cleared";
    setUpdating(true);
    setError(null);
    try {
      await ledgerClient.updateTransactionStatus({ fid, status: newStatus });
      if (onStatusChange) onStatusChange();
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setUpdating(false);
    }
  }

  const title = error
    ? `Couldn't update status: ${error}`
    : `${statusLabel(status)} — click to mark ${isReviewed ? "pending" : "reviewed"}`;

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={handleClick}
            isLoading={updating}
            aria-label={title}
            data-slot="status-mark"
            data-status={status || "Unmarked"}
            className={cn("group/status rounded-full", className)}
          >
            <Mark status={status} error={error} />
            {isReviewed && !error && (
              <Check className="text-faint opacity-0 transition-opacity group-hover/status:opacity-100 group-focus-visible/status:opacity-100" />
            )}
          </Button>
        }
      />
      <TooltipContent>{title}</TooltipContent>
    </Tooltip>
  );
}
