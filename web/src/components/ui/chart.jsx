import * as React from "react"
import * as RechartsPrimitive from "recharts"

import { cn } from "@/lib/utils"
import { Amount } from "@/components/amount.jsx"

// Format: { THEME_NAME: CSS_SELECTOR }
const THEMES = {
  light: "",
  dark: ".dark"
}

const INITIAL_DIMENSION = {
  width: 320,
  height: 200
}

const ChartContext = React.createContext(null)

function useChart() {
  const context = React.useContext(ChartContext)

  if (!context) {
    throw new Error("useChart must be used within a <ChartContainer />")
  }

  return context
}

function ChartContainer({
  id,
  className,
  children,
  config,
  initialDimension = INITIAL_DIMENSION,
  ...props
}) {
  const uniqueId = React.useId()
  const chartId = `chart-${id ?? uniqueId.replace(/:/g, "")}`

  return (
    <ChartContext.Provider value={{ config }}>
      <div
        data-slot="chart"
        data-chart={chartId}
        className={cn(
          "flex aspect-video justify-center text-xs [&_.recharts-cartesian-axis-tick_text]:fill-faint [&_.recharts-cartesian-grid_line[stroke='#ccc']]:stroke-border [&_.recharts-curve.recharts-tooltip-cursor]:stroke-border [&_.recharts-dot[stroke='#fff']]:stroke-transparent [&_.recharts-layer]:outline-hidden [&_.recharts-polar-grid_[stroke='#ccc']]:stroke-border [&_.recharts-radial-bar-background-sector]:fill-muted [&_.recharts-rectangle.recharts-tooltip-cursor]:fill-muted [&_.recharts-reference-line_[stroke='#ccc']]:stroke-border [&_.recharts-sector]:outline-hidden [&_.recharts-sector[stroke='#fff']]:stroke-transparent [&_.recharts-surface]:outline-hidden",
          className
        )}
        {...props}>
        <ChartStyle id={chartId} config={config} />
        <RechartsPrimitive.ResponsiveContainer initialDimension={initialDimension}>
          {children}
        </RechartsPrimitive.ResponsiveContainer>
      </div>
    </ChartContext.Provider>
  );
}

const ChartStyle = ({
  id,
  config
}) => {
  const colorConfig = Object.entries(config).filter(([, config]) => config.theme ?? config.color)

  if (!colorConfig.length) {
    return null
  }

  return (
    <style
      dangerouslySetInnerHTML={{
        __html: Object.entries(THEMES)
          .map(([theme, prefix]) => `
${prefix} [data-chart=${id}] {
${colorConfig
.map(([key, itemConfig]) => {
const color =
  itemConfig.theme?.[theme] ??
  itemConfig.color
return color ? `  --color-${key}: ${color};` : null
})
.join("\n")}
}
`)
          .join("\n"),
      }} />
  );
}

// ChartTooltip is recharts' Tooltip without its slide animation, so the
// tooltip tracks the pointer and screenshots are stable. Items are listed
// largest first, which matches the lines' vertical order.
const byValueDesc = (item) => -(Number(item?.value) || 0)

function ChartTooltip(props) {
  return <RechartsPrimitive.Tooltip isAnimationActive={false} itemSorter={byValueDesc} {...props} />
}

