import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { ArrowRight } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { monthName } from "../format.js";
import { AccountList } from "../components/account-list.jsx";
import { Amount } from "../components/amount.jsx";
import { InsightsChart } from "../components/insights-chart.jsx";
import { PeriodBar } from "../components/search-controls.jsx";
import { DATE_PRESETS } from "../components/search-presets.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { MetricHero, Page, Section } from "../components/page.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { Notice } from "@/components/ui/alert";
import { UNREVIEWED_SEARCH, useReviewCount } from "@/hooks/use-review-count.js";
import { OPEN_ACCOUNTS_QUERY } from "@/lib/accounts";
import { dateRangeQuery } from "@/lib/hledger-query";
import {
  deltaTone,
  firstQuantity,
  netWorthChanges,
  netWorthSummary,
  netWorthSummaryQuery,
  netWorthTimeseriesQuery,
  shortMonthLabel,
  timeseriesBegin,
} from "@/lib/net-worth.js";

// Asset and liability balances of open accounts as an hledger tree, so
// parent rows carry hledger's inclusive totals.
const ACCOUNT_TREE_PARAMS = { value: "now,USD", tree: true, query: [...OPEN_ACCOUNTS_QUERY, "type:AL"] };
const SPARK_MONTHS = 12;

// spendingTitle names the spending block after the period when it is one
// calendar month ("November spending").
function spendingTitle(from, to) {
  const m = /^(\d{4})-(\d{2})-01$/.exec(from || "");
  if (m && to) {
    const next = new Date(Date.UTC(+m[1], +m[2], 1)).toISOString().slice(0, 10);
    if (next === to) return `${monthName(parseInt(m[2], 10))} spending`;
  }
  return "Spending";
}

function ReviewNotice() {
  const count = useReviewCount();
  if (!count) return null;
  return (
    <Notice
      variant="info"
      action={
        <Link
          to="/transactions"
          search={UNREVIEWED_SEARCH}
          className="touch-target inline-flex items-center gap-1 font-medium text-primary hover:underline"
        >
          Review
          <ArrowRight className="size-3.5" aria-hidden="true" />
        </Link>
      }
    >
      <span className="font-medium">
        {count} {count === 1 ? "transaction" : "transactions"}
      </span>{" "}
      {count === 1 ? "needs" : "need"} review.
    </Notice>
  );
}

// Waterline draws assets and liabilities at true scale, split by a dashed
// rule. Liabilities are negative in hledger and shown as an owed amount.
function Waterline({ assets, liabilities }) {
  const a = Math.max(firstQuantity(assets), 0);
  const l = Math.max(-firstQuantity(liabilities), 0);
  const max = Math.max(a, l, 1);
  return (
    <div
      data-slot="waterline"
      role="group"
      aria-label="Assets versus liabilities"
      className="grid grid-cols-[4rem_minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1.5 text-[12.5px] md:grid-cols-[5.25rem_minmax(0,1fr)_6rem]"
    >
      <span className="text-muted-foreground">Assets</span>
      <span className="h-2.5 rounded-[2px] bg-acct-asset" style={{ width: `${(a / max) * 100}%` }} />
      <Amount value={assets} className="text-right" />
      <span className="relative col-span-full h-0 border-t border-dashed border-input">
        <span className="absolute -top-[9px] right-0 bg-background pl-1.5 text-[10.5px] tracking-[0.06em] text-faint uppercase">
          Waterline
        </span>
      </span>
      <span className="text-muted-foreground">Liabilities</span>
      <span className="h-2.5 rounded-[2px] bg-acct-liability" style={{ width: `${(l / max) * 100}%` }} />
      <Amount value={liabilities} negate className="text-right" />
    </div>
  );
}

export function HomePage() {
  const initial = DATE_PRESETS[0].fn();
  const [dateFrom, setDateFrom] = useState(initial.from);
  const [dateTo, setDateTo] = useState(initial.to);

  const periodQuery = dateRangeQuery(dateFrom, dateTo);

  const { data: accountsData } = useQuery({
    queryKey: queryKeys.accounts(),
    queryFn: () => ledgerClient.listAccounts({}),
  });

  const { data: balancesData } = useQuery(netWorthSummaryQuery());

  const { data: treeData, isLoading: treeLoading, error: treeError } = useQuery({
    queryKey: queryKeys.balances(ACCOUNT_TREE_PARAMS),
    queryFn: () => ledgerClient.getBalances(ACCOUNT_TREE_PARAMS),
  });

  // Same query (and cache entry) as Reports › Net worth's default range.
  // GetNetWorthTimeseries takes no query, so unlike the hero figure its
  // history still includes closed accounts.
  const { data: timeseriesData } = useQuery(netWorthTimeseriesQuery(timeseriesBegin(SPARK_MONTHS)));

  const { assets, liabilities, netWorth, commodity } = netWorthSummary(balancesData?.report);
  const hasSummary = netWorth !== null;

  const snapshots = (timeseriesData?.snapshots || []).slice(-SPARK_MONTHS);
  const { monthChange, prevMonth, ytdChange } = netWorthChanges(snapshots, netWorth);

  return (
    <Page className="gap-7">
      <PageHeader title="Overview">
        <PeriodBar dateFrom={dateFrom} dateTo={dateTo} onChange={(from, to) => { setDateFrom(from); setDateTo(to); }} />
      </PageHeader>

      <ReviewNotice />

      {hasSummary && (
        <div className="flex flex-col gap-6">
          <MetricHero
            label="Net worth"
            amount={{ commodity, quantity: String(netWorth) }}
            tone={netWorth < 0 ? "below" : "ink"}
            delta={monthChange !== null && (
              <>
                <Amount value={monthChange} commodity={commodity} signed />
                {prevMonth && ` vs ${prevMonth}`}
              </>
            )}
            deltaTone={deltaTone(monthChange)}
            sub={ytdChange !== null && (
              <><Amount value={ytdChange} commodity={commodity} signed /> this year</>
            )}
            sparkline={snapshots.length >= 2 ? {
              values: snapshots.map((s) => firstQuantity(s.netWorth)),
              label: `Net worth, ${shortMonthLabel(snapshots[0].date)} to ${shortMonthLabel(snapshots.at(-1).date)}`,
              axis: [shortMonthLabel(snapshots[0].date), shortMonthLabel(snapshots.at(-1).date)],
            } : undefined}
          />
          <Waterline assets={assets} liabilities={liabilities} />
        </div>
      )}

      <div className="grid gap-x-12 gap-y-8 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)]">
        <Section title="Accounts" description="Balances today" headerClassName="[&>span]:ml-auto">
          {treeLoading && <Loading />}
          {treeError && <ErrorBanner error={treeError} />}
          {treeData && (
            <AccountList rows={treeData.report?.rows || []} accounts={accountsData?.accounts || []} />
          )}
        </Section>
        <Section title={spendingTitle(dateFrom, dateTo)}>
          <InsightsChart periodQuery={periodQuery} />
        </Section>
      </div>
    </Page>
  );
}
