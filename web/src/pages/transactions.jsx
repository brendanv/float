import { useEffect, useState } from "react";
import { useSearch, useRouter, useNavigate } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, X, ArrowLeft, Trash2, Upload } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys, invalidateLedgerQueries } from "../query-keys.js";
import { dateRangeQuery, escapeRegex, exactRegex } from "../lib/hledger-query.js";
import { SearchControls } from "../components/search-controls.jsx";
import { DATE_PRESETS, PAYEE_NONE } from "../components/search-presets.js";
import { TransactionTable } from "../components/transaction-table.jsx";
import { TransactionSummary } from "../components/transaction-summary.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { UNREVIEWED_QUERY, useReviewCount } from "../hooks/use-review-count.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { inlineEditKeyHandler } from "../components/inline-edit.jsx";
import { AccountInput } from "../components/posting-fields.jsx";
import { Page } from "../components/page.jsx";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";

function BulkActionBar({ selectedFids, transactions, accounts, focusedAccount, onActionComplete, onClearSelection }) {
  const [mode, setMode] = useState("idle"); // 'idle' | 'add-tag' | 'remove-tag' | 'set-payee' | 'set-unknown-account'
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [working, setWorking] = useState(false);
  const [tagKey, setTagKey] = useState("");
  const [tagValue, setTagValue] = useState("");
  const [removeTagKey, setRemoveTagKey] = useState("");
  const [payee, setPayee] = useState("");
  const [unknownAccount, setUnknownAccount] = useState("");
  const [error, setError] = useState(null);

  const selectedTxs = transactions.filter((tx) => selectedFids.has(tx.fid));
  const count = selectedTxs.length;
  const fids = selectedTxs.map((tx) => tx.fid);

  // Register rows carry otherAccounts (plus the focused account) instead of postings.
  const txAccounts = (tx) => tx.postings
    ? tx.postings.map((p) => p.account)
    : [...(tx.otherAccounts || []), ...(focusedAccount ? [focusedAccount] : [])];
  const allHaveUnknown = selectedTxs.length > 0 && selectedTxs.every((tx) =>
    txAccounts(tx).some((a) => a.toLowerCase().includes("unknown"))
  );

  // Collect union of all tag keys across selected transactions
  const availableTagKeys = [...new Set(selectedTxs.flatMap((tx) => Object.keys(tx.tags || {})))].sort();

  function cancelMode() {
    setMode("idle");
    setTagKey("");
    setTagValue("");
    setRemoveTagKey("");
    setPayee("");
    setUnknownAccount("");
    setError(null);
  }

  async function runBulk(operations) {
    setWorking(true);
    setError(null);
    try {
      await ledgerClient.bulkEditTransactions({ fids, operations });
      cancelMode();
      onActionComplete();
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setWorking(false);
    }
  }

  function bulkMarkStatus(reviewed) {
    return runBulk([{ operation: { case: "markReviewed", value: { reviewed } } }]);
  }

  function bulkAddTag() {
    if (!tagKey.trim()) return;
    return runBulk([{ operation: { case: "addTag", value: { key: tagKey.trim(), value: tagValue.trim() } } }]);
  }

  function bulkRemoveTag() {
    if (!removeTagKey) return;
    return runBulk([{ operation: { case: "removeTag", value: { key: removeTagKey } } }]);
  }

  function bulkSetPayee() {
    if (!payee.trim()) return;
    return runBulk([{ operation: { case: "setPayee", value: { payee: payee.trim() } } }]);
  }

  function bulkUpdateUnknownAccount() {
    if (!unknownAccount.trim()) return;
    return runBulk([{ operation: { case: "updateUnknownAccount", value: { account: unknownAccount.trim() } } }]);
  }

  async function bulkDelete() {
    setWorking(true);
    setError(null);
    try {
      // eslint-disable-next-line no-unused-vars
      for await (const _response of ledgerClient.bulkDeleteTransactions({ fids })) {
        // progress events arrive here; terminal result ends the stream
      }
      setDeleteOpen(false);
      cancelMode();
      onActionComplete();
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setWorking(false);
    }
  }

  return (
    <div data-slot="bulk-action-bar" className="mb-2 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted px-3 py-1.5 text-control">
      <span className="shrink-0 font-medium text-foreground/80">
        {count} selected
      </span>

      {mode === "idle" && (
        <>
          <Button
            variant="ghost"
            size="xs"
            disabled={working}
            onClick={() => bulkMarkStatus(true)}
          >
            Mark reviewed
          </Button>
          <Button
            variant="ghost"
            size="xs"
            disabled={working}
            onClick={() => bulkMarkStatus(false)}
          >
            Mark unreviewed
          </Button>
          <Button
            variant="ghost"
            size="xs"
            disabled={working}
            onClick={() => setMode("add-tag")}
          >
            Add tag
          </Button>
          <Button
            variant="ghost"
            size="xs"
            disabled={working || availableTagKeys.length === 0}
            onClick={() => { setRemoveTagKey(availableTagKeys[0] || ""); setMode("remove-tag"); }}
            title={availableTagKeys.length === 0 ? "No tags on selected transactions" : undefined}
          >
            Remove tag
          </Button>
          <Button
            variant="ghost"
            size="xs"
            disabled={working}
            onClick={() => setMode("set-payee")}
          >
            Set payee
          </Button>
          {allHaveUnknown && (
            <Button
              variant="ghost"
              size="xs"
              disabled={working}
              onClick={() => setMode("set-unknown-account")}
            >
              Update unknown account
            </Button>
          )}
          <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
            <DialogTrigger
              render={<Button variant="destructive-ghost" size="xs" disabled={working} />}
            >
              <Trash2 className="size-3" data-icon="inline-start" /> Delete
            </DialogTrigger>
            <DialogContent showCloseButton={false}>
              <DialogHeader>
                <DialogTitle>Delete selected transactions?</DialogTitle>
                <DialogDescription>
                  This will permanently remove {count} selected transaction{count === 1 ? "" : "s"} from the journal. This cannot be undone.
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <Button variant="outline" onClick={() => setDeleteOpen(false)} disabled={working}>Cancel</Button>
                <Button variant="destructive" onClick={bulkDelete} isLoading={working}>
                  Delete
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </>
      )}

      {mode === "add-tag" && (
        <>
          <Input
            className="h-6 w-28"
            placeholder="key"
            value={tagKey}
            onChange={(e) => setTagKey(e.target.value)}
            onKeyDown={inlineEditKeyHandler({ onSave: bulkAddTag, onCancel: cancelMode })}
            autoFocus
          />
          <Input
            className="h-6 w-28"
            placeholder="value (optional)"
            value={tagValue}
            onChange={(e) => setTagValue(e.target.value)}
            onKeyDown={inlineEditKeyHandler({ onSave: bulkAddTag, onCancel: cancelMode })}
          />
          <Button variant="ghost" size="icon-xs" disabled={!tagKey.trim()} isLoading={working} onClick={bulkAddTag} title="Apply">
            <Check className="size-3" />
          </Button>
          <Button variant="ghost" size="icon-xs" disabled={working} onClick={cancelMode} title="Cancel">
            <X className="size-3" />
          </Button>
        </>
      )}

      {mode === "remove-tag" && (
        <>
          <NativeSelect
            size="sm"
            value={removeTagKey}
            onChange={(e) => setRemoveTagKey(e.target.value)}
            className="min-w-28"
          >
            {availableTagKeys.map((k) => (
              <NativeSelectOption key={k} value={k}>{k}</NativeSelectOption>
            ))}
          </NativeSelect>
          <Button variant="ghost" size="icon-xs" disabled={!removeTagKey} isLoading={working} onClick={bulkRemoveTag} title="Apply">
            <Check className="size-3" />
          </Button>
          <Button variant="ghost" size="icon-xs" disabled={working} onClick={cancelMode} title="Cancel">
            <X className="size-3" />
          </Button>
        </>
      )}

      {mode === "set-payee" && (
        <>
          <Input
            className="h-6 w-48"
            placeholder="payee name"
            value={payee}
            onChange={(e) => setPayee(e.target.value)}
            onKeyDown={inlineEditKeyHandler({ onSave: bulkSetPayee, onCancel: cancelMode })}
            autoFocus
          />
          <Button variant="ghost" size="icon-xs" disabled={!payee.trim()} isLoading={working} onClick={bulkSetPayee} title="Apply">
            <Check className="size-3" />
          </Button>
          <Button variant="ghost" size="icon-xs" disabled={working} onClick={cancelMode} title="Cancel">
            <X className="size-3" />
          </Button>
        </>
      )}

      {mode === "set-unknown-account" && (
        <>
          <AccountInput
            value={unknownAccount}
            onChange={setUnknownAccount}
            accounts={accounts}
            placeholder="Account"
            className="h-6 w-56"
          />
          <Button variant="ghost" size="icon-xs" disabled={!unknownAccount.trim()} isLoading={working} onClick={bulkUpdateUnknownAccount} title="Apply">
            <Check className="size-3" />
          </Button>
          <Button variant="ghost" size="icon-xs" disabled={working} onClick={cancelMode} title="Cancel">
            <X className="size-3" />
          </Button>
        </>
      )}

      {error && <span className="text-xs text-destructive">{error}</span>}

      <Button
        variant="ghost"
        size="xs"
        className="ml-auto"
        disabled={working}
        onClick={onClearSelection}
      >
        <X className="size-3" data-icon="inline-start" /> Clear
      </Button>
    </div>
  );
}

