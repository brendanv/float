import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Check } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys, invalidateLedgerQueries } from "../query-keys.js";
import { PostingFields, toPostingInput } from "../components/posting-fields.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { Page } from "../components/page.jsx";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Form, FormField, FormActions } from "@/components/ui/form";

function todayStr() {
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export function AddTransactionPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const [date, setDate] = useState(todayStr);
  const [description, setDescription] = useState("");
  const [postings, setPostings] = useState([
    { account: "", commodity: "", quantity: "" },
    { account: "", commodity: "", quantity: "" },
  ]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState(null);
  const [success, setSuccess] = useState(false);

  const { data: accountsData } = useQuery({
    queryKey: queryKeys.accounts(),
    queryFn: () => ledgerClient.listAccounts({}),
  });

  async function handleSubmit(e) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);

    try {
      const postingInputs = postings
        .filter((p) => p.account.trim())
        .map(toPostingInput);

      if (postingInputs.length < 2) {
        throw new Error("At least 2 postings are required.");
      }

      await ledgerClient.addTransaction({
        date,
        description: description.trim(),
        postings: postingInputs,
      });

      invalidateLedgerQueries(queryClient);
      setSuccess(true);
      setTimeout(() => navigate({ to: "/transactions" }), 1000);
    } catch (err) {
      setError(err);
    } finally {
      setSubmitting(false);
    }
  }

  if (success) {
    return (
      <Page>
        <PageHeader title="Add transaction" />
        <div role="status" className="flex flex-col gap-1">
          <p className="flex items-center gap-2 text-body font-medium">
            <span className="inline-flex size-5 items-center justify-center rounded-full bg-inflow-soft text-inflow">
              <Check className="size-3.5" />
            </span>
            Transaction added
          </p>
          <p className="pl-7 text-control text-muted-foreground">Redirecting to transactions…</p>
        </div>
      </Page>
    );
  }

  return (
    <Page>
      <PageHeader title="Add transaction" />
      <div className="max-w-2xl">
          <Form onSubmit={handleSubmit}>
            {error && <ErrorBanner error={error} />}
            <div className="grid gap-4 sm:grid-cols-[11rem_minmax(0,1fr)]">
              <FormField label="Date" htmlFor="date">
                <Input
                  id="date"
                  type="date"
                  value={date}
                  onChange={(e) => setDate(e.target.value)}
                  required
                />
              </FormField>
              <FormField label="Description" htmlFor="description">
                <Input
                  id="description"
                  type="text"
                  placeholder="e.g. Grocery store"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  required
                />
              </FormField>
            </div>
            <FormField label="Postings">
              <PostingFields
                postings={postings}
                onChange={setPostings}
                accounts={accountsData?.accounts || []}
                showBalance
              />
            </FormField>
            <FormActions>
              <Button type="submit" isLoading={submitting} loadingText="Adding…">
                Add transaction
              </Button>
            </FormActions>
          </Form>
      </div>
    </Page>
  );
}
