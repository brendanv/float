import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys, invalidateLedgerQueries } from "../query-keys.js";
import { Scale, ChevronRight } from "lucide-react";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { Page, Section } from "../components/page.jsx";
import { Account } from "../components/account.jsx";
import { Amount } from "../components/amount.jsx";
import { PostingFields, editablePostings, fieldsFromPosting, toPostingInput } from "../components/posting-fields.jsx";
import { formatDate } from "../format.js";
import { Notice } from "@/components/ui/alert";
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

const DOT_CLASS = { warning: "bg-warning", below: "bg-below" };

// assertionState is an account's drift status: never asserted (below), has
// transactions since its last assertion (warning), or up to date (no mark).
function assertionState(lastAssertionDate, since) {
  if (!lastAssertionDate) return { tone: "below", label: "Never asserted" };
  if (since > 0) return { tone: "warning", label: transactionCountLabel(since) };
  return { tone: null, label: "Up to date" };
}

// AssertionStatus is the status cell: an 8px dot (none when up to date) and
// its label.
function AssertionStatus({ lastAssertionDate, transactionsSinceLastAssertion = 0 }) {
  const { tone, label } = assertionState(lastAssertionDate, transactionsSinceLastAssertion);
  return (
    <span title={label} className={cn("inline-flex items-center gap-2 whitespace-nowrap", !tone && "text-muted-foreground")}>
      <span className="inline-flex size-2 shrink-0 items-center justify-center">
        {tone && <span aria-hidden="true" className={cn("size-2 rounded-full", DOT_CLASS[tone])} />}
      </span>
      <span className="max-sm:sr-only">{tone === "warning" ? "Drifting" : label}</span>
    </span>
  );
}

// toFields maps a transaction's postings to PostingFields rows, and pre-scaffolds
// an empty balance-assertion ("=") input on the posting for `account` so the user
// just types the real balance and saves.
function toFields(postings, account) {
  return editablePostings(postings).map((p) => {
    const field = fieldsFromPosting(p);
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
    <div className="flex max-w-3xl flex-col gap-3 py-3 pr-3 pl-9" onClick={(e) => e.stopPropagation()}>
      <p className="text-control text-muted-foreground">
        Most recent transaction on this account: <span className="text-foreground">{formatDate(tx.date)}</span>{" "}
        <span className="font-medium text-foreground">{tx.description}</span>. Enter the account&rsquo;s real
        balance in the <code className="font-mono text-code text-foreground">=</code> field and save to record a
        balance assertion.
      </p>
      <Form onSubmit={handleSubmit}>
        <FormField label="Postings" error={error}>
          <PostingFields postings={postings} onChange={setPostings} accounts={accounts} />
        </FormField>
        <FormActions>
          <Button type="submit" size="sm" isLoading={saving} loadingText="Saving…">
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
  const balance = collapseAmountsByCommodity(status.balance);
  const since = status.transactionsSinceLastAssertion || 0;
  return (
    <>
      <TableRow
        data-slot="assertion-row"
        className={cn(tx && "cursor-pointer", open && "bg-accent hover:bg-accent")}
        onClick={() => tx && setOpen((o) => !o)}
      >
        <TableCell className="w-8 pr-0 align-middle">
          {tx && (
            <button
              type="button"
              aria-expanded={open}
              aria-label={`${open ? "Hide" : "Edit"} assertion for ${status.account}`}
              onClick={(e) => {
                e.stopPropagation();
                setOpen((o) => !o);
              }}
              className="inline-flex size-5 items-center justify-center rounded-sm text-faint outline-none hover:text-foreground focus-visible:ring-1 focus-visible:ring-ring"
            >
              <ChevronRight className={cn("size-3.5 transition-transform duration-150", open && "rotate-90")} />
            </button>
          )}
        </TableCell>
        <TableCell>
          <Account
            name={status.account}
            variant="path"
            type={status.type === "L" ? "liability" : "asset"}
          />
        </TableCell>
        <TableCell className="text-right">
          {balance.length === 0 ? (
            <span className="text-faint">—</span>
          ) : (
            <Amount value={balance} tone={balance.some((a) => parseFloat(a.quantity) < 0) ? "below" : "auto"} />
          )}
        </TableCell>
        <TableCell className="hidden text-muted-foreground sm:table-cell">
          {status.lastAssertionDate ? formatDate(status.lastAssertionDate) : <span className="text-faint">—</span>}
        </TableCell>
        <TableCell className="hidden text-right text-muted-foreground sm:table-cell">
          {status.lastAssertionDate ? since : <span className="text-faint">{since}</span>}
        </TableCell>
        <TableCell>
          <AssertionStatus
            lastAssertionDate={status.lastAssertionDate}
            transactionsSinceLastAssertion={since}
          />
        </TableCell>
      </TableRow>
      {open && tx && (
        <TableRow className="bg-accent/40 hover:bg-accent/40">
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
    invalidateLedgerQueries(queryClient);
  }

  const never = statuses.filter((st) => !st.lastAssertionDate).length;

  return (
    <Page>
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
        <>
          {never > 0 && (
            <Notice variant="critical">
              {never === 1 ? "1 account has" : `${never} accounts have`} never been checked against a real balance.
            </Notice>
          )}
          <Section
            title="Accounts"
            description="Transactions posted since each account's last balance assertion"
          >
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="w-8 pr-0"><span className="sr-only">Expand</span></TableHead>
                  <TableHead>Account</TableHead>
                  <TableHead className="text-right">Balance</TableHead>
                  <TableHead className="hidden sm:table-cell">Last assertion</TableHead>
                  <TableHead className="hidden text-right sm:table-cell">Since</TableHead>
                  <TableHead><span className="max-sm:sr-only">Status</span></TableHead>
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
          </Section>
        </>
      )}
    </Page>
  );
}