export function TransactionsPage() {
  const queryClient = useQueryClient();
  const router = useRouter();
  const navigate = useNavigate({ from: "/transactions" });
  const routeSearch = useSearch({ from: "/transactions" });

  // account/payee/search live in the URL (links such as the payee name in a
  // row navigate here with them), so they're read from the route on every
  // render rather than copied into state once on mount.
  const importBatchId = routeSearch.importBatchId || "";
  const account = routeSearch.account || "";
  const payee = routeSearch.payee || "";
  const routeSearchText = routeSearch.search || "";

  // The review links (sidebar count, Overview notice) open this page with
  // search=not:status:*. That is the "Needs review" chip over all dates, so
  // the count they show matches the list: it's turned into the chip (and
  // "All time") rather than a search under the default "This month".
  const reviewLink = routeSearchText.trim() === UNREVIEWED_QUERY;
  const search = reviewLink ? "" : routeSearchText;

  // Skip the date filter when viewing an import or arriving to review.
  const initialRange = importBatchId || reviewLink ? { from: "", to: "" } : DATE_PRESETS[0].fn();
  const [dateFrom, setDateFrom] = useState(initialRange.from);
  const [dateTo, setDateTo] = useState(initialRange.to);
  const [tag, setTag] = useState("");
  const [status, setStatus] = useState(reviewLink ? "unreviewed" : "");
  const [selectedFids, setSelectedFids] = useState(new Set());
  const reviewCount = useReviewCount();

  // A filter change from either the controls or a link clears the selection.
  useEffect(() => {
    setSelectedFids(new Set());
  }, [account, payee, search]);

  // Following a review link (also while already on this page) switches to
  // the chip and drops the search param, so clearing the chip clears it all.
  useEffect(() => {
    if (!reviewLink) return;
    setStatus("unreviewed");
    setDateFrom("");
    setDateTo("");
    setSelectedFids(new Set());
    navigate({ search: (prev) => ({ ...prev, search: "" }), replace: true });
  }, [reviewLink, navigate]);

  function setRouteFilter(key) {
    return (value) => navigate({ search: (prev) => ({ ...prev, [key]: value }), replace: true });
  }

  const isAccountMode = !!account;

  // Until the effect above settles, read a review link as its end state so
  // the first fetch already uses the final query.
  const query = buildQuery({
    dateFrom: reviewLink ? "" : dateFrom,
    dateTo: reviewLink ? "" : dateTo,
    tag,
    status: reviewLink ? "unreviewed" : status,
    payee,
    importBatchId,
    search,
  });
  const txParams = { query, limit: 0, offset: 0 };
  const aregParams = { account, query, limit: 0, offset: 0 };

  const { data: txData, isLoading: txLoading, error: txError } = useQuery({
    queryKey: queryKeys.transactions(txParams),
    queryFn: () => ledgerClient.listTransactions(txParams),
    enabled: !isAccountMode,
  });

  const { data: aregData, isLoading: aregLoading, error: aregError } = useQuery({
    queryKey: queryKeys.accountRegister(aregParams),
    queryFn: () => ledgerClient.getAccountRegister(aregParams),
    enabled: isAccountMode,
  });

  const { data: accountsData } = useQuery({
    queryKey: queryKeys.accounts(),
    queryFn: () => ledgerClient.listAccounts({}),
  });

  const { data: tagsData } = useQuery({
    queryKey: queryKeys.tags(),
    queryFn: () => ledgerClient.listTags({}),
  });

  const data = isAccountMode ? aregData : txData;
  const isLoading = isAccountMode ? aregLoading : txLoading;
  const error = isAccountMode ? aregError : txError;

  function onDateRangeChange(from, to) {
    setDateFrom(from);
    setDateTo(to);
    setSelectedFids(new Set());
  }

  function onFilterChange(setter) {
    return (value) => {
      setter(value);
      setSelectedFids(new Set());
    };
  }

  function onStatusChange() {
    invalidateLedgerQueries(queryClient);
  }

  function onBulkActionComplete() {
    setSelectedFids(new Set());
    invalidateLedgerQueries(queryClient);
  }

  const transactions = !isAccountMode ? (data?.transactions || []) : [];
  const registerRows = isAccountMode ? (data?.rows || []) : null;
  const bulkActionTransactions = isAccountMode ? (registerRows || []) : transactions;

  return (
    <Page className="gap-4">
      <PageHeader
        title="Transactions"
        actions={
          <Button variant="ghost" size="sm" onClick={() => router.navigate({ to: "/import" })}>
            <Upload data-icon="inline-start" />
            Import CSV
          </Button>
        }
      />
      {importBatchId && (
        <div className="-mt-2 flex items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            className="-ml-2"
            onClick={() => router.navigate({ to: "/imports" })}
          >
            <ArrowLeft data-icon="inline-start" />
            Import History
          </Button>
          <span className="font-mono text-code text-muted-foreground">{importBatchId}</span>
        </div>
      )}
      <SearchControls
        dateFrom={reviewLink ? "" : dateFrom}
        dateTo={reviewLink ? "" : dateTo}
        account={account}
        tag={tag}
        status={reviewLink ? "unreviewed" : status}
        payee={payee}
        search={search}
        onDateRangeChange={onDateRangeChange}
        onAccountChange={setRouteFilter("account")}
        onTagChange={onFilterChange(setTag)}
        onPayeeChange={setRouteFilter("payee")}
        onSearchChange={setRouteFilter("search")}
        onStatusChange={onFilterChange(setStatus)}
        accounts={accountsData?.accounts || []}
        tags={tagsData?.tags || []}
        reviewCount={reviewCount}
        summary={data && <TransactionSummary transactions={transactions} registerRows={registerRows} />}
      />
      {isLoading && <Loading />}
      {error && <ErrorBanner error={error} />}
      {data && (
        <div>
          {selectedFids.size > 0 && (
            <BulkActionBar
              selectedFids={selectedFids}
              transactions={bulkActionTransactions}
              accounts={accountsData?.accounts || []}
              focusedAccount={isAccountMode ? account : ""}
              onActionComplete={onBulkActionComplete}
              onClearSelection={() => setSelectedFids(new Set())}
            />
          )}
          <TransactionTable
            key={`${account}\u0000${query.join("\u0000")}`}
            transactions={transactions}
            registerRows={registerRows}
            focusedAccount={!isAccountMode ? account : undefined}
            onStatusChange={onStatusChange}
            onDeleted={onBulkActionComplete}
            accounts={accountsData?.accounts || []}
            selectedFids={selectedFids}
            onSelectionChange={setSelectedFids}
          />
        </div>
      )}
    </Page>
  );
}

