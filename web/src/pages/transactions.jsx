import { useState } from "preact/hooks";
import { ledgerClient } from "../client.js";
import { useRpc } from "../hooks/use-rpc.js";
import { SearchControls, DATE_PRESETS } from "../components/search-controls.jsx";
import { TransactionTable } from "../components/transaction-table.jsx";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";

const PAGE_SIZE = 50;

export function TransactionsPage({ params }) {
  const initialRange = DATE_PRESETS[0].fn(); // "This month"
  const [dateFrom, setDateFrom] = useState(initialRange.from);
  const [dateTo, setDateTo] = useState(initialRange.to);
  const [account, setAccount] = useState(params?.account || "");
  const [tag, setTag] = useState("");
  const [status, setStatus] = useState("");
  const [refreshKey, setRefreshKey] = useState(0);
  const [page, setPage] = useState(0);

  const query = buildQuery(dateFrom, dateTo, account, tag, status);

  const { data, loading, error } = useRpc(
    () =>
      ledgerClient.listTransactions({
        query,
        limit: PAGE_SIZE,
        offset: page * PAGE_SIZE,
      }),
    [dateFrom, dateTo, account, tag, status, refreshKey, page]
  );

  const { data: accountsData } = useRpc(() => ledgerClient.listAccounts({}), []);
  const { data: tagsData } = useRpc(() => ledgerClient.listTags({}), []);

  function onDateRangeChange(from, to) {
    setDateFrom(from);
    setDateTo(to);
    setPage(0);
  }

  function onFilterChange(setter) {
    return (value) => {
      setter(value);
      setPage(0);
    };
  }

  function onStatusChange() {
    setRefreshKey((k) => k + 1);
  }

  const total = data?.total ?? 0;
  const totalPages = Math.ceil(total / PAGE_SIZE);
  const rangeStart = total === 0 ? 0 : page * PAGE_SIZE + 1;
  const rangeEnd = Math.min((page + 1) * PAGE_SIZE, total);

  return (
    <div>
      <SearchControls
        dateFrom={dateFrom}
        dateTo={dateTo}
        account={account}
        tag={tag}
        status={status}
        onDateRangeChange={onDateRangeChange}
        onAccountChange={onFilterChange(setAccount)}
        onTagChange={onFilterChange(setTag)}
        onStatusChange={onFilterChange(setStatus)}
        accounts={accountsData?.accounts || []}
        tags={tagsData?.tags || []}
      />
      {loading && <Loading />}
      {error && <ErrorBanner error={error} />}
      {data && (
        <>
          <TransactionTable
            transactions={data.transactions || []}
            focusedAccount={account}
            onStatusChange={onStatusChange}
            accounts={accountsData?.accounts || []}
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
                  disabled={page >= totalPages - 1}
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

function buildQuery(dateFrom, dateTo, account, tag, status) {
  const tokens = [];
  if (dateFrom && dateTo) tokens.push(`date:${dateFrom}..${dateTo}`);
  if (account) tokens.push(`acct:${account}`);
  if (tag) tokens.push(`tag:${tag}`);
  if (status === "reviewed") tokens.push("status:*");
  if (status === "unreviewed") tokens.push("not:status:*");
  return tokens;
}
