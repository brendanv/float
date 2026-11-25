import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { Copy, Trash2 } from "lucide-react";
import { ledgerClient } from "../client.js";
import { formatAmounts } from "../format.js";
import { accountSplit, sideAccount } from "@/lib/transactions.js";
import { AddTransactionModal } from "./add-transaction-modal.jsx";
import { Account } from "./account.jsx";
import { AccountInput, PostingFields, editablePostings, fieldsFromPosting, toPostingInput } from "./posting-fields.jsx";
import { InlineEdit, inlineEditKeyHandler, useInlineEditState } from "./inline-edit.jsx";
import { TagEditor } from "./tag-editor.jsx";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Form, FormActions, FormField } from "@/components/ui/form";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

// Inline editors and the full edit form for transaction rows. Every save
// sends all postings back through toPostingInput(fieldsFromPosting(p)) so
// posting comments survive (UpdateTransaction replaces all postings).

export function EditableDescriptionCell({ fid, description, date, postings, payee, note, onSaved }) {
  const navigate = useNavigate();
  const state = useInlineEditState(description);

  async function save() {
    if (state.draft.trim() === description) { state.cancel(); return; }
    await state.run(async () => {
      // Account-register rows don't carry postings; UpdateTransaction
      // replaces them all, so fetch the full transaction first.
      let fullPostings = postings;
      if (!fullPostings) {
        const resp = await ledgerClient.listTransactions({ query: [`code:${fid}`], limit: 1 });
        fullPostings = resp.transactions?.[0]?.postings;
        if (!fullPostings) throw new Error("Transaction not found");
      }
      await ledgerClient.updateTransaction({
        fid,
        description: state.draft.trim(),
        date,
        postings: editablePostings(fullPostings).map((p) => toPostingInput(fieldsFromPosting(p))),
      });
      if (onSaved) onSaved();
    });
  }

  const display = payee ? (
    <>
      <strong
        className="cursor-pointer font-medium hover:underline"
        onClick={(e) => { e.stopPropagation(); navigate({ to: "/transactions", search: { payee } }); }}
        title={"Show all transactions for " + payee}
      >{payee}</strong>
      {note && <span className="ml-1.5 text-faint">{note}</span>}
    </>
  ) : (
    <span className="font-medium">{description}</span>
  );

  return (
    <InlineEdit
      display={display}
      displayClassName="block truncate"
      canEdit={!!fid}
      editing={state.editing}
      onActivate={() => state.start(description)}
      onCancel={state.cancel}
      onSave={save}
      saving={state.saving}
      error={state.error}
      title="Click to edit description"
      editor={
        <Input
          className="h-6 w-full"
          value={state.draft}
          onInput={(e) => state.setDraft(e.target.value)}
          onKeyDown={inlineEditKeyHandler({ onSave: save, onCancel: state.cancel })}
          autoFocus
        />
      }
    />
  );
}

// EditableAccountSideCell handles the from/to columns in the general
// transactions view. `side` is "from" or "to" and selects which posting on
// the row to replace on save (the single negative or single positive
// posting). When the row has zero or multiple postings on that side, the
// cell is not editable and falls back to the full edit view.
const COUNT_LABEL = /^\d+ accounts$/;

// FlowEnd shows one side of a row's flow: the account leaf with its glyph,
// or a muted "N accounts" when that side has several postings.
function FlowEnd({ name }) {
  if (!name) return null;
  if (COUNT_LABEL.test(name)) return <span className="truncate text-faint">{name}</span>;
  return <Account name={name} />;
}

export function EditableAccountSideCell({ tx, side, accounts, onSaved }) {
  const postings = editablePostings(tx.postings);
  const account = sideAccount(tx, side);
  const state = useInlineEditState(account);
  const canEdit = !!tx.fid && !!account;

  async function save() {
    if (!state.draft || state.draft === account) { state.cancel(); return; }
    await state.run(async () => {
      const newPostings = postings.map((p) => {
        const f = fieldsFromPosting(p);
        return toPostingInput(p.account === account ? { ...f, account: state.draft } : f);
      });
      await ledgerClient.updateTransaction({
        fid: tx.fid,
        description: tx.description,
        date: tx.date,
        postings: newPostings,
        status: "Cleared",
      });
      if (onSaved) onSaved();
    });
  }

  return (
    <InlineEdit
      display={<FlowEnd name={account || accountSplit(tx)[side]} />}
      canEdit={canEdit}
      editing={state.editing}
      onActivate={() => state.start(account)}
      onCancel={state.cancel}
      onSave={save}
      saving={state.saving}
      error={state.error}
      title={`${account} — click to change`}
      displayClassName="flex min-w-0 text-muted-foreground"
      editor={
        <AccountInput
          value={state.draft}
          onChange={state.setDraft}
          accounts={accounts}
        />
      }
    />
  );
}

