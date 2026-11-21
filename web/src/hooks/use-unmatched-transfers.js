import { useQuery } from "@tanstack/react-query";
import { ledgerClient } from "@/client.js";
import { queryKeys } from "@/query-keys.js";

// useUnmatchedTransfers loads the transfer sides through the clearing account
// (default equity:transfers) that floatd could not pair with an other side.
export function useUnmatchedTransfers() {
  return useQuery({
    queryKey: queryKeys.unmatchedTransfers(),
    queryFn: () => ledgerClient.findUnmatchedTransfers({}),
  });
}

// useMissingTransferCount is the number of unmatched sides older than the
// match window (probably a missing transaction), or undefined while loading.
export function useMissingTransferCount() {
  const { data } = useUnmatchedTransfers();
  return data?.transfers.filter((t) => t.status === "missing").length;
}