function ChartTooltipContent({
  active,
  payload,
  className,
  indicator = "dot",
  hideLabel = false,
  hideIndicator = false,
  label,
  labelFormatter,
  labelClassName,
  formatter,
  color,
  nameKey,
  labelKey,
  commodity
}) {
  const { config } = useChart()

  const tooltipLabel = React.useMemo(() => {
    if (hideLabel || !payload?.length) {
      return null
    }

    const [item] = payload
    const key = `${labelKey ?? item?.dataKey ?? item?.name ?? "value"}`
    const itemConfig = getPayloadConfigFromPayload(config, item, key)
    const value =
      !labelKey && typeof label === "string"
        ? (config[label]?.label ?? label)
        : itemConfig?.label

    if (labelFormatter) {
      return (
        <div className={cn("font-medium", labelClassName)}>
          {labelFormatter(value, payload)}
        </div>
      );
    }

    if (!value) {
      return null
    }

    return <div className={cn("font-medium", labelClassName)}>{value}</div>;
  }, [
    label,
    labelFormatter,
    payload,
    hideLabel,
    labelClassName,
    config,
    labelKey,
  ])

  if (!active || !payload?.length) {
    return null
  }

  const nestLabel = payload.length === 1 && indicator !== "dot"

  return (
    <div
      className={cn(
        "grid min-w-36 items-start gap-1.5 rounded-lg bg-popover px-2.5 py-2 text-[12px] text-popover-foreground shadow-md ring-1 ring-foreground/10",
        className
      )}>
      {!nestLabel ? tooltipLabel : null}
      <div className="grid gap-1.5">
        {payload
          .filter((item) => item.type !== "none")
          .map((item, index) => {
            const key = `${nameKey ?? item.name ?? item.dataKey ?? "value"}`
            const itemConfig = getPayloadConfigFromPayload(config, item, key)
            const indicatorColor = color ?? item.payload?.fill ?? item.color

            return (
              <div
                key={index}
                className={cn(
                  "flex w-full flex-wrap items-stretch gap-2 [&>svg]:h-2.5 [&>svg]:w-2.5 [&>svg]:text-muted-foreground",
                  indicator === "dot" && "items-center"
                )}>
                {formatter && item?.value !== undefined && item.name ? (
                  formatter(item.value, item.name, item, index, item.payload)
                ) : (
                  <>
                    {itemConfig?.icon ? (
                      <itemConfig.icon />
                    ) : (
                      !hideIndicator && (
                        <div
                          className={cn("shrink-0 rounded-[2px] border-(--color-border) bg-(--color-bg)", {
                            "h-2.5 w-2.5": indicator === "dot",
                            "w-1": indicator === "line",
                            "w-0 border-[1.5px] border-dashed bg-transparent":
                              indicator === "dashed",
                            "my-0.5": nestLabel && indicator === "dashed",
                          })}
                          style={
                            {
                              "--color-bg": indicatorColor,
                              "--color-border": indicatorColor
                            }
                          } />
                      )
                    )}
                    <div
                      className={cn(
                        "flex flex-1 justify-between leading-none",
                        nestLabel ? "items-end" : "items-center"
                      )}>
                      <div className="grid gap-1.5">
                        {nestLabel ? tooltipLabel : null}
                        <span className="text-muted-foreground">
                          {itemConfig?.label ?? item.name}
                        </span>
                      </div>
                      {item.value != null && (
                        commodity ? (
                          <Amount value={item.value} commodity={commodity} className="font-medium text-foreground" />
                        ) : (
                          <span className="font-medium text-foreground tabular-nums">
                            {typeof item.value === "number"
                              ? item.value.toLocaleString()
                              : String(item.value)}
                          </span>
                        )
                      )}
                    </div>
                  </>
                )}
              </div>
            );
          })}
      </div>
    </div>
  );
}

const ChartLegend = RechartsPrimitive.Legend

function ChartLegendContent({
  className,
  hideIcon = false,
  payload,
  verticalAlign = "bottom",
  nameKey
}) {
  const { config } = useChart()

  if (!payload?.length) {
    return null
  }

  return (
    <div
      className={cn(
        "flex items-center justify-center gap-4",
        verticalAlign === "top" ? "pb-3" : "pt-3",
        className
      )}>
      {payload
        .filter((item) => item.type !== "none")
        .map((item, index) => {
          const key = `${nameKey ?? item.dataKey ?? "value"}`
          const itemConfig = getPayloadConfigFromPayload(config, item, key)

          return (
            <div
              key={index}
              className={cn(
                "flex items-center gap-1.5 [&>svg]:h-3 [&>svg]:w-3 [&>svg]:text-muted-foreground"
              )}>
              {itemConfig?.icon && !hideIcon ? (
                <itemConfig.icon />
              ) : (
                <div
                  className="h-2 w-2 shrink-0 rounded-[2px]"
                  style={{
                    backgroundColor: item.color,
                  }} />
              )}
              {itemConfig?.label}
            </div>
          );
        })}
    </div>
  );
}

