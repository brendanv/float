import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ledgerClient } from "@/client.js";
import { queryKeys } from "@/query-keys.js";

// Reviewed means Cleared (*). Unmarked and Pending transactions need review.
export const UNREVIEWED_QUERY = "not:status:*";

// Search params for /transactions that list the transactions needing review.
export const UNREVIEWED_SEARCH = { search: UNREVIEWED_QUERY };

const DISMISSED_KEY = "float-review-dismissed";

// useReviewCount returns the number of transactions that are not Cleared
// (all dates), or undefined while loading. It asks ListTransactions for one
// row and reads `total`, which counts every match before the limit.
export function useReviewCount() {
  const { data } = useQuery({
    queryKey: queryKeys.unreviewedCount(),
    queryFn: () => ledgerClient.listTransactions({ query: [UNREVIEWED_QUERY], limit: 1, offset: 0 }),
    select: (res) => res.total ?? 0,
  });
  return data;
}

function readDismissed() {
  try {
    const mark = JSON.parse(localStorage.getItem(DISMISSED_KEY) || "null");
    return mark && typeof mark.date === "string" && typeof mark.count === "number" ? mark : null;
  } catch {
    return null;
  }
}

export function useReviewNotice() {
  const count = useReviewCount();
  const [dismissed, setDismissed] = useState(readDismissed);
  const { data: newestDate } = useQuery({
    queryKey: queryKeys.newestUnreviewed(count),
    queryFn: () => ledgerClient.listTransactions({ query: [UNREVIEWED_QUERY], limit: 1, offset: count - 1 }),
    select: (res) => res.transactions?.[0]?.date ?? "",
    enabled: count > 0,
  });
  const loaded = count === 0 || newestDate !== undefined;
  const hidden = !!dismissed && count <= dismissed.count && (!loaded || newestDate <= dismissed.date);
  const dismiss = () => {
    if (!loaded) return;
    const mark = { date: newestDate ?? "", count };
    try {
      localStorage.setItem(DISMISSED_KEY, JSON.stringify(mark));
    } catch {
    }
    setDismissed(mark);
  };
  return { count, hidden, dismiss };
}
