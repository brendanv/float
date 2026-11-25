import { useId, useState } from "react";
import { cn } from "@/lib/utils";
import { Amount } from "./amount.jsx";

export function Page({ className, ...props }) {
  return (
    <div
      data-slot="page"
      className={cn("flex flex-col gap-8", className)}
      {...props}
    />
  );
}

// Section is a semantic page section (<section> + <h2>): a heading over a
// hairline with an optional description and action, content straight on the
// page.
export function Section({ title, description, action, children, className, headerClassName, ...props }) {
  const id = useId();
  return (
    <section
      data-slot="section"
      aria-labelledby={title ? id : undefined}
      className={cn("flex min-w-0 flex-col gap-3", className)}
      {...props}
    >
      {(title || description || action) && (
        <div className={cn("flex min-h-8 flex-wrap items-end gap-x-3 gap-y-1 border-b border-border pb-2", headerClassName)}>
          {title && <h2 id={id} className="text-heading font-semibold">{title}</h2>}
          {description && <span className="text-control text-muted-foreground">{description}</span>}
          {/* The action slot overhangs upward so controls up to 32px keep every
              section header the same height (side-by-side hairlines align). */}
          {action && <div className="ml-auto flex items-center gap-2 sm:-mt-2">{action}</div>}
        </div>
      )}
      {children}
    </section>
  );
}

// Eyebrow is the 11px uppercase eyebrow used over figures and in group rows.
export function Eyebrow({ className, ...props }) {
  return <span data-slot="eyebrow" className={cn("text-label uppercase text-faint", className)} {...props} />;
}

const DELTA_TONE = {
  inflow: "bg-inflow-soft text-inflow",
  below: "bg-below-soft text-below",
  ink: "bg-muted text-foreground",
};

// DeltaChip is the pill beside a hero figure ("+$650.00 vs October").
// tone: "inflow" (up), "below" (down) or "ink" (neutral).
export function DeltaChip({ tone = "ink", className, ...props }) {
  return (
    <span
      data-slot="delta-chip"
      className={cn(
        "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[12.5px] leading-4 font-medium whitespace-nowrap tabular-nums",
        DELTA_TONE[tone] ?? DELTA_TONE.ink,
        className,
      )}
      {...props}
    />
  );
}

