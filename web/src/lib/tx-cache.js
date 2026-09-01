// Cache patching for transaction mutations.
//
// The transactions page fetches whole result sets and pages through them
// client-side, so invalidating on every single-row edit means re-downloading
// everything before the row visibly changes. The mutation RPCs already return
// the updated Transaction, so we splice it into the cached responses for
// instant feedback and let revalidation reconcile in the background.
//
// Filter state is embedded in the query key, so there are many live cache
// entries per prefix; setQueriesData patches all of them at once.

import { queryKeys } from "../query-keys.js";

const TX_PREFIX = queryKeys.transactionsPrefix();
const REGISTER_PREFIX = queryKeys.accountRegisterPrefix();

// registerFieldsFrom copies the fields an AccountRegisterRow shares with a
// Transaction. Register-specific fields (otherAccounts, change, runningTotal)
// are deliberately left alone: they depend on the focused account and on every
// preceding row, so they can only be recomputed server-side. Revalidation
// supplies the authoritative values.
function registerFieldsFrom(tx) {
  return {
    date: tx.date,
    description: tx.description,
    payee: tx.payee,
    note: tx.note,
    status: tx.status,
    tags: tx.tags,
  };
}

function mapMatching(list, fid, fn) {
  if (!list) return { list, changed: false };
  let changed = false;
  const next = list.map((item) => {
    if (item.fid !== fid) return item;
    changed = true;
    return fn(item);
  });
  return { list: next, changed };
}

// patchTransaction replaces the row for tx.fid in every cached transaction
// list and account register. Returns without touching the cache when tx has
// no fid (nothing to match on).
export function patchTransaction(queryClient, tx) {
  if (!tx?.fid) return;

  queryClient.setQueriesData({ queryKey: TX_PREFIX }, (data) => {
    if (!data) return data;
    const { list, changed } = mapMatching(data.transactions, tx.fid, () => tx);
    return changed ? { ...data, transactions: list } : data;
  });

  queryClient.setQueriesData({ queryKey: REGISTER_PREFIX }, (data) => {
    if (!data) return data;
    const { list, changed } = mapMatching(data.rows, tx.fid, (row) => ({
      ...row,
      ...registerFieldsFrom(tx),
    }));
    return changed ? { ...data, rows: list } : data;
  });
}

// removeTransaction drops the row for fid from every cached transaction list
// and account register, keeping the reported total in step.
export function removeTransaction(queryClient, fid) {
  if (!fid) return;

  const drop = (key, field) => {
    queryClient.setQueriesData({ queryKey: key }, (data) => {
      if (!data?.[field]) return data;
      const next = data[field].filter((item) => item.fid !== fid);
      if (next.length === data[field].length) return data;
      return { ...data, [field]: next, total: Math.max(0, (data.total ?? next.length) - 1) };
    });
  };

  drop(TX_PREFIX, "transactions");
  drop(REGISTER_PREFIX, "rows");
}

// revalidateTransactions refetches transaction lists and account registers in
// the background. The queries already hold data, so isLoading stays false and
// the table never blanks while the refetch is in flight.
//
// Always pair this with a patch rather than reasoning about whether an edit can
// move a row out of the active filter — it can (a status edit under a status:
// filter, a description edit under a search filter). The patch buys the instant
// feedback; this guarantees the list settles to what the server actually has.
export function revalidateTransactions(queryClient) {
  queryClient.invalidateQueries({ queryKey: TX_PREFIX });
  queryClient.invalidateQueries({ queryKey: REGISTER_PREFIX });
}
