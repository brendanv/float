import { useState } from "preact/hooks";
import { ledgerClient } from "../client.js";
import { useRpc } from "../hooks/use-rpc.js";
import { AccountList } from "../components/account-list.jsx";
import { BalanceSummary } from "../components/balance-summary.jsx";
import { InsightsChart } from "../components/insights-chart.jsx";
import { DATE_PRESETS, PeriodBar } from "../components/search-controls.jsx";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";

export function HomePage() {
  const initial = DATE_PRESETS[0].fn();
  const [dateFrom, setDateFrom] = useState(initial.from);
  const [dateTo, setDateTo] = useState(initial.to);

  const periodQuery = dateFrom && dateTo ? [`date:${dateFrom}..${dateTo}`] : [];

  const accounts = useRpc(() => ledgerClient.listAccounts({}), []);
  const balances = useRpc(() => ledgerClient.getBalances({ depth: 1 }), []);
  const accountBalances = useRpc(() => ledgerClient.getBalances({}), []);

  const balanceRows = balances.data?.report?.rows || [];
  const allAccounts = accounts.data?.accounts || [];
  const sidebarAccounts = allAccounts.filter((a) => a.type === "A" || a.type === "C" || a.type === "L");
  const accountBalanceRows = accountBalances.data?.report?.rows || [];

  return (
    <div>
      <PeriodBar dateFrom={dateFrom} dateTo={dateTo} onChange={(from, to) => { setDateFrom(from); setDateTo(to); }} />
      <BalanceSummary balanceRows={balanceRows} />

      <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div class="card bg-base-100 shadow-sm lg:col-span-1">
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
        <div class="card bg-base-100 shadow-sm lg:col-span-2">
          <div class="card-body p-4">
            <InsightsChart periodQuery={periodQuery} />
          </div>
        </div>
      </div>
    </div>
  );
}