// Sparkline draws a small accent line with a soft area fill over `values`
// (numbers, oldest first). `label` is the accessible name; `axis` is an
// optional [start, end] pair of captions under the line. With `labels`
// (parallel to `values`) hovering or arrow-keying the line shows that point's
// label and value; `formatValue(value, index)` renders the value (default:
// the number as-is).
export function Sparkline({ values, labels, formatValue, label, axis, className }) {
  const gradientId = useId();
  const [active, setActive] = useState(null);
  const points = (values || [])
    .map((v, i) => ({ v: Number(v), i }))
    .filter((p) => Number.isFinite(p.v));
  if (points.length < 2) return null;

  const W = 240;
  const H = 72;
  const pad = 4;
  const nums = points.map((p) => p.v);
  const min = Math.min(...nums);
  const max = Math.max(...nums);
  const span = max - min || 1;
  const xy = points.map(({ v }, i) => [
    (i / (points.length - 1)) * W,
    pad + (1 - (v - min) / span) * (H - pad * 2),
  ]);
  const line = xy.map(([x, y], i) => `${i ? "L" : "M"}${x.toFixed(1)} ${y.toFixed(1)}`).join(" ");
  const area = `${line} L${W} ${H} L0 ${H} Z`;

  const interactive = Array.isArray(labels);
  const hovered = interactive && active !== null && active < points.length ? active : null;
  const indexAt = (e) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const frac = rect.width ? (e.clientX - rect.left) / rect.width : 0;
    return Math.min(points.length - 1, Math.max(0, Math.round(frac * (points.length - 1))));
  };
  const onKeyDown = (e) => {
    const last = points.length - 1;
    const cur = hovered ?? last;
    const next = { ArrowLeft: cur - 1, ArrowRight: cur + 1, Home: 0, End: last }[e.key];
    if (next === undefined) return;
    e.preventDefault();
    setActive(Math.min(last, Math.max(0, next)));
  };

  let readout = null;
  if (hovered !== null) {
    const { v, i } = points[hovered];
    const [x, y] = xy[hovered];
    const xPct = (x / W) * 100;
    const yPct = (y / H) * 100;
    // Keep the readout inside the chart at the edges.
    const shift = xPct < 20 ? "0%" : xPct > 80 ? "-100%" : "-50%";
    readout = (
      <>
        <span
          aria-hidden="true"
          className="pointer-events-none absolute size-2 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-background bg-primary"
          style={{ left: `${xPct}%`, top: `${yPct}%` }}
        />
        <div
          data-slot="sparkline-readout"
          role="status"
          className="pointer-events-none absolute bottom-full z-10 mb-1.5 flex flex-col whitespace-nowrap rounded-md border bg-popover px-2 py-1 text-xs text-popover-foreground shadow-sm"
          style={{ left: `${xPct}%`, transform: `translateX(${shift})` }}
        >
          <span className="text-faint">{labels[i]}</span>
          <span className="font-medium tabular-nums">{formatValue ? formatValue(v, i) : v}</span>
        </div>
      </>
    );
  }

  return (
    <div data-slot="sparkline" className={cn("flex min-w-0 flex-col gap-1", className)}>
      <div
        className={cn("relative", interactive && "cursor-crosshair rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/50")}
        {...(interactive && {
          tabIndex: 0,
          onPointerMove: (e) => setActive(indexAt(e)),
          onPointerDown: (e) => setActive(indexAt(e)),
          onPointerLeave: () => setActive(null),
          onFocus: () => setActive(points.length - 1),
          onBlur: () => setActive(null),
          onKeyDown,
        })}
      >
        <svg
          viewBox={`0 0 ${W} ${H}`}
          preserveAspectRatio="none"
          role="img"
          aria-label={label}
          className="block h-[72px] w-full overflow-visible"
        >
          <defs>
            <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0" stopColor="var(--primary)" stopOpacity="0.18" />
              <stop offset="1" stopColor="var(--primary)" stopOpacity="0" />
            </linearGradient>
          </defs>
          <line x1="0" x2={W} y1={H} y2={H} stroke="var(--border)" strokeWidth="1" vectorEffect="non-scaling-stroke" />
          <path d={area} fill={`url(#${gradientId})`} />
          <path
            d={line}
            fill="none"
            stroke="var(--primary)"
            strokeWidth="1.8"
            strokeLinejoin="round"
            vectorEffect="non-scaling-stroke"
          />
          {hovered !== null && (
            <line
              x1={xy[hovered][0]}
              x2={xy[hovered][0]}
              y1="0"
              y2={H}
              stroke="var(--border)"
              strokeWidth="1"
              strokeDasharray="2 2"
              vectorEffect="non-scaling-stroke"
            />
          )}
        </svg>
        {readout}
      </div>
      {axis && (
        <div className="flex justify-between text-label font-normal tracking-normal text-faint">
          <span>{axis[0]}</span>
          <span>{axis[1]}</span>
        </div>
      )}
    </div>
  );
}

// MetricHero is the page hero: label, display-size figure, a delta chip with
// an optional muted aside, and an optional sparkline on the right.
// - amount/tone: passed to <Amount size="display">; or a preformatted `value`.
// - delta/deltaTone: content and tone of the DeltaChip.
// - sub: muted text after the chip ("+$950.00 this year").
// - sparkline: { values, labels, formatValue, label, axis } for <Sparkline>, or a node.
export function MetricHero({ label, amount, value, tone, delta, deltaTone, sub, sparkline, action, className }) {
  const spark = sparkline && !sparkline.$$typeof && Array.isArray(sparkline.values)
    ? <Sparkline {...sparkline} />
    : sparkline;
  return (
    <div
      data-slot="metric-hero"
      className={cn("grid items-end gap-x-12 gap-y-6", spark && "md:grid-cols-[auto_minmax(0,1fr)]", className)}
    >
      <div className="flex min-w-0 flex-col gap-2.5">
        <div className="flex items-center gap-3">
          <Eyebrow>{label}</Eyebrow>
          {action && <div className="ml-auto">{action}</div>}
        </div>
        <div className="min-w-0 text-display font-medium">
          {amount !== undefined ? <Amount value={amount} size="display" tone={tone} /> : value}
        </div>
        {(delta || sub) && (
          <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
            {delta && <DeltaChip tone={deltaTone}>{delta}</DeltaChip>}
            {sub && <span className="text-[12.5px] text-faint">{sub}</span>}
          </div>
        )}
      </div>
      {spark}
    </div>
  );
}
