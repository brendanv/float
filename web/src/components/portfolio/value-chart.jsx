import { useId, useMemo } from "react";
import { Area, CartesianGrid, ComposedChart, LabelList, Line, XAxis, YAxis } from "recharts";
import {
  CHART_STROKE,
  CHART_X_AXIS,
  CHART_Y_AXIS,
  ChartAreaFill,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  ChartTooltipRow,
  chartEndLabel,
  compactCurrency,
} from "@/components/ui/chart";
import { Amount } from "@/components/amount.jsx";
import { Section } from "@/components/page.jsx";
import { Segmented } from "@/components/segmented.jsx";
import { PORTFOLIO_RANGES, formatLabel } from "./performance.js";

const RANGE_OPTIONS = PORTFOLIO_RANGES.map(({ label, value }) => ({ label, value }));

// ValueSection is the flat "Value over time" section; the range toggle also
// limits the history used for the period returns.
function ValueSection({ timeRange, onTimeRangeChange, children }) {
  return (
    <Section
      title="Value over time"
      action={
        <Segmented
          label="History range"
          options={RANGE_OPTIONS}
          value={timeRange}
          onChange={onTimeRangeChange}
        />
      }
    >
      {children}
    </Section>
  );
}

export function PortfolioChart({ snapshots, timeRange, onTimeRangeChange }) {
  const fillId = useId().replace(/:/g, "");
  const hasCostBasis = useMemo(
    () => snapshots?.some((s) => s.costBasis),
    [snapshots],
  );

  if (!snapshots || snapshots.length < 2) {
    return (
      <ValueSection timeRange={timeRange} onTimeRangeChange={onTimeRangeChange}>
        <p className="text-control text-muted-foreground">
          Not enough snapshots to chart portfolio value.
        </p>
      </ValueSection>
    );
  }

  const chartData = snapshots.map((s) => ({
    date: formatLabel(s.date),
    marketValue: s.totalValue ? parseFloat(s.totalValue.quantity) : null,
    ...(hasCostBasis && {
      costBasis: s.costBasis ? parseFloat(s.costBasis.quantity) : null,
    }),
  }));

  // Market value is the primary series (accent, area fill); cost basis is a
  // quiet dashed ink-3 line. Two series, so the line ends are labelled
  // instead of drawing a legend.
  const chartConfig = {
    marketValue: { label: "Market value", color: "var(--primary)" },
    ...(hasCostBasis && {
      costBasis: { label: "Cost basis", color: "var(--faint)" },
    }),
  };
  const last = chartData.length - 1;
  const currency = snapshots.find((s) => s.totalValue)?.totalValue?.commodity || "USD";

  return (
    <ValueSection timeRange={timeRange} onTimeRangeChange={onTimeRangeChange}>
      <ChartContainer config={chartConfig} className="portfolio-chart aspect-auto h-80 w-full">
        <ComposedChart
          accessibilityLayer
          data={chartData}
          margin={{ top: 8, left: 4, right: 84, bottom: 0 }}
        >
          <ChartAreaFill id={fillId} color="var(--color-marketValue)" />
          <CartesianGrid vertical={false} />
          <XAxis dataKey="date" {...CHART_X_AXIS} />
          <YAxis {...CHART_Y_AXIS} tickFormatter={(v) => compactCurrency(v, currency)} />
          <ChartTooltip
            cursor={false}
            content={
              <ChartTooltipContent
                formatter={(value, name, item) => (
                  <ChartTooltipRow color={item.color} label={chartConfig[name]?.label ?? name}>
                    <Amount value={value} commodity={currency} />
                  </ChartTooltipRow>
                )}
              />
            }
          />
          {hasCostBasis && (
            <Line
              dataKey="costBasis"
              type="monotone"
              stroke="var(--color-costBasis)"
              strokeWidth={CHART_STROKE}
              strokeDasharray="5 3"
              dot={false}
              isAnimationActive={false}
            >
              <LabelList dataKey="costBasis" content={chartEndLabel("Cost basis", "var(--color-costBasis)", last)} />
            </Line>
          )}
          <Area
            dataKey="marketValue"
            type="monotone"
            stroke="var(--color-marketValue)"
            strokeWidth={CHART_STROKE}
            fill={`url(#${fillId})`}
            dot={false}
            isAnimationActive={false}
          >
            <LabelList dataKey="marketValue" content={chartEndLabel("Market value", "var(--color-marketValue)", last)} />
          </Area>
        </ComposedChart>
      </ChartContainer>
    </ValueSection>
  );
}
