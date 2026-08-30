import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { CircleCheck } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { todayStr } from "../format.js";
import { PostingFields, toPostingInput } from "./posting-fields.jsx";
import { TagEditor } from "./tag-editor.jsx";
import { ErrorBanner } from "./error-banner.jsx";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Combobox } from "@/components/ui/combobox";
import { Form, FormField, FormRow, FormActions } from "@/components/ui/form";
import {
  ResponsiveDialog,
  ResponsiveDialogContent,
  ResponsiveDialogHeader,
  ResponsiveDialogTitle,
} from "@/components/ui/responsive-dialog";

// Above this many templates the picker switches from a row of pill buttons
// to a searchable dropdown so it doesn't overwhelm the form.
const TEMPLATE_PILL_LIMIT = 6;

function defaultPostings(initialPostings) {
  if (initialPostings && initialPostings.length >= 2) {
    return initialPostings.map((p) => ({
      account: p.account || "",
      commodity: p.commodity || "",
      quantity: p.quantity || "",
    }));
  }
  return [
    { account: "", commodity: "", quantity: "" },
    { account: "", commodity: "", quantity: "" },
  ];
}

function postingsFromTemplate(template) {
  return template.postings.map((p) => ({
    account: p.account || "",
    commodity: p.commodity || "",
    quantity: p.defaultQuantity || "",
  }));
}

function TemplatePicker({ templates, selectedId, onSelect }) {
  if (templates.length > TEMPLATE_PILL_LIMIT) {
    return (
      <Combobox
        value={selectedId}
        onChange={onSelect}
        options={templates.map((t) => ({ value: t.id, label: t.name }))}
        placeholder="Choose a template…"
        searchPlaceholder="Search templates..."
        emptyMessage="No template found."
      />
    );
  }
  return (
    <div className="flex flex-wrap gap-1.5">
      {templates.map((t) => (
        <Button
          key={t.id}
          type="button"
          size="xs"
          variant={selectedId === t.id ? "default" : "outline"}
          onClick={() => onSelect(selectedId === t.id ? "" : t.id)}
        >
          {t.name}
        </Button>
      ))}
    </div>
  );
}

export function AddTransactionForm({ onSuccess, initialValues }) {
  const queryClient = useQueryClient();
  const [date, setDate] = useState(todayStr);
  const [description, setDescription] = useState(initialValues?.description || "");
  const [postings, setPostings] = useState(() => defaultPostings(initialValues?.postings));
  const [tags, setTags] = useState(() => initialValues?.tags || {});
  const [selectedTemplateId, setSelectedTemplateId] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState(null);

  const { data: accountsData } = useQuery({
    queryKey: queryKeys.accounts(),
    queryFn: () => ledgerClient.listAccounts({}),
  });

  // Templates only make sense when starting a fresh transaction, not when
  // duplicating an existing one (initialValues already seeds the form).
  const { data: templatesData } = useQuery({
    queryKey: queryKeys.templates(),
    queryFn: () => ledgerClient.listTemplates({}),
    enabled: !initialValues,
  });
  const templates = templatesData?.templates ?? [];

  function applyTemplate(id) {
    setSelectedTemplateId(id);
    const template = templates.find((t) => t.id === id);
    if (!template) return;
    setDescription([template.payee, template.note].filter(Boolean).join(" | "));
    setPostings(postingsFromTemplate(template));
    setTags(template.tags || {});
  }

  async function handleSubmit(e) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      const postingInputs = postings
        .filter((p) => p.account.trim())
        .map(toPostingInput);
      if (postingInputs.length < 2) throw new Error("At least 2 postings are required.");
      await ledgerClient.addTransaction({
        date,
        description: description.trim(),
        postings: postingInputs,
        tags,
      });
      queryClient.invalidateQueries({ queryKey: ["transactions"] });
      queryClient.invalidateQueries({ queryKey: ["accountRegister"] });
      queryClient.invalidateQueries({ queryKey: ["balances"] });
      queryClient.invalidateQueries({ queryKey: ["netWorthTimeseries"] });
      onSuccess?.();
    } catch (err) {
      setError(err);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Form onSubmit={handleSubmit}>
      {error && <ErrorBanner error={error} />}
      {!initialValues && templates.length > 0 && (
        <FormField label="Start from a template" hint="optional">
          <TemplatePicker
            templates={templates}
            selectedId={selectedTemplateId}
            onSelect={applyTemplate}
          />
        </FormField>
      )}
      <FormRow cols={2}>
        <FormField label="Date" htmlFor="txn-date" className="sm:col-span-1">
          <Input
            id="txn-date"
            type="date"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            required
          />
        </FormField>
        <FormField label="Description" htmlFor="txn-description" className="sm:col-span-1">
          <Input
            id="txn-description"
            type="text"
            placeholder="e.g. Grocery store"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            required
          />
        </FormField>
      </FormRow>
      <FormField label="Postings">
        <PostingFields
          postings={postings}
          onChange={setPostings}
          accounts={accountsData?.accounts || []}
        />
      </FormField>
      <FormField label="Tags">
        <TagEditor value={tags} onChange={setTags} />
      </FormField>
      <FormActions>
        <Button type="submit" isLoading={submitting} loadingText="Adding…">
          Add Transaction
        </Button>
      </FormActions>
    </Form>
  );
}

export function AddTransactionModal({ open, onOpenChange, initialValues }) {
  const [success, setSuccess] = useState(false);

  function handleOpenChange(next) {
    if (!next) setSuccess(false);
    onOpenChange(next);
  }

  function handleSuccess() {
    setSuccess(true);
    setTimeout(() => handleOpenChange(false), 1000);
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={handleOpenChange}>
      <ResponsiveDialogContent size="md" showCloseButton>
        <ResponsiveDialogHeader>
          <ResponsiveDialogTitle>{initialValues ? "Duplicate Transaction" : "Add Transaction"}</ResponsiveDialogTitle>
        </ResponsiveDialogHeader>
        {success ? (
          <div className="flex flex-col items-center gap-2 py-4 text-center">
            <CircleCheck className="size-12 text-success" />
            <p className="text-sm font-medium">Transaction added successfully!</p>
          </div>
        ) : (
          <AddTransactionForm onSuccess={handleSuccess} initialValues={initialValues} />
        )}
      </ResponsiveDialogContent>
    </ResponsiveDialog>
  );
}