export function EditableOtherAccountCell({ fid, otherAccounts, accounts, onSaved }) {
  const state = useInlineEditState("");
  const [loading, setLoading] = useState(false);
  const [txData, setTxData] = useState(null);

  const canEdit = !!fid && otherAccounts.length === 1;
  const displayText = otherAccounts.length === 0 ? ""
    : otherAccounts.length === 1 ? otherAccounts[0]
    : `${otherAccounts.length} accounts`;
  const display = <FlowEnd name={displayText} />;

  async function activate() {
    setLoading(true);
    state.setError?.(null);
    try {
      const resp = await ledgerClient.listTransactions({ query: [`code:${fid}`], limit: 1 });
      const tx = resp.transactions?.[0];
      if (!tx) throw new Error("Transaction not found");
      setTxData(tx);
      state.start(otherAccounts[0]);
    } catch (err) {
      state.setError(err.message || String(err));
    } finally {
      setLoading(false);
    }
  }

  async function save() {
    if (!txData || !state.draft) return;
    await state.run(async () => {
      const oldAccount = otherAccounts[0];
      const newPostings = editablePostings(txData.postings).map((p) => {
        const f = fieldsFromPosting(p);
        return toPostingInput(p.account === oldAccount ? { ...f, account: state.draft } : f);
      });
      await ledgerClient.updateTransaction({
        fid: txData.fid,
        description: txData.description,
        date: txData.date,
        postings: newPostings,
        status: "Cleared",
      });
      setTxData(null);
      if (onSaved) onSaved();
    });
  }

  function cancel() {
    state.cancel();
    setTxData(null);
  }

  return (
    <InlineEdit
      display={display}
      canEdit={canEdit}
      editing={state.editing}
      loading={loading}
      onActivate={activate}
      onCancel={cancel}
      onSave={save}
      saving={state.saving}
      error={state.error}
      title={`${displayText} — click to change`}
      displayClassName="flex min-w-0 text-muted-foreground"
      editor={
        <AccountInput
          value={state.draft}
          onChange={state.setDraft}
          accounts={accounts}
        />
      }
    />
  );
}

