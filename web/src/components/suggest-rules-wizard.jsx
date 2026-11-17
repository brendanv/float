import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { ErrorBanner } from "./error-banner.jsx";
import { AccountInput } from "./posting-fields.jsx";
import { Account } from "./account.jsx";
import { TagChip, tagLabel } from "./tag-editor.jsx";
import { FormField } from "@/components/ui/form";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  ResponsiveDialog,
  ResponsiveDialogClose,
  ResponsiveDialogContent,
  ResponsiveDialogDescription,
  ResponsiveDialogFooter,
  ResponsiveDialogHeader,
  ResponsiveDialogTitle,
} from "@/components/ui/responsive-dialog";
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
    <ResponsiveDialog open={open} onOpenChange={handleOpenChange}>
      <ResponsiveDialogContent className="sm:max-w-2xl">
        {step === "source" && (
          <>
            <ResponsiveDialogHeader>
              <ResponsiveDialogTitle>Suggest rules</ResponsiveDialogTitle>
              <ResponsiveDialogDescription>
                Choose which transactions to analyze for rule suggestions.
              </ResponsiveDialogDescription>
            </ResponsiveDialogHeader>
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
            <ResponsiveDialogFooter>
              <ResponsiveDialogClose asChild>
                <Button variant="outline" size="sm" disabled={loading}>Cancel</Button>
              </ResponsiveDialogClose>
              <Button
                size="sm"
                onClick={handleAnalyze}
                disabled={!canAnalyze}
                isLoading={loading}
                loadingText="Analyzing…"
              >
                Analyze
              </Button>
            </ResponsiveDialogFooter>
          </>
        )}

        {step === "suggestions" && (
          <>
            <ResponsiveDialogHeader>
              <ResponsiveDialogTitle>Suggested rules</ResponsiveDialogTitle>
              <ResponsiveDialogDescription>
                {suggestions.length === 0
                  ? "No rules could be suggested for the selected transactions."
                  : `${suggestions.length} rule(s) suggested. Select the ones you'd like to create.`}
              </ResponsiveDialogDescription>
            </ResponsiveDialogHeader>
            {suggestions.length > 0 && (
              <div data-slot="rule-suggestions" className="flex max-h-[50vh] flex-col gap-2 overflow-y-auto p-px">
                <label className="flex items-center gap-2 border-b border-border px-3 pb-2 text-control text-muted-foreground">
                  <Checkbox
                    checked={selected.size === suggestions.length}
                    onCheckedChange={toggleAll}
                  />
                  Select all
                </label>
                {suggestions.map((s, i) => (
                  <div
                    key={i}
                    className={cn(
                      "flex gap-3 rounded-lg p-3 ring-1",
                      selected.has(i) ? "bg-accent ring-primary/25" : "ring-border",
                    )}
                  >
                    <Checkbox
                      aria-label={`Select rule ${s.pattern}`}
                      checked={selected.has(i)}
                      onCheckedChange={() => toggleOne(i)}
                      className="mt-0.5 shrink-0"
                    />
                    <div className="flex min-w-0 flex-col gap-1.5">
                      <div className="flex flex-wrap items-center gap-1.5">
                        <code className="font-mono text-code break-all">{s.pattern}</code>
                        {s.tags && Object.entries(s.tags).map(([k, v]) => (
                          <TagChip key={k}>{tagLabel(k, v)}</TagChip>
                        ))}
                      </div>
                      {(s.payee || s.account) && (
                        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-control">
                          {s.payee && (
                            <span>
                              <span className="text-muted-foreground">Payee </span>
                              <span className="font-medium">{s.payee}</span>
                            </span>
                          )}
                          {s.account && (
                            <span className="inline-flex min-w-0 items-center gap-1.5">
                              <span className="text-muted-foreground">Category</span>
                              <Account name={s.account} variant="path" />
                            </span>
                          )}
                        </div>
                      )}
                      {s.reasoning && (
                        <p className="text-control text-muted-foreground">{s.reasoning}</p>
                      )}
                      {s.exampleFids && s.exampleFids.length > 0 && (
                        <p className="text-[12px] text-faint">
                          {s.exampleFids.length} example transaction{s.exampleFids.length !== 1 ? "s" : ""}
                        </p>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
            {saveError && <ErrorBanner error={saveError} />}
            <ResponsiveDialogFooter>
              <Button variant="ghost" size="sm" onClick={() => setStep("source")} disabled={saving}>
                ← Back
              </Button>
              <ResponsiveDialogClose asChild>
                <Button variant="outline" size="sm" disabled={saving}>Cancel</Button>
              </ResponsiveDialogClose>
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
            </ResponsiveDialogFooter>
          </>
        )}
      </ResponsiveDialogContent>
    </ResponsiveDialog>
  );
}
