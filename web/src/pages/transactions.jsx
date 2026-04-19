import { useState } from "react";
import { useSearch, useRouter } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, ChevronLeft, ChevronRight, X, ArrowLeft } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { SearchControls, DATE_PRESETS, PAYEE_NONE } from "../components/search-controls.jsx";
import { TransactionTable } from "../components/transaction-table.jsx";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";

const PAGE_SIZE = 50;

function BulkActionBar({ selectedFids, transactions, onActionComplete, onClearSelection }) {
  const [mode, setMode] = useState("idle"); // 'idle' | 'add-tag' | 'remove-tag' | 'set-payee'
  const [working, setWorking] = useState(false);
  const [tagKey, setTagKey] = useState("");
  const [tagValue, setTagValue] = useState("");
  const [removeTagKey, setRemoveTagKey] = useState("");
  const [payee, setPayee] = useState("");
  const [error, setError] = useState(null);

  const selectedTxs = transactions.filter((tx) => selectedFids.has(tx.fid));
  const count = selectedTxs.length;
  const fids = selectedTxs.map((tx) => tx.fid);

  // Collect union of all tag keys across selected transactions
  const availableTagKeys = [...new Set(selectedTxs.flatMap((tx) => Object.keys(tx.tags || {})))].sort();

  function cancelMode() {
    setMode("idle");
    setTagKey("");
    setTagValue("");
    setRemoveTagKey("");
    setPayee("");
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

  return (
    <div className="mb-2 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted px-3 py-2 text-sm">
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
        </>
      )}

      {mode === "add-tag" && (
        <>
          <Input
            className="h-6 w-28"
            placeholder="tag key"
            value={tagKey}
            onChange={(e) => setTagKey(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") bulkAddTag(); if (e.key === "Escape") cancelMode(); }}
            autoFocus
          />
          <Input
            className="h-6 w-28"
            placeholder="value (optional)"
            value={tagValue}
            onChange={(e) => setTagValue(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") bulkAddTag(); if (e.key === "Escape") cancelMode(); }}
          />
          <Button size="xs" disabled={working || !tagKey.trim()} onClick={bulkAddTag}>
            {working ? <Loader2 className="size-3 animate-spin" /> : "Apply"}
          </Button>
          <Button variant="ghost" size="xs" disabled={working} onClick={cancelMode}>Cancel</Button>
        </>
      )}

      {mode === "remove-tag" && (
        <>
          <Select value={removeTagKey} onValueChange={setRemoveTagKey}>
            <SelectTrigger size="sm">
              <SelectValue>{removeTagKey}</SelectValue>
            </SelectTrigger>
            <SelectContent>
              {availableTagKeys.map((k) => (
                <SelectItem key={k} value={k}>{k}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button size="xs" disabled={working || !removeTagKey} onClick={bulkRemoveTag}>
            {working ? <Loader2 className="size-3 animate-spin" /> : "Apply"}
          </Button>
          <Button variant="ghost" size="xs" disabled={working} onClick={cancelMode}>Cancel</Button>
        </>
      )}

      {mode === "set-payee" && (
        <>
          <Input
            className="h-6 w-48"
            placeholder="payee name"
            value={payee}
            onChange={(e) => setPayee(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") bulkSetPayee(); if (e.key === "Escape") cancelMode(); }}
            autoFocus
          />
          <Button size="xs" disabled={working || !payee.trim()} onClick={bulkSetPayee}>
            {working ? <Loader2 className="size-3 animate-spin" /> : "Apply"}
          </Button>
          <Button variant="ghost" size="xs" disabled={working} onClick={cancelMode}>Cancel</Button>
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
  const routeSearch = useSearch({ from: "/transactions" });

  const importBatchId = routeSearch.importBatchId || "";
  const initialRange = importBatchId ? { from: "", to: "" } : DATE_PRESETS[0].fn(); // skip date filter when viewing import
  const [dateFrom, setDateFrom] = useState(initialRange.from);
  const [dateTo, setDateTo] = useState(initialRange.to);
  const [account, setAccount] = useState(routeSearch.account || "");
  const [tag, setTag] = useState("");
  const [status, setStatus] = useState("");
  const [payee, setPayee] = useState(routeSearch.payee || "");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const [selectedFids, setSelectedFids] = useState(new Set());

  const isAccountMode = !!account;

  const txQuery = buildQuery(dateFrom, dateTo, account, tag, status, payee, importBatchId, search);
  const txParams = { query: txQuery, limit: PAGE_SIZE, offset: page * PAGE_SIZE };

  const aregQuery = buildAregisterQuery(dateFrom, dateTo, tag, status, payee, importBatchId, search);
  const aregParams = { account, query: aregQuery, limit: PAGE_SIZE, offset: page * PAGE_SIZE };

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
    setPage(0);
    setSelectedFids(new Set());
  }

  function onFilterChange(setter) {
    return (value) => {
      setter(value);
      setPage(0);
      setSelectedFids(new Set());
    };
  }

  function onStatusChange() {
    if (isAccountMode) {
      queryClient.invalidateQueries({ queryKey: queryKeys.accountRegister(aregParams) });
    } else {
      queryClient.invalidateQueries({ queryKey: queryKeys.transactions(txParams) });
    }
  }

  function onBulkActionComplete() {
    setSelectedFids(new Set());
    queryClient.invalidateQueries({ queryKey: ["transactions"] });
    queryClient.invalidateQueries({ queryKey: ["accountRegister"] });
  }

  function applyQuickFilter(filters) {
    setDateFrom(filters.dateFrom);
    setDateTo(filters.dateTo);
    setAccount(filters.account);
    setTag(filters.tag);
    setStatus(filters.status);
    setPayee(filters.payee);
    setPage(0);
    setSelectedFids(new Set());
  }

  const total = data?.total ?? 0;
  const hasNext = data?.hasNext ?? false;
  const totalPages = Math.ceil(total / PAGE_SIZE);
  const rangeStart = total === 0 ? 0 : page * PAGE_SIZE + 1;
  const rangeEnd = Math.min((page + 1) * PAGE_SIZE, total);

  const transactions = !isAccountMode ? (data?.transactions || []) : [];
  const registerRows = isAccountMode ? (data?.rows || []) : null;

  return (
    <div className="flex flex-col gap-6">
      {importBatchId && (
        <div className="mb-3 flex items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => router.navigate({ to: "/imports" })}
            className="gap-1.5"
          >
            <ArrowLeft data-icon="inline-start" />
            Import History
          </Button>
          <span className="text-sm text-muted-foreground font-mono">{importBatchId}</span>
        </div>
      )}
      <SearchControls
        dateFrom={dateFrom}
        dateTo={dateTo}
        account={account}
        tag={tag}
        status={status}
        payee={payee}
        search={search}
        onDateRangeChange={onDateRangeChange}
        onAccountChange={onFilterChange(setAccount)}
        onTagChange={onFilterChange(setTag)}
        onPayeeChange={onFilterChange(setPayee)}
        onSearchChange={onFilterChange(setSearch)}
        onQuickFilter={applyQuickFilter}
        accounts={accountsData?.accounts || []}
        tags={tagsData?.tags || []}
      />
      {isLoading && <Loading />}
      {error && <ErrorBanner error={error} />}
      {data && (
        <>
          {!isAccountMode && selectedFids.size > 0 && (
            <BulkActionBar
              selectedFids={selectedFids}
              transactions={transactions}
              onActionComplete={onBulkActionComplete}
              onClearSelection={() => setSelectedFids(new Set())}
            />
          )}
          <TransactionTable
            transactions={transactions}
            registerRows={registerRows}
            focusedAccount={!isAccountMode ? account : undefined}
            onStatusChange={onStatusChange}
            onDeleted={onBulkActionComplete}
            accounts={accountsData?.accounts || []}
            selectedFids={selectedFids}
            onSelectionChange={setSelectedFids}
          />
          {totalPages > 1 && (
            <>
            <Separator />
            <div className="flex items-center justify-between px-2 py-3">
              <span className="text-sm text-muted-foreground">
                {rangeStart}–{rangeEnd} of {total}
              </span>
              <div className="flex items-center gap-1">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPage((p) => p - 1)}
                  disabled={page === 0}
                >
                  <ChevronLeft />
                </Button>
                <span className="px-2 text-sm tabular-nums text-muted-foreground">
                  {page + 1} / {totalPages}
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPage((p) => p + 1)}
                  disabled={!hasNext}
                >
                  <ChevronRight />
                </Button>
              </div>
            </div>
            </>
          )}
        </>
      )}
    </div>
  );
}

function buildQuery(dateFrom, dateTo, account, tag, status, payee, importBatchId, search) {
  const tokens = [];
  if (dateFrom && dateTo) tokens.push(`date:${dateFrom}..${dateTo}`);
  if (account) tokens.push(`acct:${account}`);
  if (importBatchId) tokens.push(`tag:float-import=${importBatchId}`);
  else if (tag) tokens.push(`tag:${tag}`);
  if (payee === PAYEE_NONE) tokens.push("not:payee:.+");
  else if (payee) tokens.push(`payee:${payee}`);
  if (status === "reviewed") tokens.push("status:*");
  if (status === "unreviewed") tokens.push("not:status:*");
  if (search) tokens.push(`desc:${search}`);
  return tokens;
}

function buildAregisterQuery(dateFrom, dateTo, tag, status, payee, importBatchId, search) {
  const tokens = [];
  if (dateFrom && dateTo) tokens.push(`date:${dateFrom}..${dateTo}`);
  if (importBatchId) tokens.push(`tag:float-import=${importBatchId}`);
  else if (tag) tokens.push(`tag:${tag}`);
  if (payee === PAYEE_NONE) tokens.push("not:payee:.+");
  else if (payee) tokens.push(`payee:${payee}`);
  if (status === "reviewed") tokens.push("status:*");
  if (status === "unreviewed") tokens.push("not:status:*");
  if (search) tokens.push(`desc:${search}`);
  return tokens;
}
