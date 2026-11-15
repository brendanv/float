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
import { OPEN_ACCOUNTS_QUERY } from "@/lib/accounts";
import { dateRangeQuery } from "@/lib/hledger-query";

const SUMMARY_PARAMS = { depth: 1, value: "now,USD", query: OPEN_ACCOUNTS_QUERY };
// Asset and liability balances of open accounts as an hledger tree, so
// parent rows carry hledger's inclusive totals.
const ACCOUNT_TREE_PARAMS = { value: "now,USD", tree: true, query: [...OPEN_ACCOUNTS_QUERY, "type:AL"] };

export function HomePage() {
  const initial = DATE_PRESETS[0].fn();
  const [dateFrom, setDateFrom] = useState(initial.from);
  const [dateTo, setDateTo] = useState(initial.to);

  const periodQuery = dateRangeQuery(dateFrom, dateTo);

  const { data: accountsData } = useQuery({
    queryKey: queryKeys.accounts(),
    queryFn: () => ledgerClient.listAccounts({}),
  });

  const { data: balancesData } = useQuery({
    queryKey: queryKeys.balances(SUMMARY_PARAMS),
    queryFn: () => ledgerClient.getBalances(SUMMARY_PARAMS),
  });

  const { data: treeData, isLoading: treeLoading, error: treeError } = useQuery({
    queryKey: queryKeys.balances(ACCOUNT_TREE_PARAMS),
    queryFn: () => ledgerClient.getBalances(ACCOUNT_TREE_PARAMS),
  });

  return (
    <Page>
      <PageHeader title="Home" description="Where your money is today, and where it went." />

      <DashboardGrid>
        <div className="col-span-12">
          <BalanceSummary balanceRows={balancesData?.report?.rows || []} />
        </div>

        <PageCard
          title="Accounts"
          description="Current balances"
          className="col-span-12 h-full lg:col-span-5 xl:col-span-4"
        >
          {treeLoading && <Loading />}
          {treeError && <ErrorBanner error={treeError} />}
          {treeData && (
            <AccountList rows={treeData.report?.rows || []} accounts={accountsData?.accounts || []} />
          )}
        </PageCard>
        <PageCard
          title="Insights"
          description="Spending and income for the selected period."
          action={
            <PeriodBar dateFrom={dateFrom} dateTo={dateTo} onChange={(from, to) => { setDateFrom(from); setDateTo(to); }} />
          }
          className="col-span-12 h-full lg:col-span-7 xl:col-span-8"
          contentClassName="min-h-[320px]"
        >
          <InsightsChart periodQuery={periodQuery} />
        </PageCard>
      </DashboardGrid>
    </Page>
  );
}
