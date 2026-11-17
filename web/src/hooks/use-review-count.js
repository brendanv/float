import { useQuery } from "@tanstack/react-query";
import { ledgerClient } from "@/client.js";
import { queryKeys } from "@/query-keys.js";

// Reviewed means Cleared (*). Unmarked and Pending transactions need review.
export const UNREVIEWED_QUERY = "not:status:*";

// Search params for /transactions that list the transactions needing review.
export const UNREVIEWED_SEARCH = { search: UNREVIEWED_QUERY };

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
