import { useState } from "react";
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

const DISMISSED_KEY = "float-missing-transfers-dismissed";

// recentMissing is the unmatched sides from the last RECENT_MONTHS that are
// older than the match window (probably a missing transaction).
function recentMissing(data) {
  const cutoff = recentCutoff();
  return data?.transfers.filter((t) => t.status === "missing" && t.transaction.date >= cutoff);
}

function transferKey(t) {
  const tx = t.transaction;
  return [tx.fid, tx.date, tx.description, t.route, t.amount?.quantity, t.amount?.commodity].join("|");
}

function readDismissed() {
  try {
    const keys = JSON.parse(localStorage.getItem(DISMISSED_KEY) || "null");
    return Array.isArray(keys) ? new Set(keys) : null;
  } catch {
    return null;
  }
}

// useMissingTransferNotice returns the number of recentMissing sides
// (undefined while loading) and whether the notice is dismissed: it stays
// hidden until a missing side appears that wasn't listed when it was dismissed.
export function useMissingTransferNotice() {
  const { data } = useUnmatchedTransfers();
  const [dismissed, setDismissed] = useState(readDismissed);
  const missing = recentMissing(data);
  const keys = missing?.map(transferKey) ?? [];
  const hidden = !!dismissed && keys.every((k) => dismissed.has(k));
  const dismiss = () => {
    if (!missing) return;
    try {
      localStorage.setItem(DISMISSED_KEY, JSON.stringify(keys));
    } catch {
    }
    setDismissed(new Set(keys));
  };
  return { count: missing?.length, hidden, dismiss };
}
