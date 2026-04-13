import { useState } from "react";
import { useSearch } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { SearchControls, DATE_PRESETS, PAYEE_NONE } from "../components/search-controls.jsx";
import { TransactionTable } from "../components/transaction-table.jsx";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";

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
    return runBulk([{ markReviewed: { reviewed } }]);
  }

  function bulkAddTag() {
    if (!tagKey.trim()) return;
    return runBulk([{ addTag: { key: tagKey.trim(), value: tagValue.trim() } }]);
  }

  function bulkRemoveTag() {
    if (!removeTagKey) return;
    return runBulk([{ removeTag: { key: removeTagKey } }]);
  }

  function bulkSetPayee() {
    if (!payee.trim()) return;
    return runBulk([{ setPayee: { payee: payee.trim() } }]);
  }

  return (
    <div class="flex flex-wrap items-center gap-2 px-3 py-2 mb-2 rounded-lg bg-base-200 border border-base-300 text-sm">
      <span class="font-medium text-base-content/80 shrink-0">
        {count} selected
      </span>

      {mode === "idle" && (
        <>
          <button
            class="btn btn-xs btn-ghost"
            disabled={working}
            onClick={() => bulkMarkStatus(true)}
            title="Mark selected as reviewed"
          >
            Mark reviewed
          </button>
          <button
            class="btn btn-xs btn-ghost"
            disabled={working}
            onClick={() => bulkMarkStatus(false)}
            title="Mark selected as unreviewed"
          >
            Mark unreviewed
          </button>
          <button
            class="btn btn-xs btn-ghost"
            disabled={working}
            onClick={() => setMode("add-tag")}
          >
            Add tag
          </button>
          <button
            class="btn btn-xs btn-ghost"
            disabled={working || availableTagKeys.length === 0}
            onClick={() => { setRemoveTagKey(availableTagKeys[0] || ""); setMode("remove-tag"); }}
            title={availableTagKeys.length === 0 ? "No tags on selected transactions" : undefined}
          >
            Remove tag
          </button>
          <button
            class="btn btn-xs btn-ghost"
            disabled={working}
            onClick={() => setMode("set-payee")}
          >
            Set payee
          </button>
        </>
      )}

      {mode === "add-tag" && (
        <>
          <input
            class="input input-xs input-bordered w-28"
            placeholder="tag key"
            value={tagKey}
            onInput={(e) => setTagKey(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") bulkAddTag(); if (e.key === "Escape") cancelMode(); }}
            autoFocus
          />
          <input
            class="input input-xs input-bordered w-28"
            placeholder="value (optional)"
            value={tagValue}
            onInput={(e) => setTagValue(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") bulkAddTag(); if (e.key === "Escape") cancelMode(); }}
          />
          <button class="btn btn-xs btn-primary" disabled={working || !tagKey.trim()} onClick={bulkAddTag}>
            {working ? <span class="loading loading-spinner loading-xs" /> : "Apply"}
          </button>
          <button class="btn btn-xs btn-ghost" disabled={working} onClick={cancelMode}>Cancel</button>
        </>
      )}

      {mode === "remove-tag" && (
        <>
          <select
            class="select select-xs select-bordered"
            value={removeTagKey}
            onChange={(e) => setRemoveTagKey(e.target.value)}
          >
            {availableTagKeys.map((k) => <option key={k} value={k}>{k}</option>)}
          </select>
          <button class="btn btn-xs btn-primary" disabled={working || !removeTagKey} onClick={bulkRemoveTag}>
            {working ? <span class="loading loading-spinner loading-xs" /> : "Apply"}
          </button>
          <button class="btn btn-xs btn-ghost" disabled={working} onClick={cancelMode}>Cancel</button>
        </>
      )}

      {mode === "set-payee" && (
        <>
          <input
            class="input input-xs input-bordered w-48"
            placeholder="payee name"
            value={payee}
            onInput={(e) => setPayee(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") bulkSetPayee(); if (e.key === "Escape") cancelMode(); }}
            autoFocus
          />
          <button class="btn btn-xs btn-primary" disabled={working || !payee.trim()} onClick={bulkSetPayee}>
            {working ? <span class="loading loading-spinner loading-xs" /> : "Apply"}
          </button>
          <button class="btn btn-xs btn-ghost" disabled={working} onClick={cancelMode}>Cancel</button>
        </>
      )}

      {error && <span class="text-error text-xs">{error}</span>}

      <button
        class="btn btn-xs btn-ghost ml-auto"
        disabled={working}
        onClick={onClearSelection}
        title="Clear selection"
      >
        ✕ Clear
      </button>
    </div>
  );
}

export function TransactionsPage() {
  const queryClient = useQueryClient();
  const routeSearch = useSearch({ from: "/transactions" });

  const initialRange = DATE_PRESETS[0].fn(); // "This month"
  const [dateFrom, setDateFrom] = useState(initialRange.from);
  const [dateTo, setDateTo] = useState(initialRange.to);
  const [account, setAccount] = useState(routeSearch.account || "");
  const [tag, setTag] = useState("");
  const [status, setStatus] = useState("");
  const [payee, setPayee] = useState(routeSearch.payee || "");
  const [page, setPage] = useState(0);
  const [selectedFids, setSelectedFids] = useState(new Set());

  const query = buildQuery(dateFrom, dateTo, account, tag, status, payee);
  const queryParams = { query, limit: PAGE_SIZE, offset: page * PAGE_SIZE };

  const { data, isLoading, error } = useQuery({
    queryKey: queryKeys.transactions(queryParams),
    queryFn: () => ledgerClient.listTransactions(queryParams),
  });

  const { data: accountsData } = useQuery({
    queryKey: queryKeys.accounts(),
    queryFn: () => ledgerClient.listAccounts({}),
  });

  const { data: tagsData } = useQuery({
    queryKey: queryKeys.tags(),
    queryFn: () => ledgerClient.listTags({}),
  });

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
    queryClient.invalidateQueries({ queryKey: queryKeys.transactions(queryParams) });
  }

  function onBulkActionComplete() {
    setSelectedFids(new Set());
    queryClient.invalidateQueries({ queryKey: ["transactions"] });
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

  const transactions = data?.transactions || [];

  return (
    <div>
      <SearchControls
        dateFrom={dateFrom}
        dateTo={dateTo}
        account={account}
        tag={tag}
        status={status}
        payee={payee}
        onDateRangeChange={onDateRangeChange}
        onAccountChange={onFilterChange(setAccount)}
        onTagChange={onFilterChange(setTag)}
        onPayeeChange={onFilterChange(setPayee)}
        onQuickFilter={applyQuickFilter}
        accounts={accountsData?.accounts || []}
        tags={tagsData?.tags || []}
      />
      {isLoading && <Loading />}
      {error && <ErrorBanner error={error} />}
      {data && (
        <>
          {selectedFids.size > 0 && (
            <BulkActionBar
              selectedFids={selectedFids}
              transactions={transactions}
              onActionComplete={onBulkActionComplete}
              onClearSelection={() => setSelectedFids(new Set())}
            />
          )}
          <TransactionTable
            transactions={transactions}
            focusedAccount={account}
            onStatusChange={onStatusChange}
            accounts={accountsData?.accounts || []}
            selectedFids={selectedFids}
            onSelectionChange={setSelectedFids}
          />
          {totalPages > 1 && (
            <div class="flex items-center justify-between px-2 py-3 border-t border-base-300">
              <span class="text-sm text-base-content/60">
                {rangeStart}–{rangeEnd} of {total}
              </span>
              <div class="join">
                <button
                  class="join-item btn btn-sm"
                  onClick={() => setPage((p) => p - 1)}
                  disabled={page === 0}
                >
                  ‹
                </button>
                <button class="join-item btn btn-sm btn-disabled pointer-events-none">
                  {page + 1} / {totalPages}
                </button>
                <button
                  class="join-item btn btn-sm"
                  onClick={() => setPage((p) => p + 1)}
                  disabled={!hasNext}
                >
                  ›
                </button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}

function buildQuery(dateFrom, dateTo, account, tag, status, payee) {
  const tokens = [];
  if (dateFrom && dateTo) tokens.push(`date:${dateFrom}..${dateTo}`);
  if (account) tokens.push(`acct:${account}`);
  if (tag) tokens.push(`tag:${tag}`);
  if (payee === PAYEE_NONE) tokens.push("not:payee:.+");
  else if (payee) tokens.push(`payee:${payee}`);
  if (status === "reviewed") tokens.push("status:*");
  if (status === "unreviewed") tokens.push("not:status:*");
  return tokens;
}
