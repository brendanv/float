import { useQuery } from "@tanstack/react-query";
import { ledgerClient } from "@/client.js";
import { queryKeys } from "@/query-keys.js";
import { todayStr } from "@/format.js";

export const RECENT_MONTHS = 6;

export function recentCutoff(today = todayStr()) {
  const [y, m, d] = today.split("-").map(Number);
  const month = new Date(y, m - 1 - RECENT_MONTHS, 1);
  const lastDay = new Date(month.getFullYear(), month.getMonth() + 1, 0).getDate();
  return `${month.getFullYear()}-${String(month.getMonth() + 1).padStart(2, "0")}-${String(Math.min(d, lastDay)).padStart(2, "0")}`;
}

// useUnmatchedTransfers loads the transfer sides through the clearing account
// (default equity:transfers) that floatd could not pair with an other side.
export function useUnmatchedTransfers() {
  return useQuery({
    queryKey: queryKeys.unmatchedTransfers(),
    queryFn: () => ledgerClient.findUnmatchedTransfers({}),
  });
}

// useMissingTransferCount is the number of unmatched sides from the last
// RECENT_MONTHS that are older than the match window (probably a missing
// transaction), or undefined while loading.
export function useMissingTransferCount() {
  const { data } = useUnmatchedTransfers();
  const cutoff = recentCutoff();
  return data?.transfers.filter((t) => t.status === "missing" && t.transaction.date >= cutoff).length;
}