function getPayloadConfigFromPayload(
  config,
  payload,
  key
) {
  if (typeof payload !== "object" || payload === null) {
    return undefined
  }

  const payloadPayload =
    "payload" in payload &&
    typeof payload.payload === "object" &&
    payload.payload !== null
      ? payload.payload
      : undefined

  let configLabelKey = key

  if (
    key in payload &&
    typeof payload[key] === "string"
  ) {
    configLabelKey = payload[key]
  } else if (
    payloadPayload &&
    key in payloadPayload &&
    typeof payloadPayload[key] === "string"
  ) {
    configLabelKey = payloadPayload[key]
  }

  return configLabelKey in config ? config[configLabelKey] : config[key]
}

// --- Waterline chart conventions -------------------------------------------
// Lines are 1.8px; the primary series gets a soft area fill (ChartAreaFill)
// and every line ends in a dot. With three or fewer series, label the line
// ends (chartEndLabel) instead of drawing a legend. Axis ticks are ink-3 and
// grid lines are the rule color (set on ChartContainer). Entry animations
// are off (isAnimationActive={false} on every series, the tooltip has it
// built in) so charts render settled and screenshots are stable.

const CHART_STROKE = 1.8

// Shared axis props: no tick or axis lines, quiet spacing.
const CHART_X_AXIS = { tickLine: false, axisLine: false, tickMargin: 8, minTickGap: 24 }
const CHART_Y_AXIS = { tickLine: false, axisLine: false, tickMargin: 4, width: 52 }

const compactFormatters = new Map()

// compactCurrency formats an axis tick as "$12.5K". Non-fiat commodities
// fall back to a compact number.
function compactCurrency(value, commodity = "USD") {
  let fmt = compactFormatters.get(commodity)
  if (!fmt) {
    try {
      fmt = new Intl.NumberFormat("en-US", {
        style: "currency",
        currency: commodity,
        notation: "compact",
        maximumFractionDigits: 1,
      })
    } catch {
      fmt = new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 1 })
    }
    compactFormatters.set(commodity, fmt)
  }
  return fmt.format(value)
}

// ChartAreaFill is the vertical gradient under the primary series. Render it
// inside the chart and use fill={`url(#${id})`} on the <Area>.
function ChartAreaFill({ id, color, opacity = 0.14 }) {
  return (
    <defs>
      <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
        <stop offset="0" stopColor={color} stopOpacity={opacity} />
        <stop offset="1" stopColor={color} stopOpacity={0} />
      </linearGradient>
    </defs>
  )
}

// chartEndLabel returns a <LabelList content> renderer that draws the
// endpoint dot and, when `name` is set, the series name beside it. Leave
// about 76px of right margin for the name.
function chartEndLabel(name, color, lastIndex) {
  function EndLabel({ x, y, index }) {
    if (index !== lastIndex || x == null || y == null) return null
    return (
      <g data-slot="chart-end-label">
        <circle cx={x} cy={y} r={3} fill={color} stroke="var(--background)" strokeWidth={1.5} />
        {name && (
          <text x={x + 8} y={y} dy="0.35em" fill={color} fontSize={11.5} fontWeight={500}>
            {name}
          </text>
        )}
      </g>
    )
  }
  return EndLabel
}

// ChartTooltipRow is one tooltip line for a ChartTooltipContent formatter:
// a swatch, the series label in ink-2 and the value (usually an <Amount>).
function ChartTooltipRow({ color, label, children }) {
  return (
    <>
      <div className="h-2.5 w-2.5 shrink-0 rounded-[2px]" style={{ backgroundColor: color }} />
      <div className="flex flex-1 items-center justify-between gap-3 leading-none">
        <span className="text-muted-foreground">{label}</span>
        <span className="font-medium text-foreground">{children}</span>
      </div>
    </>
  )
}

export {
  CHART_STROKE,
  CHART_X_AXIS,
  CHART_Y_AXIS,
  ChartAreaFill,
  ChartTooltipRow,
  chartEndLabel,
  compactCurrency,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  ChartLegend,
  ChartLegendContent,
  ChartStyle,
}
