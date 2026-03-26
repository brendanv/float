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
      <article style={{ textAlign: "center", padding: "2rem" }}>
        <p>Transaction added successfully!</p>
        <p class="secondary">Redirecting to transactions...</p>
      </article>
    );
  }

  return (
    <div>
      <h3>Add Transaction</h3>
      {error && <ErrorBanner error={error} />}
      <form onSubmit={handleSubmit}>
        <label>
          Date
          <input
            type="date"
            value={date}
            onInput={(e) => setDate(e.target.value)}
            required
          />
        </label>
        <label>
          Description
          <input
            type="text"
            placeholder="e.g. Grocery store"
            value={description}
            onInput={(e) => setDescription(e.target.value)}
            required
          />
        </label>
        <label>Postings</label>
        <PostingFields
          postings={postings}
          onChange={setPostings}
          accounts={accounts.data?.accounts || []}
        />
        <button type="submit" disabled={submitting} aria-busy={submitting} style={{ marginTop: "1rem" }}>
          {submitting ? "Adding..." : "Add Transaction"}
        </button>
      </form>
    </div>
  );
}