const HLEDGER_KEYWORD_RE = /^(not:)?(acct|code|cur|desc|date|date2|tag|payee|note|status|real|depth|amt|type|sym|commodity|inacct|inaccteq):/;

// Free text searches descriptions literally; text that already contains an
// hledger query keyword is passed through as raw query tokens.
function searchTokens(search) {
  if (!search) return [];
  const parts = search.trim().split(/\s+/);
  if (parts.some((p) => HLEDGER_KEYWORD_RE.test(p))) {
    return parts;
  }
  return [`desc:${escapeRegex(search.trim())}`];
}

// buildQuery serves both the transaction list and the account register; the
// register's focused account is passed separately as GetAccountRegister.account.
function buildQuery({ dateFrom, dateTo, tag, status, payee, importBatchId, search }) {
  const tokens = [...dateRangeQuery(dateFrom, dateTo)];
  if (importBatchId) tokens.push(`tag:float-import=${exactRegex(importBatchId)}`);
  else if (tag) tokens.push(`tag:${exactRegex(tag)}`);
  if (payee === PAYEE_NONE) tokens.push("not:desc:.*[|].*");
  else if (payee) tokens.push(`payee:${exactRegex(payee)}`);
  if (status === "reviewed") tokens.push("status:*");
  if (status === "unreviewed") tokens.push("not:status:*");
  tokens.push(...searchTokens(search));
  return tokens;
}
