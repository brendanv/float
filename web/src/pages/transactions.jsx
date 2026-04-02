import { useState } from "preact/hooks";
import { ledgerClient } from "../client.js";
import { useRpc } from "../hooks/use-rpc.js";
import { SearchControls, DATE_PRESETS } from "../components/search-controls.jsx";
import { TransactionTable } from "../components/transaction-table.jsx";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { formatAmounts } from "../format.js";

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

  async function runBulk(fn) {
    setWorking(true);
    setError(null);
    try {
      for (const tx of selectedTxs) {
        await fn(tx);
      }
      cancelMode();
      onActionComplete();
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setWorking(false);
    }
  }

  function bulkMarkStatus(status) {
    return runBulk((tx) => ledgerClient.updateTransactionStatus({ fid: tx.fid, status }));
  }

  function bulkAddTag() {
    if (!tagKey.trim()) return;
    return runBulk((tx) => ledgerClient.modifyTags({ fid: tx.fid, tags: { ...(tx.tags || {}), [tagKey.trim()]: tagValue.trim() } }));
  }

  function bulkRemoveTag() {
    if (!removeTagKey) return;
    return runBulk((tx) => {
      const next = Object.fromEntries(Object.entries(tx.tags || {}).filter(([k]) => k !== removeTagKey));
      return ledgerClient.modifyTags({ fid: tx.fid, tags: next });
    });
  }

  function bulkSetPayee() {
    if (!payee.trim()) return;
    return runBulk((tx) => ledgerClient.updateTransaction({
      fid: tx.fid,
      description: tx.description,
      date: tx.date,
      postings: (tx.postings || []).map((p) => ({ account: p.account, amount: formatAmounts(p.amounts) })),
      payee: payee.trim(),
    }));
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
            onClick={() => bulkMarkStatus("Cleared")}
            title="Mark selected as reviewed"
          >
            Mark reviewed
          </button>
          <button
            class="btn btn-xs btn-ghost"
            disabled={working}
            onClick={() => bulkMarkStatus("Pending")}
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

export function TransactionsPage({ params }) {
  const initialRange = DATE_PRESETS[0].fn(); // "This month"
  const [dateFrom, setDateFrom] = useState(initialRange.from);
  const [dateTo, setDateTo] = useState(initialRange.to);
  const [account, setAccount] = useState(params?.account || "");
  const [tag, setTag] = useState("");
  const [status, setStatus] = useState("");
  const [payee, setPayee] = useState(params?.payee || "");
  const [refreshKey, setRefreshKey] = useState(0);
  const [page, setPage] = useState(0);
  const [selectedFids, setSelectedFids] = useState(new Set());

  const query = buildQuery(dateFrom, dateTo, account, tag, status, payee);

  const { data, loading, error } = useRpc(
    () =>
      ledgerClient.listTransactions({
        query,
        limit: PAGE_SIZE,
        offset: page * PAGE_SIZE,
      }),
    [dateFrom, dateTo, account, tag, status, payee, refreshKey, page]
  );

  const { data: accountsData } = useRpc(() => ledgerClient.listAccounts({}), []);
  const { data: tagsData } = useRpc(() => ledgerClient.listTags({}), []);

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
    setRefreshKey((k) => k + 1);
  }

  function onBulkActionComplete() {
    setSelectedFids(new Set());
    setRefreshKey((k) => k + 1);
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
        onStatusChange={onFilterChange(setStatus)}
        onPayeeChange={onFilterChange(setPayee)}
        accounts={accountsData?.accounts || []}
        tags={tagsData?.tags || []}
      />
      {loading && <Loading />}
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
            <div class="flex items-center justify-between px-4 py-3 border-t border-gray-200 dark:border-gray-700">
              <p class="text-sm text-gray-600 dark:text-gray-400">
                {rangeStart}–{rangeEnd} of {total}
              </p>
              <div class="flex gap-2">
                <button
                  onClick={() => setPage((p) => p - 1)}
                  disabled={page === 0}
                  class="px-3 py-1 text-sm rounded border border-gray-300 dark:border-gray-600 disabled:opacity-40 hover:bg-gray-100 dark:hover:bg-gray-700 disabled:cursor-not-allowed"
                >
                  Previous
                </button>
                <span class="px-3 py-1 text-sm text-gray-600 dark:text-gray-400">
                  {page + 1} / {totalPages}
                </span>
                <button
                  onClick={() => setPage((p) => p + 1)}
                  disabled={!hasNext}
                  class="px-3 py-1 text-sm rounded border border-gray-300 dark:border-gray-600 disabled:opacity-40 hover:bg-gray-100 dark:hover:bg-gray-700 disabled:cursor-not-allowed"
                >
                  Next
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
  if (payee) tokens.push(`payee:${payee}`);
  if (status === "reviewed") tokens.push("status:*");
  if (status === "unreviewed") tokens.push("not:status:*");
  return tokens;
}
