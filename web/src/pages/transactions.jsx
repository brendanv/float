import { useState } from "preact/hooks";
import { ledgerClient } from "../client.js";
import { useRpc } from "../hooks/use-rpc.js";
import { SearchControls, DATE_PRESETS } from "../components/search-controls.jsx";
import { TransactionTable } from "../components/transaction-table.jsx";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";

export function TransactionsPage({ params }) {
  const initialRange = DATE_PRESETS[0].fn(); // "This month"
  const [dateFrom, setDateFrom] = useState(initialRange.from);
  const [dateTo, setDateTo] = useState(initialRange.to);
  const [account, setAccount] = useState(params?.account || "");
  const [tag, setTag] = useState("");
  const [status, setStatus] = useState("");
  const [refreshKey, setRefreshKey] = useState(0);

  const query = buildQuery(dateFrom, dateTo, account, tag, status);

  const { data, loading, error } = useRpc(
    () => ledgerClient.listTransactions({ query }),
    [dateFrom, dateTo, account, tag, status, refreshKey]
  );

  const { data: accountsData } = useRpc(() => ledgerClient.listAccounts({}), []);
  const { data: tagsData } = useRpc(() => ledgerClient.listTags({}), []);

  function onDateRangeChange(from, to) {
    setDateFrom(from);
    setDateTo(to);
  }

  function onStatusChange() {
    setRefreshKey((k) => k + 1);
  }

  return (
    <div>
      <SearchControls
        dateFrom={dateFrom}
        dateTo={dateTo}
        account={account}
        tag={tag}
        status={status}
        onDateRangeChange={onDateRangeChange}
        onAccountChange={setAccount}
        onTagChange={setTag}
        onStatusChange={setStatus}
        accounts={accountsData?.accounts || []}
        tags={tagsData?.tags || []}
      />
      {loading && <Loading />}
      {error && <ErrorBanner error={error} />}
      {data && (
        <TransactionTable
          transactions={data.transactions || []}
          focusedAccount={account}
          onStatusChange={onStatusChange}
          accounts={accountsData?.accounts || []}
        />
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
