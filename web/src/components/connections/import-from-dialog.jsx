import { useState } from "react";
import { ledgerClient } from "@/client.js";
import { ErrorBanner } from "@/components/error-banner.jsx";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Form, FormField } from "@/components/ui/form";
import {
  ResponsiveDialog,
  ResponsiveDialogContent,
  ResponsiveDialogDescription,
  ResponsiveDialogFooter,
  ResponsiveDialogHeader,
  ResponsiveDialogTitle,
} from "@/components/ui/responsive-dialog";

// ImportFromDialog sets or clears a linked account's import cutoff: Stripe
// transactions dated before it are never fetched or imported (manually or by
// the daily auto-import). onSaved receives the updated StripeLinkedAccount.
export function ImportFromDialog({ open, account, onSaved, onClose }) {
  const [value, setValue] = useState(account.importFrom ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  async function save(importFrom) {
    setSaving(true);
    setError(null);
    try {
      const res = await ledgerClient.setStripeAccountImportFrom({
        stripeAccountId: account.stripeAccountId,
        importFrom,
      });
      await onSaved(res.account);
    } catch (err) {
      setError(err);
      setSaving(false);
    }
  }

  const inputId = `import-from-${account.stripeAccountId}`;

  return (
    <ResponsiveDialog open={open} onOpenChange={(v) => { if (!v) onClose(); }}>
      <ResponsiveDialogContent className="sm:max-w-sm">
        <ResponsiveDialogHeader>
          <ResponsiveDialogTitle>Import start date</ResponsiveDialogTitle>
          <ResponsiveDialogDescription>
            {account.displayName || account.stripeAccountId}: transactions before this date are never
            fetched or imported. Set it after relinking an account whose history is already in the journal.
          </ResponsiveDialogDescription>
        </ResponsiveDialogHeader>
        <Form onSubmit={(e) => { e.preventDefault(); save(value); }}>
          {error && <ErrorBanner error={error} />}
          <FormField label="Import from" htmlFor={inputId}>
            <Input id={inputId} type="date" value={value} onChange={(e) => setValue(e.target.value)} />
          </FormField>
          <ResponsiveDialogFooter>
            {account.importFrom && (
              <Button
                type="button"
                variant="ghost"
                className="sm:mr-auto"
                disabled={saving}
                onClick={() => save("")}
              >
                Clear
              </Button>
            )}
            <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
            <Button type="submit" disabled={!value} isLoading={saving} loadingText="Saving…">
              Save
            </Button>
          </ResponsiveDialogFooter>
        </Form>
      </ResponsiveDialogContent>
    </ResponsiveDialog>
  );
}
