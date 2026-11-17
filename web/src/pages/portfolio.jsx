import { useState, useMemo } from "react";
import { Link } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { FunnelIcon } from "@phosphor-icons/react";
import { ledgerClient } from "../client.js";
import { queryKeys, invalidateLedgerQueries } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { Amount } from "../components/amount.jsx";
import { MetricHero, Page, Section } from "../components/page.jsx";
import { ExclusionsDialog, usePortfolioExclusions } from "../components/portfolio/exclusions.jsx";
import { PortfolioChart } from "../components/portfolio/value-chart.jsx";
import { MultiPeriodReturns } from "../components/portfolio/returns.jsx";
import { AllocationChart } from "../components/portfolio/allocation.jsx";
import { HoldingsTable } from "../components/portfolio/holdings-table.jsx";
import {
  STALE_PRICE_DAYS,
  beginDateForRange,
  buildPortfolioPerformance,
  daysSinceDate,
  formatPct,
  gainTone,
} from "../components/portfolio/performance.js";
import { Notice } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

function plural(n, word) {
  return `${n} ${word}${n === 1 ? "" : "s"}`;
}

// PriceNotice is the one-line missing/stale price warning with its actions:
// a link to Prices and "Generate from purchases" (GeneratePricesFromCost).
function PriceNotice({ variant, linkLabel, generate, children }) {
  const linkTone = variant === "critical" ? "text-below" : "text-foreground";
  return (
    <Notice
      variant={variant}
      action={
        <>
          <Link to="/prices" className="touch-target">{linkLabel}</Link>
          <span aria-hidden="true" className="text-faint">·</span>
          <button
            type="button"
            className={cn("touch-target font-medium underline-offset-3 hover:underline disabled:opacity-50", linkTone)}
            disabled={generate.isPending}
            onClick={() => generate.mutate()}
          >
            {generate.isPending ? "Generating…" : "Generate from purchases"}
          </button>
        </>
      }
    >
      {children}
      {generate.isError && (
        <span className="ml-2 text-below">{generate.error?.message ?? "Failed"}</span>
      )}
    </Notice>
  );
}

function PortfolioHero({ totalValue, currencyTotals, asOfDate, performance, holdingsCount }) {
  const mixed = currencyTotals.length > 1;
  const gain = performance?.investmentGain;
  const gainPct = performance && performance.netContributions > 0
    ? (gain / performance.netContributions) * 100
    : null;

  const sub = [
    performance ? (
      <span key="gain">Investment gain</span>
    ) : (
      <span key="gain">No cost basis data</span>
    ),
    performance && (
      <span key="contrib">
        <Amount value={performance.netContributions.toFixed(2)} commodity={performance.currency} /> net contributions
      </span>
    ),
    <span key="count">{plural(holdingsCount, "holding")}</span>,
    mixed ? (
      <span key="asof">Multiple currencies — values not summed</span>
    ) : asOfDate ? (
      <span key="asof">as of {asOfDate}</span>
    ) : null,
  ].filter(Boolean);

  return (
    <MetricHero
      label="Portfolio value"
      amount={totalValue ?? (mixed ? currencyTotals : undefined)}
      value={!totalValue && !mixed ? "—" : undefined}
      delta={performance && (
        <>
          <Amount value={gain.toFixed(2)} commodity={performance.currency} signed />
          {gainPct !== null && <span>· {formatPct(gainPct)}</span>}
        </>
      )}
      deltaTone={gainTone(gain)}
      sub={
        <span className="inline-flex flex-wrap gap-x-1.5">
          {sub.map((part, i) => (
            <span key={part.key} className="inline-flex gap-x-1.5">
              {i > 0 && <span aria-hidden="true">·</span>}
              {part}
            </span>
          ))}
        </span>
      }
    />
  );
}

