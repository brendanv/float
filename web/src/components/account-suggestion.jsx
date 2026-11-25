import { useState } from "react";
import { Sparkles } from "lucide-react";
import { ledgerClient } from "../client.js";
import { splitAccount } from "@/lib/accounts.js";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

const CONFIDENCE = { high: "High confidence", medium: "Medium confidence", low: "Low confidence" };

// AccountSuggestionChip shows the AI-suggested account the server computed
// when the transaction was written (tx.suggestedAccount) and applies it in
// one click via update_unknown_account, which also marks it reviewed. It sits
// in the description cell on desktop (the From/To cells are too narrow for
// "unknown" plus a chip) and after the flow line on phones.
export function AccountSuggestionChip({ tx, onApplied, className }) {
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);
  const s = tx.suggestedAccount;
  if (!s?.account || !tx.fid) return null;

  async function apply(e) {
    e.stopPropagation();
    setSaving(true);
    setError(null);
    try {
      await ledgerClient.bulkEditTransactions({
        fids: [tx.fid],
        operations: [{ operation: { case: "updateUnknownAccount", value: { account: s.account } } }],
      });
      onApplied?.();
    } catch (err) {
      setError(err.message || String(err));
      setSaving(false);
    }
  }

  const { leaf } = splitAccount(s.account);
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <button
            type="button"
            data-slot="account-suggestion"
            aria-label={`Apply suggested account ${s.account}`}
            aria-invalid={error ? true : undefined}
            disabled={saving}
            onClick={apply}
            onKeyDown={(e) => e.stopPropagation()}
            className={cn(
              "touch-target inline-flex h-5 min-w-0 shrink items-center gap-1 rounded-full border border-dashed border-primary/45 px-1.5 text-[12px] leading-none whitespace-nowrap text-primary",
              "hover:border-solid hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-60",
              s.confidence === "low" && "border-border text-muted-foreground",
              error && "border-destructive text-destructive",
              className,
            )}
          />
        }
      >
        <Sparkles aria-hidden="true" className="size-3 shrink-0" />
        <span className="min-w-0 truncate">{leaf}</span>
      </TooltipTrigger>
      <TooltipContent className="max-w-72">
        <span className="flex flex-col gap-0.5">
          {error ? (
            <span>Couldn't apply: {error}</span>
          ) : (
            <>
              <span>
                Suggested <span className="font-mono">{s.account}</span>
              </span>
              <span className="opacity-80">
                {CONFIDENCE[s.confidence] || s.confidence}
                {s.reasoning ? ` · ${s.reasoning}` : ""}
              </span>
              <span className="opacity-80">Click to apply and mark reviewed.</span>
            </>
          )}
        </span>
      </TooltipContent>
    </Tooltip>
  );
}
