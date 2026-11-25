import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Toaster, toast } from "sonner";
import { ConnectError } from "@connectrpc/connect";
import { ledgerClient } from "../client.js";
import { invalidateLedgerQueries } from "../query-keys.js";
import { onUndoable, undoLabel } from "@/lib/undo.js";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

// Only the newest snapshot can be undone, so every undo toast reuses one id:
// a newer undoable change replaces the previous toast.
const TOAST_ID = "float-undo";
const DURATION_MS = 8000;

function ToastBody({ tone = "default", children, action }) {
  return (
    <div
      data-slot="undo-toast"
      className={cn(
        "flex w-full items-center gap-3 rounded-lg py-1.5 pr-1.5 pl-4 text-control shadow-md ring-1 ring-foreground/10 sm:w-[22rem]",
        tone === "critical" ? "bg-below-soft text-foreground" : "bg-popover text-popover-foreground",
      )}
    >
      <span className="min-w-0 flex-1 py-1 leading-snug">{children}</span>
      {action}
    </div>
  );
}

function ActionButton({ label, onClick }) {
  const [pending, setPending] = useState(false);
  return (
    <Button
      variant="ghost"
      size="sm"
      className="shrink-0 font-medium text-primary hover:text-primary"
      isLoading={pending}
      loadingText={label}
      onClick={async () => {
        setPending(true);
        try {
          await onClick();
        } finally {
          setPending(false);
        }
      }}
    >
      {label}
    </Button>
  );
}

// UndoToaster renders the toast region and, for every undoable change the
// client reports, an "<change> · Undo" toast. Undo reverts that snapshot
// (UndoSnapshot) and offers Redo, which undoes the undo, and so on.
export function UndoToaster({ bottomOffset }) {
  const queryClient = useQueryClient();

  useEffect(() => {
    function show({ label, actionLabel, onAction }) {
      toast.custom(
        () => (
          <ToastBody action={onAction && <ActionButton label={actionLabel} onClick={onAction} />}>
            {label}
          </ToastBody>
        ),
        { id: TOAST_ID, duration: DURATION_MS },
      );
    }

    function showError(err, verb) {
      const message = err instanceof ConnectError ? err.rawMessage : String(err?.message ?? err);
      toast.custom(
        () => (
          <ToastBody
            tone="critical"
            action={
              <Button
                variant="ghost"
                size="sm"
                className="shrink-0 font-medium"
                onClick={() => {
                  toast.dismiss(TOAST_ID);
                  window.location.hash = "#/snapshots";
                }}
              >
                Snapshots
              </Button>
            }
          >
            Couldn&rsquo;t {verb}: {message}
          </ToastBody>
        ),
        { id: TOAST_ID, duration: DURATION_MS * 2 },
      );
    }

    // offer shows label with an action that reverts `hash`; afterwards the
    // reverse action is offered on the snapshot the revert created.
    function offer(label, hash, { undone = false } = {}) {
      const verb = undone ? "redo" : "undo";
      show({
        label: undone ? "Change undone" : label,
        actionLabel: undone ? "Redo" : "Undo",
        onAction: async () => {
          let res;
          try {
            res = await ledgerClient.undoSnapshot({ hash });
          } catch (err) {
            showError(err, verb);
            return;
          }
          await invalidateLedgerQueries(queryClient);
          if (res.snapshot) {
            offer(label, res.snapshot, { undone: !undone });
          } else {
            show({ label: undone ? label : "Change undone" });
          }
        },
      });
    }

    return onUndoable((change) => offer(undoLabel(change), change.hash));
  }, [queryClient]);

  const offset = bottomOffset ? { bottom: bottomOffset } : undefined;
  return (
    <Toaster
      position="bottom-right"
      offset={offset}
      mobileOffset={offset}
      toastOptions={{ unstyled: true, className: "w-full sm:w-auto" }}
    />
  );
}
