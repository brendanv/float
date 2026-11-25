// Undoable changes. floatd sets the "float-snapshot" response trailer on
// mutations that can be undone (see UndoSnapshotRequest in ledger.proto); the
// client interceptor in client.js reports them here and UndoToaster offers
// to undo the latest one.

export const SNAPSHOT_TRAILER = "float-snapshot";

const listeners = new Set();

// onUndoable subscribes fn({ method, hash, request }) and returns an
// unsubscribe function. request is the unary request message (undefined for
// streaming calls).
export function onUndoable(fn) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

export function reportUndoable(change) {
  for (const fn of listeners) fn(change);
}

function count(n, one, many) {
  return n === 1 ? one : `${n} ${many}`;
}

// undoLabel describes a change in its toast, e.g. "Transaction deleted".
export function undoLabel({ method, request }) {
  switch (method) {
    case "DeleteTransaction":
      return "Transaction deleted";
    case "UpdateTransaction":
      return "Transaction saved";
    case "BulkEditTransactions":
      return count(request?.fids?.length ?? 0, "Transaction updated", "transactions updated");
    case "BulkDeleteTransactions":
      return "Transactions deleted";
    case "ApplyRules":
      return "Rules applied";
    case "ImportTransactions":
    case "ImportStructuredTransactions":
      return "Transactions imported";
    case "DeleteRule":
      return "Rule deleted";
    case "DeleteTemplate":
      return "Template deleted";
    case "DeletePrice":
      return "Price deleted";
    case "DeleteAccountDeclaration":
      return "Account declaration deleted";
    case "SetAccountClosed":
      return request?.closed ? "Account closed" : "Account reopened";
    case "RestoreSnapshot":
      return "Snapshot restored";
    default:
      return "Change saved";
  }
}
