import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { PostingFields } from "../components/posting-fields.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";

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
    { account: "", amount: "" },
    { account: "", amount: "" },
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
        .map((p) => ({
          account: p.account.trim(),
          amount: p.amount.trim(),
        }));

      if (postingInputs.length < 2) {
        throw new Error("At least 2 postings are required.");
      }

      await ledgerClient.addTransaction({
        date,
        description: description.trim(),
        postings: postingInputs,
      });

      queryClient.invalidateQueries({ queryKey: ["transactions"] });
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
      <div class="card bg-base-100 shadow-sm max-w-lg mx-auto mt-8">
        <div class="card-body items-center text-center">
          <div class="text-success text-5xl mb-2">✓</div>
          <p class="text-lg font-medium">Transaction added successfully!</p>
          <p class="text-base-content/60 text-sm">Redirecting to transactions...</p>
        </div>
      </div>
    );
  }

  return (
    <div>
      <h2 class="text-2xl font-bold mb-6">Add Transaction</h2>
      <div class="card bg-base-100 shadow-sm max-w-lg">
        <div class="card-body">
          {error && <ErrorBanner error={error} />}
          <form onSubmit={handleSubmit} class="space-y-4">
            <div class="form-control">
              <label class="label">
                <span class="label-text">Date</span>
              </label>
              <input
                type="date"
                value={date}
                onInput={(e) => setDate(e.target.value)}
                required
                class="input input-bordered w-full"
              />
            </div>
            <div class="form-control">
              <label class="label">
                <span class="label-text">Description</span>
              </label>
              <input
                type="text"
                placeholder="e.g. Grocery store"
                value={description}
                onInput={(e) => setDescription(e.target.value)}
                required
                class="input input-bordered w-full"
              />
            </div>
            <div class="form-control">
              <label class="label">
                <span class="label-text">Postings</span>
              </label>
              <PostingFields
                postings={postings}
                onChange={setPostings}
                accounts={accountsData?.accounts || []}
              />
            </div>
            <button type="submit" disabled={submitting} class="btn btn-primary w-full">
              {submitting ? <span class="loading loading-spinner loading-sm"></span> : "Add Transaction"}
            </button>
          </form>
        </div>
      </div>
    </div>
  );
}
