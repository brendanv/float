import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, CircleCheck } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { PostingFields } from "../components/posting-fields.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";

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
      queryClient.invalidateQueries({ queryKey: ["accountRegister"] });
      queryClient.invalidateQueries({ queryKey: ["balances"] });
      queryClient.invalidateQueries({ queryKey: ["netWorthTimeseries"] });
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
      <Card className="mx-auto mt-8 max-w-lg">
        <CardContent className="flex flex-col items-center text-center">
          <CircleCheck className="mb-2 size-12 text-success" />
          <p className="text-lg font-medium">Transaction added successfully!</p>
          <p className="text-sm text-muted-foreground">Redirecting to transactions...</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <h2 className="text-xl font-bold">Add Transaction</h2>
      <Card className="max-w-lg">
        <CardContent>
          {error && <div className="mb-4"><ErrorBanner error={error} /></div>}
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="date">Date</Label>
              <Input
                id="date"
                type="date"
                value={date}
                onChange={(e) => setDate(e.target.value)}
                required
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="description">Description</Label>
              <Input
                id="description"
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
        </CardContent>
      </Card>
    </div>
  );
}
