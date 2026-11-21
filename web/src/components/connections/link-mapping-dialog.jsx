import { useState } from "react";
import { filterAndSortAccountNames } from "@/lib/accounts.js";
import { ErrorBanner } from "@/components/error-banner.jsx";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Form, FormField } from "@/components/ui/form";
import { Combobox } from "@/components/ui/combobox";
import {
  ResponsiveDialog,
  ResponsiveDialogContent,
  ResponsiveDialogFooter,
  ResponsiveDialogHeader,
  ResponsiveDialogTitle,
} from "@/components/ui/responsive-dialog";

// LinkMappingDialog maps Stripe Financial Connections accounts to hledger
// accounts (and display names) after linking, or for one unmapped account.
// The optional start date skips older transactions, e.g. after relinking an
// account whose history was already imported under its old Stripe IDs.
export function LinkMappingDialog({ open, fcAccounts, accountDeclarations, onComplete, onClose }) {
  const [mappings, setMappings] = useState(() =>
    Object.fromEntries(
      fcAccounts.map((a) => [
        a.id,
        { hledgerAccount: "", displayName: a.display_name ?? a.displayName ?? a.id, importFrom: "" },
      ])
    )
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  function setMapping(fcId, field, value) {
    setMappings((prev) => ({ ...prev, [fcId]: { ...prev[fcId], [field]: value } }));
  }

  const allMapped = fcAccounts.every((a) => mappings[a.id]?.hledgerAccount);
  const accountOptions = filterAndSortAccountNames(accountDeclarations, {
    includePrefixes: ["assets", "liabilities"],
  });

  async function handleSubmit(e) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      await onComplete(mappings);
    } catch (err) {
      setError(err);
      setSaving(false);
    }
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={(v) => { if (!v) onClose(); }}>
      <ResponsiveDialogContent className="sm:max-w-lg">
        <ResponsiveDialogHeader>
          <ResponsiveDialogTitle>Map linked accounts</ResponsiveDialogTitle>
        </ResponsiveDialogHeader>
        <Form onSubmit={handleSubmit}>
          {error && <ErrorBanner error={error} />}
          <p className="text-control text-muted-foreground">
            {fcAccounts.length === 1
              ? "1 account was linked. Choose the hledger account for it."
              : `${fcAccounts.length} accounts were linked. Choose the hledger account for each.`}
          </p>
          {fcAccounts.map((a) => (
            <div key={a.id} className="flex flex-col gap-3 border-t border-border pt-3">
              <span className="font-mono text-[11.5px] text-faint">{a.id}</span>
              <FormField label="Display name">
                <Input
                  value={mappings[a.id]?.displayName ?? ""}
                  onChange={(e) => setMapping(a.id, "displayName", e.target.value)}
                  placeholder={a.display_name ?? a.displayName ?? a.id}
                />
              </FormField>
              <FormField label="hledger account">
                <Combobox
                  value={mappings[a.id]?.hledgerAccount || ""}
                  onChange={(v) => setMapping(a.id, "hledgerAccount", v)}
                  options={accountOptions}
                  placeholder="Select account…"
                  searchPlaceholder="Search accounts…"
                  emptyMessage="No matching account."
                  clearable
                />
              </FormField>
              <FormField
                label="Import from"
                htmlFor={`import-from-${a.id}`}
                hint="Optional"
                description="Transactions before this date are never fetched or imported. Set it when relinking an account that was already imported."
              >
                <Input
                  id={`import-from-${a.id}`}
                  type="date"
                  value={mappings[a.id]?.importFrom ?? ""}
                  onChange={(e) => setMapping(a.id, "importFrom", e.target.value)}
                />
              </FormField>
            </div>
          ))}
          <ResponsiveDialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
            <Button
              type="submit"
              disabled={!allMapped}
              isLoading={saving}
              loadingText="Saving…"
            >
              Save
            </Button>
          </ResponsiveDialogFooter>
        </Form>
      </ResponsiveDialogContent>
    </ResponsiveDialog>
  );
}
