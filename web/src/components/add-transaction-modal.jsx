import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, CircleCheck } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { PostingFields } from "./posting-fields.jsx";
import { ErrorBanner } from "./error-banner.jsx";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

function todayStr() {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

function AddTransactionForm({ onSuccess }) {
  const queryClient = useQueryClient();
  const [date, setDate] = useState(todayStr);
  const [description, setDescription] = useState("");
  const [postings, setPostings] = useState([
    { account: "", amount: "" },
    { account: "", amount: "" },
  ]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState(null);

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
        .map((p) => ({ account: p.account.trim(), amount: p.amount.trim() }));
      if (postingInputs.length < 2) throw new Error("At least 2 postings are required.");
      await ledgerClient.addTransaction({
        date,
        description: description.trim(),
        postings: postingInputs,
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
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      {error && <ErrorBanner error={error} />}
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="txn-date">Date</Label>
        <Input
          id="txn-date"
          type="date"
          value={date}
          onChange={(e) => setDate(e.target.value)}
          required
        />
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="txn-description">Description</Label>
        <Input
          id="txn-description"
          type="text"
          placeholder="e.g. Grocery store"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          required
        />
      </div>
      <div className="flex flex-col gap-1.5">
        <Label>Postings</Label>
        <PostingFields
          postings={postings}
          onChange={setPostings}
          accounts={accountsData?.accounts || []}
        />
      </div>
      <Button type="submit" disabled={submitting} className="w-full">
        {submitting ? <Loader2 className="size-4 animate-spin" /> : "Add Transaction"}
      </Button>
    </form>
  );
}

export function AddTransactionModal({ open, onOpenChange }) {
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
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-lg" showCloseButton>
        <DialogHeader>
          <DialogTitle>Add Transaction</DialogTitle>
        </DialogHeader>
        {success ? (
          <div className="flex flex-col items-center gap-2 py-4 text-center">
            <CircleCheck className="size-12 text-success" />
            <p className="text-sm font-medium">Transaction added successfully!</p>
          </div>
        ) : (
          <AddTransactionForm onSuccess={handleSuccess} />
        )}
      </DialogContent>
    </Dialog>
  );
}