export function EditableDetailRow({ tx, accounts, onSaved, onDeleted, onCancel, inDialog = false }) {
  const initialPostings = editablePostings(tx.postings).map(fieldsFromPosting);
  const generatedPostings = (tx.postings || []).filter((p) => p.generated);
  const initialTags = tx.tags || {};
  const initialDate = tx.date;
  const initialDescription = tx.description ?? "";
  const [date, setDate] = useState(initialDate);
  const [description, setDescription] = useState(initialDescription);
  const [postings, setPostings] = useState(initialPostings);
  const [tags, setTags] = useState(initialTags);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);
  const [deleting, setDeleting] = useState(false);
  const [duplicateOpen, setDuplicateOpen] = useState(false);

  const isDirty = date !== initialDate
    || description.trim() !== initialDescription
    || JSON.stringify(postings) !== JSON.stringify(initialPostings)
    || JSON.stringify(tags) !== JSON.stringify(initialTags);

  async function handleSubmit(e) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    // Rows added with "Add posting" but left blank are dropped, as in the add form.
    const trimmed = postings.map(toPostingInput).filter((p) => p.account);
    const autoReview = trimmed.every((p) => !p.account.toLowerCase().includes("unknown"));
    try {
      await ledgerClient.updateTransaction({
        fid: tx.fid,
        description: description.trim(),
        date,
        postings: trimmed,
        tags,
        // The editor shows the full tag set, so an empty map means "remove
        // them all" rather than "leave tags alone".
        replaceTags: true,
        status: autoReview ? "Cleared" : (tx.status ?? ""),
      });
      if (onSaved) onSaved();
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setSaving(false);
    }
  }

  function cancel() {
    setDate(initialDate);
    setDescription(initialDescription);
    setPostings(initialPostings);
    setTags(initialTags);
    setError(null);
    if (onCancel) onCancel();
  }

  async function handleDelete() {
    setDeleting(true);
    try {
      await ledgerClient.deleteTransaction({ fid: tx.fid });
      if (onDeleted) onDeleted();
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setDeleting(false);
    }
  }

  return (
    <div
      className={inDialog ? "pb-4" : "sticky left-0 max-w-[100cqw] p-3"}
      onClick={(e) => e.stopPropagation()}
    >
      <Form onSubmit={handleSubmit}>
        <FormField label="Date" htmlFor={`txn-date-${tx.fid}`}>
          <Input
            id={`txn-date-${tx.fid}`}
            type="date"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            required
          />
        </FormField>
        <FormField label="Description" htmlFor={`txn-desc-${tx.fid}`}>
          <Input
            id={`txn-desc-${tx.fid}`}
            value={description}
            onInput={(e) => setDescription(e.target.value)}
            placeholder="Payee | note"
          />
        </FormField>
        <FormField label="Postings" error={error}>
          <PostingFields postings={postings} onChange={setPostings} accounts={accounts} />
          {generatedPostings.map((p, i) => (
            <div key={i} className="mt-1 font-mono text-xs text-muted-foreground">
              {p.account} {formatAmounts(p.amounts)} · inferred by hledger
            </div>
          ))}
        </FormField>
        <FormField label="Tags">
          <TagEditor value={tags} onChange={setTags} />
        </FormField>
        <FormActions align="between">
          <div className="flex gap-2">
            <Button variant="outline" size={inDialog ? "sm" : "xs"} disabled={saving || deleting} onClick={() => setDuplicateOpen(true)}>
              <Copy data-icon="inline-start" /> Duplicate
            </Button>
            <Button
              variant="destructive-ghost"
              size={inDialog ? "sm" : "xs"}
              disabled={saving}
              isLoading={deleting}
              loadingText="Deleting…"
              onClick={handleDelete}
            >
              <Trash2 data-icon="inline-start" /> Delete
            </Button>
          </div>
          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:items-center">
            <Button type="button" variant="outline" size={inDialog ? "default" : "xs"} onClick={cancel} disabled={saving || deleting}>
              Cancel
            </Button>
            <Button type="submit" size={inDialog ? "default" : "xs"} disabled={!isDirty || deleting} isLoading={saving}>
              Save
            </Button>
          </div>
        </FormActions>
      </Form>
      {/* Rendered outside <Form>: React events bubble through portals, so a
          nested modal's submit would also trigger this row's update. */}
      <AddTransactionModal
        open={duplicateOpen}
        onOpenChange={setDuplicateOpen}
        initialValues={{
          description: tx.description,
          postings: initialPostings,
          tags: tx.tags,
        }}
      />
    </div>
  );
}

// ── stripe indicator ───────────────────────────────────────────────────────

export function StripeIndicator({ id }) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span className="inline-flex shrink-0 cursor-default items-center rounded-sm px-1 py-0.5" aria-label="Stripe transaction">
            <svg role="img" viewBox="0 0 24 24" className="size-2.5 fill-faint" xmlns="http://www.w3.org/2000/svg">
              <path d="M13.976 9.15c-2.172-.806-3.356-1.426-3.356-2.409 0-.831.683-1.305 1.901-1.305 2.227 0 4.515.858 6.09 1.631l.89-5.494C18.252.975 15.697 0 12.165 0 9.667 0 7.589.654 6.104 1.872 4.56 3.147 3.757 4.992 3.757 7.218c0 4.039 2.467 5.76 6.476 7.219 2.585.92 3.445 1.574 3.445 2.583 0 .98-.84 1.545-2.354 1.545-1.875 0-4.965-.921-6.99-2.109l-.9 5.555C5.175 22.99 8.385 24 11.714 24c2.641 0 4.843-.624 6.328-1.813 1.664-1.305 2.525-3.236 2.525-5.732 0-4.128-2.524-5.851-6.594-7.305h.003z" />
            </svg>
          </span>
        }
      />
      <TooltipContent>Stripe transaction: {id}</TooltipContent>
    </Tooltip>
  );
}
