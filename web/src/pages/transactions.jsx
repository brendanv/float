import { useState } from "preact/hooks";
import { ledgerClient } from "../client.js";
import { useRpc } from "../hooks/use-rpc.js";
import { PeriodSelector } from "../components/period-selector.jsx";
import { FilterInput } from "../components/filter-input.jsx";
import { TransactionTable } from "../components/transaction-table.jsx";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";

function pad2(n) {
  return n < 10 ? "0" + n : "" + n;
}

export function TransactionsPage({ params }) {
  const now = new Date();
  const [year, setYear] = useState(now.getFullYear());
  const [month, setMonth] = useState(now.getMonth() + 1);
  const [filter, setFilter] = useState("");

  const accountFilter = params?.account || "";

  const query = buildQuery(year, month, filter, accountFilter);

  const { data, loading, error } = useRpc(
    () => ledgerClient.listTransactions({ query }),
    [year, month, filter, accountFilter]
  );

  function onPeriodChange(y, m) {
    setYear(y);
    setMonth(m);
  }

  return (
    <div>
      {accountFilter && (
        <p>
          Filtered to: <strong>{accountFilter}</strong>{" "}
          <a href="#/transactions">clear</a>
        </p>
      )}
      <PeriodSelector year={year} month={month} onChange={onPeriodChange} />
      <FilterInput value={filter} onChange={setFilter} />
      {loading && <Loading />}
      {error && <ErrorBanner error={error} />}
      {data && <TransactionTable transactions={data.transactions || []} />}
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
