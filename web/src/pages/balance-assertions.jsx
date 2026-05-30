import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Scale, ChevronRight, ChevronDown, AlertTriangle } from "lucide-react";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { Page, PageCard } from "../components/page.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { PostingFields, toPostingInput } from "../components/posting-fields.jsx";
import { formatCurrency, formatDate } from "../format.js";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Form, FormField, FormActions } from "@/components/ui/form";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";

// collapseAmountsByCommodity sums amounts with the same commodity, dropping cost
// annotations. Investment accounts can have many lots at different cost bases;
// for drift-checking purposes we only need total units per commodity.
function collapseAmountsByCommodity(amounts) {
  if (!amounts || amounts.length === 0) return [];
  const totals = new Map();
  for (const a of amounts) {
    totals.set(a.commodity, (totals.get(a.commodity) || 0) + (parseFloat(a.quantity) || 0));
  }
  return Array.from(totals.entries()).map(([commodity, total]) => ({
    commodity,
    quantity: String(total),
  }));
}

function transactionCountLabel(count) {
  if (count === 0) return "No transactions since";
  if (count === 1) return "1 transaction since";
  return `${count} transactions since`;
}

function AssertionBadge({ lastAssertionDate, transactionsSinceLastAssertion = 0 }) {
  const countLabel = transactionCountLabel(transactionsSinceLastAssertion);
  if (!lastAssertionDate) {
    return (
      <Badge variant="destructive">
        <AlertTriangle data-icon="inline-start" /> Never asserted ({countLabel.toLowerCase()})
      </Badge>
    );
  }
  if (transactionsSinceLastAssertion === 0) {
    return <Badge variant="secondary">Up to date</Badge>;
  }
  return <Badge variant="secondary">{countLabel}</Badge>;
}

// toFields maps a transaction's postings to PostingFields rows, and pre-scaffolds
// an empty balance-assertion ("=") input on the posting for `account` so the user
// just types the real balance and saves.
function toFields(postings, account) {
  return (postings || []).map((p) => {
    const a = p.amounts && p.amounts[0];
    const ba = p.balanceAssertion;
    const field = {
      account: p.account,
      commodity: a ? a.commodity : "",
      quantity: a ? a.quantity : "",
      cost: a ? a.cost : undefined,
      balanceAssertion: ba?.amount
        ? { commodity: ba.amount.commodity, quantity: ba.amount.quantity }
        : undefined,
    };
    if (p.account === account && !field.balanceAssertion) {
      field.balanceAssertion = { commodity: field.commodity || "", quantity: "" };
    }
    return field;
  });
}

function AssertionEditor({ account, tx, accounts, onSaved }) {
  const [postings, setPostings] = useState(() => toFields(tx.postings, account));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  async function handleSubmit(e) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      await ledgerClient.updateTransaction({
        fid: tx.fid,
        description: tx.description,
        date: tx.date,
        postings: postings.map(toPostingInput),
        status: tx.status || "",
      });
      if (onSaved) onSaved();
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="p-3" onClick={(e) => e.stopPropagation()}>
      <p className="mb-2 text-sm text-muted-foreground">
        Most recent transaction touching this account ({formatDate(tx.date)} &mdash;{" "}
        {tx.description}). Enter the account&rsquo;s real balance in the &ldquo;=&rdquo; field
        and save to record a balance assertion.
      </p>
      <Form onSubmit={handleSubmit}>
        <FormField label="Postings" error={error}>
          <PostingFields postings={postings} onChange={setPostings} accounts={accounts} />
        </FormField>
        <FormActions>
          <Button type="submit" size="xs" isLoading={saving}>
            Save assertion
          </Button>
        </FormActions>
      </Form>
    </div>
  );
}

function AssertionRow({ status, accounts, onSaved }) {
  const [open, setOpen] = useState(false);
  const tx = status.lastTransaction;
  const collapsedBalance = collapseAmountsByCommodity(status.balance);
  return (
    <>
      <TableRow
        className={cn(tx && "cursor-pointer")}
        onClick={() => tx && setOpen((o) => !o)}
      >
        <TableCell className="w-8 align-middle">
          {tx &&
            (open ? (
              <ChevronDown className="size-4 text-muted-foreground" />
            ) : (
              <ChevronRight className="size-4 text-muted-foreground" />
            ))}
        </TableCell>
        <TableCell className="font-medium">{status.account}</TableCell>
        <TableCell>
          <Badge variant="outline">{status.type === "L" ? "Liability" : "Asset"}</Badge>
        </TableCell>
        <TableCell className="text-right font-mono tabular-nums">
          {collapsedBalance.length === 0 ? (
            "—"
          ) : (
            collapsedBalance.map((a) => (
              <div key={a.commodity}>{formatCurrency(a.quantity, a.commodity)}</div>
            ))
          )}
        </TableCell>
        <TableCell>
          <div>{status.lastAssertionDate ? formatDate(status.lastAssertionDate) : "—"}</div>
          <div className="text-xs text-muted-foreground">
            {transactionCountLabel(status.transactionsSinceLastAssertion || 0)}
          </div>
        </TableCell>
        <TableCell>
          <AssertionBadge
            lastAssertionDate={status.lastAssertionDate}
            transactionsSinceLastAssertion={status.transactionsSinceLastAssertion || 0}
          />
        </TableCell>
      </TableRow>
      {open && tx && (
        <TableRow className="bg-muted/30 hover:bg-muted/30">
          <TableCell colSpan={6} className="p-0 whitespace-normal">
            <AssertionEditor
              account={status.account}
              tx={tx}
              accounts={accounts}
              onSaved={onSaved}
            />
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

export function BalanceAssertionsPage() {
  const queryClient = useQueryClient();
  const { data, isLoading, error } = useQuery({
    queryKey: queryKeys.balanceAssertionStatus(),
    queryFn: () => ledgerClient.getBalanceAssertionStatus({}),
  });
  const { data: accountsData } = useQuery({
    queryKey: queryKeys.accounts(),
    queryFn: () => ledgerClient.listAccounts({}),
  });

  const accounts = accountsData?.accounts || [];
  const statuses = data?.accounts || [];

  function handleSaved() {
    queryClient.invalidateQueries({ queryKey: queryKeys.balanceAssertionStatus() });
    queryClient.invalidateQueries({ queryKey: queryKeys.transactions() });
    queryClient.invalidateQueries({ queryKey: queryKeys.balances() });
    queryClient.invalidateQueries({ queryKey: queryKeys.accountRegister() });
  }

  return (
    <Page>
      <PageHeader
        title="Balance Assertions"
        description="See how many asset and liability account transactions have posted since each account's last enforced balance assertion. Expand an account to edit its most recent transaction and add a fresh assertion."
      />
      {isLoading ? (
        <Loading />
      ) : error ? (
        <ErrorBanner error={error} />
      ) : statuses.length === 0 ? (
        <EmptyState
          icon={Scale}
          title="No accounts to check"
          description="No asset or liability accounts with transactions were found."
        />
      ) : (
        <PageCard contentClassName="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-8" />
                <TableHead>Account</TableHead>
                <TableHead>Type</TableHead>
                <TableHead className="text-right">Current balance</TableHead>
                <TableHead>Last assertion</TableHead>
                <TableHead>Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {statuses.map((status) => (
                <AssertionRow
                  key={status.account}
                  status={status}
                  accounts={accounts}
                  onSaved={handleSaved}
                />
              ))}
            </TableBody>
          </Table>
        </PageCard>
      )}
    </Page>
  );
}
