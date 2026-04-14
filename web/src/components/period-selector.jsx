import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { monthName } from "../format.js";

export function PeriodSelector({ year, month, onChange }) {
  function prev() {
    if (month === 1) {
      onChange(year - 1, 12);
    } else {
      onChange(year, month - 1);
    }
  }

  function next() {
    if (month === 12) {
      onChange(year + 1, 1);
    } else {
      onChange(year, month + 1);
    }
  }

  return (
    <div className="mb-4 flex items-center gap-1">
      <Button variant="ghost" size="icon-sm" onClick={prev} aria-label="Previous month">
        <ChevronLeft />
      </Button>
      <span className="min-w-32 text-center text-sm font-semibold sm:min-w-40">
        {monthName(month)} {year}
      </span>
      <Button variant="ghost" size="icon-sm" onClick={next} aria-label="Next month">
        <ChevronRight />
      </Button>
    </div>
  );
}
