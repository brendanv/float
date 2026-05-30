import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { ErrorBanner } from "./error-banner.jsx";
import { AccountInput } from "./posting-fields.jsx";
import { FormField } from "@/components/ui/form";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { cn } from "@/lib/utils";

const SOURCE_LABELS = {
  unreviewed: "Unreviewed transactions",
  account: "From a specific account",
  nopayee: "Transactions without payees",
};

export function SuggestRulesWizard({ open, onOpenChange, accounts, onRulesAdded, initialSourceType = "unreviewed" }) {
  const [step, setStep] = useState("source");
  const [sourceType, setSourceType] = useState(initialSourceType);
  const [accountName, setAccountName] = useState("");
  const [suggestions, setSuggestions] = useState([]);
  const [selected, setSelected] = useState(new Set());
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState(null);

  function resetWizard() {
    setStep("source");
    setSourceType(initialSourceType);
    setAccountName("");
    setSuggestions([]);
    setSelected(new Set());
    setLoading(false);
    setError(null);
    setSaving(false);
    setSaveError(null);
  }

  function handleOpenChange(val) {
    if (!val) resetWizard();
    onOpenChange(val);
  }

  function buildQuery() {
    if (sourceType === "account") return `acct:${accountName}`;
    if (sourceType === "nopayee") return "not:desc:.*[|].*";
    return "not:status:*";
  }

  async function handleAnalyze() {
    setError(null);
    setLoading(true);
    try {
      const res = await ledgerClient.suggestRules({ query: buildQuery(), fids: [] });
      const suggs = res.suggestions ?? [];
      setSuggestions(suggs);
      setSelected(new Set(suggs.map((_, i) => i)));
      setStep("suggestions");
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  }

  async function handleSave() {
    setSaveError(null);
    setSaving(true);
    try {
      const rulesToAdd = [...selected].map((i) => {
        const s = suggestions[i];
        return {
          pattern: s.pattern,
          payee: s.payee,
          account: s.account,
          tags: s.tags ?? {},
          priority: 0,
          autoReviewed: true,
        };
      });
      await ledgerClient.addRule({ rules: rulesToAdd });
      onRulesAdded();
      handleOpenChange(false);
    } catch (err) {
      setSaveError(err);
    } finally {
      setSaving(false);
    }
  }

  function toggleAll() {
    if (selected.size === suggestions.length) {
      setSelected(new Set());
    } else {
      setSelected(new Set(suggestions.map((_, i) => i)));
    }
  }

  function toggleOne(i) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(i)) next.delete(i);
      else next.add(i);
      return next;
    });
  }

  const canAnalyze = sourceType !== "account" || accountName.trim().length > 0;

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        {step === "source" && (
          <>
            <DialogHeader>
              <DialogTitle>Suggest Rules</DialogTitle>
              <DialogDescription>
                Choose which transactions to analyze for rule suggestions.
              </DialogDescription>
            </DialogHeader>
            <div className="flex flex-col gap-4">
              {error && <ErrorBanner error={error} />}
              <FormField label="Transaction source">
                <NativeSelect
                  value={sourceType}
                  onChange={(e) => setSourceType(e.target.value)}
                  className="w-full"
                >
                  <NativeSelectOption value="unreviewed">Unreviewed transactions</NativeSelectOption>
                  <NativeSelectOption value="account">From a specific account</NativeSelectOption>
                  <NativeSelectOption value="nopayee">Transactions without payees</NativeSelectOption>
                </NativeSelect>
              </FormField>
              {sourceType === "account" && (
                <FormField label="Account">
                  <AccountInput
                    value={accountName}
                    onChange={setAccountName}
                    accounts={accounts}
                    placeholder="expenses:unknown"
                  />
                </FormField>
              )}
            </div>
            <DialogFooter>
              <DialogClose asChild>
                <Button variant="outline" size="sm" disabled={loading}>Cancel</Button>
              </DialogClose>
              <Button
                size="sm"
                onClick={handleAnalyze}
                disabled={!canAnalyze}
                isLoading={loading}
                loadingText="Analyzing…"
              >
                Analyze
              </Button>
            </DialogFooter>
          </>
        )}

        {step === "suggestions" && (
          <>
            <DialogHeader>
              <DialogTitle>Suggested Rules</DialogTitle>
              <DialogDescription>
                {suggestions.length === 0
                  ? "No rules could be suggested for the selected transactions."
                  : `${suggestions.length} rule(s) suggested. Select the ones you'd like to create.`}
              </DialogDescription>
            </DialogHeader>
            {suggestions.length > 0 && (
              <div className="flex max-h-[50vh] flex-col gap-2 overflow-y-auto">
                <div className="flex items-center gap-2 border-b pb-1">
                  <Checkbox
                    checked={selected.size === suggestions.length}
                    onCheckedChange={toggleAll}
                  />
                  <span className="text-xs text-muted-foreground">Select all</span>
                </div>
                {suggestions.map((s, i) => (
                  <div
                    key={i}
                    className={cn(
                      "flex gap-3 rounded border p-3",
                      selected.has(i) ? "border-primary/30 bg-primary/5" : "border-transparent bg-muted/30"
                    )}
                  >
                    <Checkbox
                      checked={selected.has(i)}
                      onCheckedChange={() => toggleOne(i)}
                      className="mt-0.5 shrink-0"
                    />
                    <div className="flex min-w-0 flex-col gap-2">
                      <div className="flex flex-wrap items-center gap-1.5">
                        <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">{s.pattern}</code>
                        {s.tags && Object.keys(s.tags).length > 0 &&
                          Object.entries(s.tags).map(([k, v]) => (
                            <Badge key={k} variant="outline" className="font-mono text-xs">
                              {v ? `${k}=${v}` : k}
                            </Badge>
                          ))
                        }
                      </div>
                      {(s.payee || s.account) && (
                        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
                          {s.payee && (
                            <span>
                              <span className="text-muted-foreground">Payee: </span>
                              <span className="font-medium">{s.payee}</span>
                            </span>
                          )}
                          {s.account && (
                            <span>
                              <span className="text-muted-foreground">Category: </span>
                              <span className="font-mono">{s.account}</span>
                            </span>
                          )}
                        </div>
                      )}
                      {s.reasoning && (
                        <p className="text-xs text-muted-foreground">{s.reasoning}</p>
                      )}
                      {s.exampleFids && s.exampleFids.length > 0 && (
                        <p className="text-xs text-muted-foreground/50">
                          {s.exampleFids.length} example transaction{s.exampleFids.length !== 1 ? "s" : ""}
                        </p>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
            {saveError && <ErrorBanner error={saveError} />}
            <DialogFooter>
              <Button variant="ghost" size="sm" onClick={() => setStep("source")} disabled={saving}>
                ← Back
              </Button>
              <DialogClose asChild>
                <Button variant="outline" size="sm" disabled={saving}>Cancel</Button>
              </DialogClose>
              {suggestions.length > 0 && (
                <Button
                  size="sm"
                  onClick={handleSave}
                  disabled={selected.size === 0}
                  isLoading={saving}
                  loadingText="Adding…"
                >
                  Add {selected.size} Rule(s)
                </Button>
              )}
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
