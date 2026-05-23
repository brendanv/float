import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { AccountList } from "../components/account-list.jsx";
import { BalanceSummary } from "../components/balance-summary.jsx";
import { InsightsChart } from "../components/insights-chart.jsx";
import { PeriodBar } from "../components/search-controls.jsx";
import { DATE_PRESETS } from "../components/search-presets.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { DashboardGrid, Page, PageCard } from "../components/page.jsx";
import { PageHeader } from "../components/page-header.jsx";

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
    queryKey: queryKeys.balances({ depth: 1, value: "now,USD" }),
    queryFn: () => ledgerClient.getBalances({ depth: 1, value: "now,USD" }),
  });

  const { data: accountBalancesData } = useQuery({
    queryKey: queryKeys.balances({ value: "now,USD" }),
    queryFn: () => ledgerClient.getBalances({ value: "now,USD" }),
  });

  const balanceRows = balancesData?.report?.rows || [];
  const allAccounts = accountsData?.accounts || [];
  const sidebarAccounts = allAccounts.filter((a) => a.type === "A" || a.type === "C" || a.type === "L");
  const accountBalanceRows = accountBalancesData?.report?.rows || [];

  return (
    <Page>
      <PageHeader title="Home" description="Balances, account coverage, and activity for the selected period.">
        <PeriodBar dateFrom={dateFrom} dateTo={dateTo} onChange={(from, to) => { setDateFrom(from); setDateTo(to); }} />
      </PageHeader>

      <DashboardGrid>
        <div className="col-span-12">
          <BalanceSummary balanceRows={balanceRows} />
        </div>

        <PageCard title="Accounts" className="col-span-12 h-full xl:col-span-4">
          {accountsLoading && <Loading />}
          {accountsError && <ErrorBanner error={accountsError} />}
          {accountsData && (
            <AccountList
              accounts={sidebarAccounts}
              balanceRows={accountBalanceRows}
            />
          )}
        </PageCard>
        <PageCard
          title="Insights"
          description="Income and expense concentration for this period."
          className="col-span-12 h-full xl:col-span-8"
          contentClassName="min-h-[320px]"
        >
          <InsightsChart periodQuery={periodQuery} />
        </PageCard>
      </DashboardGrid>
    </Page>
  );
}
