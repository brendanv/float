import { useState } from "preact/hooks";
import { ledgerClient } from "../client.js";
import { useRpc } from "../hooks/use-rpc.js";
import { PostingFields } from "../components/posting-fields.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { navigate } from "../router.jsx";

function todayStr() {
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export function AddTransactionPage() {
  const [date, setDate] = useState(todayStr);
  const [description, setDescription] = useState("");
  const [postings, setPostings] = useState([
    { account: "", amount: "" },
    { account: "", amount: "" },
  ]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState(null);
  const [success, setSuccess] = useState(false);

  const accounts = useRpc(() => ledgerClient.listAccounts({}), []);

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

      setSuccess(true);
      setTimeout(() => navigate("/transactions"), 1000);
    } catch (err) {
      setError(err);
    } finally {
      setSubmitting(false);
    }
  }

  if (success) {
    return (
      <div class="card bg-base-100 shadow-sm max-w-lg mx-auto">
        <div class="card-body items-center text-center">
          <div class="text-success text-5xl mb-2">✓</div>
          <p class="text-lg font-medium">Transaction added successfully!</p>
          <p class="text-base-content/60 text-sm">Redirecting to transactions...</p>
        </div>
      </div>
    );
  }

  return (
    <div class="max-w-lg">
      <h3 class="text-xl font-semibold mb-4">Add Transaction</h3>
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
            accounts={accounts.data?.accounts || []}
          />
        </div>
        <button type="submit" disabled={submitting} class="btn btn-primary w-full">
          {submitting ? <span class="loading loading-spinner loading-sm"></span> : "Add Transaction"}
        </button>
      </form>
    </div>
  );
}
