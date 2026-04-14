import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { AccountList } from "../components/account-list.jsx";
import { BalanceSummary } from "../components/balance-summary.jsx";
import { InsightsChart } from "../components/insights-chart.jsx";
import { DATE_PRESETS, PeriodBar } from "../components/search-controls.jsx";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { Card, CardContent } from "@/components/ui/card";

export function HomePage() {
  const initial = DATE_PRESETS[0].fn();
  const [dateFrom, setDateFrom] = useState(initial.from);
  const [dateTo, setDateTo] = useState(initial.to);

  const periodQuery = dateFrom && dateTo ? [`date:${dateFrom}..${dateTo}`] : [];

  const { data: accountsData, isLoading: accountsLoading, error: accountsError } = useQuery({
    queryKey: queryKeys.accounts(),
    queryFn: () => ledgerClient.listAccounts({}),
  });

  const { data: balancesData } = useQuery({
    queryKey: queryKeys.balances({ depth: 1 }),
    queryFn: () => ledgerClient.getBalances({ depth: 1 }),
  });

  const { data: accountBalancesData } = useQuery({
    queryKey: queryKeys.balances({}),
    queryFn: () => ledgerClient.getBalances({}),
  });

  const balanceRows = balancesData?.report?.rows || [];
  const allAccounts = accountsData?.accounts || [];
  const sidebarAccounts = allAccounts.filter((a) => a.type === "A" || a.type === "C" || a.type === "L");
  const accountBalanceRows = accountBalancesData?.report?.rows || [];

  return (
    <div>
      <PeriodBar dateFrom={dateFrom} dateTo={dateTo} onChange={(from, to) => { setDateFrom(from); setDateTo(to); }} />
      <BalanceSummary balanceRows={balanceRows} />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <Card className="lg:col-span-1">
          <CardContent>
            {accountsLoading && <Loading />}
            {accountsError && <ErrorBanner error={accountsError} />}
            {accountsData && (
              <AccountList
                accounts={sidebarAccounts}
                balanceRows={accountBalanceRows}
              />
            )}
          </CardContent>
        </Card>
        <Card className="lg:col-span-2">
          <CardContent>
            <InsightsChart periodQuery={periodQuery} />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
