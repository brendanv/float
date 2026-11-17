import { useMemo } from "react";
import { Amount } from "@/components/amount.jsx";
import { DeltaChip, Section } from "@/components/page.jsx";
import { formatPct, gainTone, periodReturns } from "./performance.js";

// MultiPeriodReturns lists the cash-flow adjusted gain per period: period,
// signed amount and a % chip.
export function MultiPeriodReturns({ snapshots }) {
  const items = useMemo(() => periodReturns(snapshots), [snapshots]);
  if (!items.length) return null;

  return (
    <Section title="Cash-flow adjusted returns">
      <p className="text-[12.5px] text-faint">
        Gain excluding net deposits and withdrawals.
      </p>
      <ul data-slot="period-returns" className="flex flex-col">
        {items.map((item) => {
          const tone = gainTone(item.investmentGain);
          return (
            <li
              key={item.label}
              className="flex items-center gap-3 border-b border-border py-1.5 last:border-b-0"
            >
              <span className="w-10 text-muted-foreground" title={item.name}>
                {item.label}
              </span>
              <Amount
                value={item.investmentGain.toFixed(2)}
                commodity={item.currency}
                signed
                tone={tone}
                className="ml-auto"
              />
              <DeltaChip tone={tone} className="min-w-14 justify-center">
                {formatPct(item.returnPct)}
              </DeltaChip>
            </li>
          );
        })}
      </ul>
    </Section>
  );
}
