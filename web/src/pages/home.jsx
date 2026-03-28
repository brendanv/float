import { useState } from "preact/hooks";
import { ledgerClient } from "../client.js";
import { useRpc } from "../hooks/use-rpc.js";
import { AccountList } from "../components/account-list.jsx";
import { BalanceSummary } from "../components/balance-summary.jsx";
import { InsightsChart } from "../components/insights-chart.jsx";
import { TransactionTable } from "../components/transaction-table.jsx";
import { PeriodSelector } from "../components/period-selector.jsx";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";

function pad2(n) {
  return n < 10 ? "0" + n : "" + n;
}

export function HomePage() {
  const now = new Date();
  const [year, setYear] = useState(now.getFullYear());
  const [month, setMonth] = useState(now.getMonth() + 1);

  const periodQuery = [`date:${year}-${pad2(month)}`];

  const accounts = useRpc(() => ledgerClient.listAccounts({}), []);
  const balances = useRpc(() => ledgerClient.getBalances({ depth: 1 }), []);
  const accountBalances = useRpc(() => ledgerClient.getBalances({}), []);
  const txns = useRpc(
    () => ledgerClient.listTransactions({ query: periodQuery }),
    [year, month]
  );

  function onPeriodChange(y, m) {
    setYear(y);
    setMonth(m);
  }

  const balanceRows = balances.data?.report?.rows || [];
  const allAccounts = accounts.data?.accounts || [];
  const sidebarAccounts = allAccounts.filter((a) => a.type === "A" || a.type === "C" || a.type === "L");
  const accountBalanceRows = accountBalances.data?.report?.rows || [];

  return (
    <div>
      <PeriodSelector year={year} month={month} onChange={onPeriodChange} />
      <BalanceSummary balanceRows={balanceRows} />

      <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div class="lg:col-span-1 space-y-6">
          <div class="card bg-base-100 shadow-sm">
            <div class="card-body p-4">
              {accounts.loading && <Loading />}
              {accounts.error && <ErrorBanner error={accounts.error} />}
              {accounts.data && (
                <AccountList
                  accounts={sidebarAccounts}
                  balanceRows={accountBalanceRows}
                />
              )}
            </div>
          </div>
          <div class="card bg-base-100 shadow-sm">
            <div class="card-body p-4">
              <InsightsChart periodQuery={periodQuery} />
            </div>
          </div>
        </div>
        <div class="lg:col-span-2">
          <div class="card bg-base-100 shadow-sm">
            <div class="card-body p-4">
              <h4 class="card-title text-base mb-2">Transactions</h4>
              {txns.loading && <Loading />}
              {txns.error && <ErrorBanner error={txns.error} />}
              {txns.data && (
                <TransactionTable transactions={txns.data.transactions || []} />
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