export function PortfolioPage() {
  const [accountPrefix, setAccountPrefix] = useState("");
  const [sorting, setSorting] = useState([{ id: "currentValue", desc: true }]);
  const [timeRange, setTimeRange] = useState("all");
  const [exclusionsOpen, setExclusionsOpen] = useState(false);
  const timeseriesBegin = useMemo(
    () => beginDateForRange(timeRange),
    [timeRange],
  );
  const queryClient = useQueryClient();
  const generateMutation = useMutation({
    mutationFn: () => ledgerClient.generatePricesFromCost({ accountPrefix }),
    onSuccess: () => {
      invalidateLedgerQueries(queryClient);
    },
  });

  const { excludedSymbols, excludedAccountPrefixes, totalExclusions, update: updateExclusions } = usePortfolioExclusions();
  const activeFilterCount = (accountPrefix ? 1 : 0) + totalExclusions;

  const { data, isLoading, error } = useQuery({
    queryKey: queryKeys.portfolioHoldings(accountPrefix),
    queryFn: () => ledgerClient.getPortfolioHoldings({ accountPrefix }),
  });

  const { data: tsData } = useQuery({
    queryKey: queryKeys.portfolioTimeseries(accountPrefix, timeseriesBegin),
    queryFn: () =>
      ledgerClient.getPortfolioTimeseries({
        accountPrefix,
        begin: timeseriesBegin,
      }),
  });

  const holdings = data?.holdings ?? [];
  const totalValue = data?.totalValue;
  const currencyTotals = data?.currencyTotals ?? [];
  const asOfDate = data?.asOfDate;
  const snapshots = tsData?.snapshots ?? [];
  const valuedCount = holdings.filter((h) => h.currentValue).length;
  const unvaluedCount = holdings.length - valuedCount;
  const staleCount = holdings.filter((h) => {
    const days = daysSinceDate(h.priceDate);
    return h.currentValue && days !== null && days > STALE_PRICE_DAYS;
  }).length;

  const performance = useMemo(
    () => buildPortfolioPerformance(snapshots, holdings),
    [snapshots, holdings],
  );

  function handleExcludeSymbol(symbol) {
    if (!excludedSymbols.includes(symbol)) {
      updateExclusions([...excludedSymbols, symbol], excludedAccountPrefixes);
    }
  }

  return (
    <Page className="gap-7">
      <ExclusionsDialog
        open={exclusionsOpen}
        onOpenChange={setExclusionsOpen}
        accountPrefix={accountPrefix}
        onAccountPrefixChange={setAccountPrefix}
      />
      <PageHeader title="Investment Portfolio">
        <Button
          variant={activeFilterCount > 0 ? "secondary" : "outline"}
          size="sm"
          className="h-8 gap-1.5"
          onClick={() => setExclusionsOpen(true)}
        >
          <FunnelIcon size={14} />
          Filters
          {activeFilterCount > 0 && (
            <Badge variant="default" className="h-4 px-1 text-[10px]">
              {activeFilterCount}
            </Badge>
          )}
        </Button>
      </PageHeader>

      {isLoading && <Loading />}
      {error && <ErrorBanner error={error} />}

      {data && holdings.length === 0 && (
        <p className="text-control text-muted-foreground">
          No non-currency commodity holdings found under{" "}
          <code className="font-mono text-code">{accountPrefix || "assets"}</code>. Record investment
          transactions with commodity amounts (e.g.{" "}
          <code className="font-mono text-code">10 AAPL @ 175 USD</code>) to see your portfolio here.
        </p>
      )}

      {data && holdings.length > 0 && (
        <>
          <PortfolioHero
            totalValue={totalValue}
            currencyTotals={currencyTotals}
            asOfDate={asOfDate}
            performance={performance}
            holdingsCount={holdings.length}
          />

          {(unvaluedCount > 0 || staleCount > 0) && (
            <div className="flex flex-col gap-2">
              {unvaluedCount > 0 && (
                <PriceNotice variant="critical" linkLabel="Add prices" generate={generateMutation}>
                  <span className="font-medium">{plural(unvaluedCount, "holding")}</span>{" "}
                  missing price data.
                </PriceNotice>
              )}
              {staleCount > 0 && (
                <PriceNotice variant="warning" linkLabel="Update prices" generate={generateMutation}>
                  <span className="font-medium">{plural(staleCount, "holding")}</span>{" "}
                  with price data older than {STALE_PRICE_DAYS} days.
                </PriceNotice>
              )}
            </div>
          )}

          <div className="grid gap-x-12 gap-y-8 xl:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
            <PortfolioChart
              snapshots={snapshots}
              timeRange={timeRange}
              onTimeRangeChange={setTimeRange}
            />
            <div className="flex min-w-0 flex-col gap-8">
              <MultiPeriodReturns snapshots={snapshots} />
              <AllocationChart holdings={holdings} currencyTotals={currencyTotals} />
            </div>
          </div>

          <Section title="Holdings" className="gap-0">
            <HoldingsTable
              holdings={holdings}
              sorting={sorting}
              onSortingChange={setSorting}
              onExcludeSymbol={handleExcludeSymbol}
            />
          </Section>
        </>
      )}
    </Page>
  );
}
