import { useState } from "preact/hooks";
import { ledgerClient } from "../client.js";
import { useRpc } from "../hooks/use-rpc.js";
import { PeriodSelector } from "../components/period-selector.jsx";
import { FilterInput } from "../components/filter-input.jsx";
import { TransactionTable } from "../components/transaction-table.jsx";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { navigate } from "../router.jsx";

function pad2(n) {
  return n < 10 ? "0" + n : "" + n;
}

export function TransactionsPage({ params }) {
  const now = new Date();
  const [year, setYear] = useState(now.getFullYear());
  const [month, setMonth] = useState(now.getMonth() + 1);
  const [filter, setFilter] = useState("");
  const [refreshKey, setRefreshKey] = useState(0);

  const accountFilter = params?.account || "";

  const query = buildQuery(year, month, filter, accountFilter);

  const { data, loading, error } = useRpc(
    () => ledgerClient.listTransactions({ query }),
    [year, month, filter, accountFilter, refreshKey]
  );

  const { data: accountsData } = useRpc(() => ledgerClient.listAccounts({}), []);

  function onPeriodChange(y, m) {
    setYear(y);
    setMonth(m);
  }

  function onStatusChange() {
    setRefreshKey((k) => k + 1);
  }

  return (
    <div>
      {accountFilter && (
        <div class="alert mb-4">
          <span>Filtered to: <strong>{accountFilter}</strong></span>
          <a
            class="btn btn-ghost btn-xs"
            href="#/transactions"
            onClick={(e) => { e.preventDefault(); navigate("/transactions"); }}
          >
            clear
          </a>
        </div>
      )}
      <PeriodSelector year={year} month={month} onChange={onPeriodChange} />
      <FilterInput value={filter} onChange={setFilter} />
      {loading && <Loading />}
      {error && <ErrorBanner error={error} />}
      {data && (
        <TransactionTable
          transactions={data.transactions || []}
          focusedAccount={accountFilter}
          onStatusChange={onStatusChange}
          accounts={accountsData?.accounts || []}
        />
      )}
    </div>
  );
}

function buildQuery(year, month, filter, account) {
  const tokens = [`date:${year}-${pad2(month)}`];
  if (account) tokens.push(account);
  if (filter.trim()) {
    tokens.push(...filter.trim().split(/\s+/));
  }
  return tokens;
}
