import { useCallback, useMemo, useState } from "react";
import { ledgerClient } from "@/client.js";
import { ErrorBanner } from "@/components/error-banner.jsx";
import { CandidateTable } from "@/components/import/candidate-table.jsx";
import { Button } from "@/components/ui/button";
import { Notice } from "@/components/ui/alert";

// Fetch / refresh / import state for the Connections page. The hooks keep
// the RPC flow the old panels had; the page decides where the buttons and
// results render.

function plural(n, one, many = `${one}s`) {
  return `${n} ${n === 1 ? one : many}`;
}

function useSourceSelection() {
  // selectedIds is a Set of sourceId strings (unique Stripe transaction IDs).
  const [selectedIds, setSelectedIds] = useState(new Set());
  const toggle = useCallback((sourceId) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(sourceId)) next.delete(sourceId);
      else next.add(sourceId);
      return next;
    });
  }, []);
  return [selectedIds, setSelectedIds, toggle];
}

function autoSelect(candidates) {
  const ids = new Set();
  candidates.forEach((c) => {
    if (c.sourceId) ids.add(c.sourceId);
  });
  return ids;
}

// useAccountFetch drives one linked account: Refresh & Fetch, Fetch, Import.
export function useAccountFetch(account, onImported) {
  const [candidates, setCandidates] = useState(null);
  const [fetching, setFetching] = useState(false);
  const [fetchError, setFetchError] = useState(null);
  const [selectedIds, setSelectedIds, toggleCandidate] = useSourceSelection();
  const [importing, setImporting] = useState(false);
  const [importProgress, setImportProgress] = useState(null);
  const [importError, setImportError] = useState(null);
  const [importResult, setImportResult] = useState(null);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshStatus, setRefreshStatus] = useState(null);
  const [refreshError, setRefreshError] = useState(null);
  const [refreshThrottledAt, setRefreshThrottledAt] = useState(null);

  async function handleFetch() {
    setFetching(true);
    setFetchError(null);
    setImportResult(null);
    setCandidates(null);
    try {
      const res = await ledgerClient.fetchStripeTransactions({
        stripeAccountId: account.stripeAccountId,
      });
      setCandidates(res.candidates);
      setSelectedIds(autoSelect(res.candidates));
    } catch (err) {
      setFetchError(err);
    } finally {
      setFetching(false);
    }
  }

  async function handleRefresh() {
    setRefreshing(true);
    setRefreshError(null);
    setRefreshThrottledAt(null);
    setRefreshStatus({ status: "starting", elapsedSeconds: 0, attempt: 0 });
    let succeeded = false;
    try {
      for await (const res of ledgerClient.refreshStripeAccount(
        { stripeAccountId: account.stripeAccountId },
        { timeoutMs: 6 * 60 * 1000 },
      )) {
        if (res.payload.case === "progress") {
          const p = res.payload.value;
          setRefreshStatus({
            status: p.status,
            elapsedSeconds: Number(p.elapsedSeconds ?? 0),
            attempt: p.attempt ?? 0,
            message: p.message,
          });
        } else if (res.payload.case === "result") {
          const r = res.payload.value;
          succeeded = r.succeeded;
          if (r.throttled) {
            setRefreshThrottledAt(Number(r.nextRefreshAvailableAt ?? 0));
          }
          if (!r.succeeded) {
            setRefreshError(new Error(r.errorMessage || "refresh failed"));
          }
        }
      }
    } catch (err) {
      setRefreshError(err);
    } finally {
      setRefreshing(false);
      setRefreshStatus(null);
    }
    if (succeeded) {
      await handleFetch();
    }
  }

  function toggleAll() {
    if (!candidates) return;
    const allIds = candidates.filter((c) => c.sourceId).map((c) => c.sourceId);
    if (selectedIds.size === allIds.length) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(allIds));
    }
  }

  async function handleImport() {
    if (selectedIds.size === 0) return;
    setImportError(null);
    setImporting(true);
    setImportProgress({ imported: 0, total: selectedIds.size });
    try {
      for await (const res of ledgerClient.importStripeTransactions({
        stripeAccountId: account.stripeAccountId,
        stripeTransactionIds: Array.from(selectedIds),
      })) {
        if (res.payload.case === "progress") {
          setImportProgress({
            imported: res.payload.value.imported,
            total: res.payload.value.total,
          });
        } else if (res.payload.case === "result") {
          setImportResult(res.payload.value);
          setCandidates(null);
          if (onImported) onImported();
        }
      }
    } catch (err) {
      setImportError(err);
    } finally {
      setImporting(false);
      setImportProgress(null);
    }
  }

  return {
    importFrom: account.importFrom,
    candidates,
    fetching,
    fetchError,
    selectedIds,
    toggleCandidate,
    toggleAll,
    importing,
    importProgress,
    importError,
    importResult,
    refreshing,
    refreshStatus,
    refreshError,
    refreshThrottledAt,
    handleFetch,
    handleRefresh,
    handleImport,
  };
}

// useFetchAll drives Refresh & Fetch All / Fetch All / Import across every
// configured account. refreshAccountStatus maps stripeAccountId to
// {status, elapsedSeconds, attempt, message, succeeded?, throttled?, error?}.
export function useFetchAll(configuredAccounts, onImported) {
  const [accountCandidates, setAccountCandidates] = useState(null);
  const [fetching, setFetching] = useState(false);
  const [fetchError, setFetchError] = useState(null);
  const [selectedIds, setSelectedIds, toggleCandidate] = useSourceSelection();
  const [importing, setImporting] = useState(false);
  const [importProgress, setImportProgress] = useState(null);
  const [importError, setImportError] = useState(null);
  const [importResult, setImportResult] = useState(null);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshAccountStatus, setRefreshAccountStatus] = useState(new Map());
  const [refreshError, setRefreshError] = useState(null);

  async function handleFetchAll() {
    setFetching(true);
    setFetchError(null);
    setImportResult(null);
    setAccountCandidates(null);
    try {
      const res = await ledgerClient.fetchAllStripeTransactions({});
      setAccountCandidates(res.accountCandidates);
      setSelectedIds(autoSelect(res.accountCandidates.flatMap((ac) => ac.candidates)));
    } catch (err) {
      setFetchError(err);
    } finally {
      setFetching(false);
    }
  }

  async function handleRefreshAll() {
    setRefreshing(true);
    setRefreshError(null);
    setRefreshAccountStatus(new Map());
    let anySucceeded = false;
    try {
      for await (const res of ledgerClient.refreshAllStripeAccounts(
        {},
        { timeoutMs: 6 * 60 * 1000 },
      )) {
        if (res.payload.case === "progress") {
          const p = res.payload.value;
          setRefreshAccountStatus((prev) => {
            const next = new Map(prev);
            const existing = next.get(p.stripeAccountId) ?? {};
            next.set(p.stripeAccountId, {
              ...existing,
              status: p.status,
              elapsedSeconds: Number(p.elapsedSeconds ?? 0),
              attempt: p.attempt ?? 0,
              message: p.message,
            });
            return next;
          });
        } else if (res.payload.case === "result") {
          const r = res.payload.value;
          if (r.succeeded) anySucceeded = true;
          setRefreshAccountStatus((prev) => {
            const next = new Map(prev);
            const existing = next.get(r.stripeAccountId) ?? {};
            next.set(r.stripeAccountId, {
              ...existing,
              succeeded: r.succeeded,
              throttled: r.throttled || false,
              nextRefreshAvailableAt: Number(r.nextRefreshAvailableAt ?? 0),
              error: r.succeeded ? null : r.errorMessage,
            });
            return next;
          });
        }
      }
    } catch (err) {
      setRefreshError(err);
    } finally {
      setRefreshing(false);
    }
    if (anySucceeded) {
      await handleFetchAll();
    }
  }

  const allCandidates = useMemo(() => {
    if (!accountCandidates) return [];
    return accountCandidates.flatMap((ac) =>
      ac.candidates.map((c) => ({
        ...c,
        _accountId: ac.account?.stripeAccountId,
        _accountName: ac.account?.displayName || ac.account?.stripeAccountId,
      }))
    );
  }, [accountCandidates]);

  function toggleAll() {
    const allIds = allCandidates.filter((c) => c.sourceId).map((c) => c.sourceId);
    if (selectedIds.size === allIds.length) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(allIds));
    }
  }

  async function handleImport() {
    if (selectedIds.size === 0) return;
    setImportError(null);
    setImporting(true);
    setImportProgress({ imported: 0, total: selectedIds.size });

    const selectionsByAccount = new Map();
    allCandidates.forEach((c) => {
      if (selectedIds.has(c.sourceId)) {
        if (!selectionsByAccount.has(c._accountId)) {
          selectionsByAccount.set(c._accountId, []);
        }
        selectionsByAccount.get(c._accountId).push(c.sourceId);
      }
    });
    const selections = Array.from(selectionsByAccount.entries()).map(
      ([stripeAccountId, stripeTransactionIds]) => ({ stripeAccountId, stripeTransactionIds })
    );

    try {
      for await (const res of ledgerClient.importAllStripeTransactions({ selections })) {
        if (res.payload.case === "progress") {
          setImportProgress({
            imported: res.payload.value.imported,
            total: res.payload.value.total,
          });
        } else if (res.payload.case === "result") {
          setImportResult(res.payload.value);
          setAccountCandidates(null);
          if (onImported) onImported();
        }
      }
    } catch (err) {
      setImportError(err);
    } finally {
      setImporting(false);
      setImportProgress(null);
    }
  }

  return {
    accountCount: configuredAccounts.length,
    accountCandidates,
    candidates: accountCandidates ? allCandidates : null,
    fetching,
    fetchError,
    selectedIds,
    toggleCandidate,
    toggleAll,
    importing,
    importProgress,
    importError,
    importResult,
    refreshing,
    refreshAccountStatus,
    refreshError,
    handleFetchAll,
    handleRefreshAll,
    handleImport,
  };
}

// refreshLine describes one account's Refresh All progress, with a tone
// ("ink" | "warning" | "below").
export function refreshLine(st) {
  if (st.throttled) {
    const next = st.nextRefreshAvailableAt
      ? new Date(st.nextRefreshAvailableAt * 1000).toLocaleString()
      : "later";
    return { text: `Throttled, next refresh at ${next}`, tone: "warning" };
  }
  if (st.succeeded === true) return { text: "Refreshed", tone: "ink" };
  if (st.succeeded === false) return { text: `Refresh failed: ${st.error ?? "unknown error"}`, tone: "below" };
  if (st.status === "polling") return { text: `Refreshing (${st.elapsedSeconds}s, attempt ${st.attempt})`, tone: "ink" };
  return { text: st.message || st.status || "Starting", tone: "ink" };
}

// FetchResults shows a fetch's outcome: errors and notices, then the fetched
// transactions with Select all / Import N selected. `fetch` is the object
// from useAccountFetch or useFetchAll.
export function FetchResults({ fetch, showAccount = false, resultText, className }) {
  const {
    candidates,
    fetchError,
    selectedIds,
    toggleCandidate,
    toggleAll,
    importing,
    importProgress,
    importError,
    importResult,
    refreshError,
    refreshThrottledAt,
  } = fetch;

  const isSelected = useCallback((row) => selectedIds.has(row.sourceId), [selectedIds]);
  const isDisabled = useCallback((row) => !row.sourceId, []);
  const onToggle = useCallback((row) => toggleCandidate(row.sourceId), [toggleCandidate]);

  const hasContent = refreshError || refreshThrottledAt > 0 || fetchError || importResult || importError || candidates;
  if (!hasContent) return null;

  const total = candidates ? candidates.length : 0;

  return (
    <div data-slot="fetch-results" className={className}>
      <div className="flex flex-col gap-3">
        {refreshError && <ErrorBanner error={refreshError} />}
        {refreshThrottledAt > 0 && (
          <Notice variant="warning">
            Stripe throttled this refresh. Next refresh available at{" "}
            {new Date(refreshThrottledAt * 1000).toLocaleString()}. Showing existing transactions.
          </Notice>
        )}
        {fetchError && <ErrorBanner error={fetchError} />}
        {importResult && (
          <Notice variant="info">
            Imported {plural(importResult.importedCount, "transaction")}{resultText ? ` ${resultText}` : ""}.
          </Notice>
        )}
        {importError && <ErrorBanner error={importError} />}
        {candidates && (
          <div className="flex flex-col gap-1">
            <div className="flex min-h-8 flex-wrap items-center gap-2">
              <span className="text-control text-muted-foreground">
                {plural(total, "new transaction")}
                {fetch.importFrom && ` since ${fetch.importFrom}`}
              </span>
              {total > 0 && (
                <div className="ml-auto flex gap-2">
                  <Button variant="ghost" size="sm" onClick={toggleAll}>
                    {selectedIds.size === total ? "Deselect all" : "Select all"}
                  </Button>
                  <Button
                    size="sm"
                    onClick={fetch.handleImport}
                    disabled={selectedIds.size === 0}
                    isLoading={importing}
                    loadingText={
                      importProgress && importProgress.total > 0
                        ? `Importing ${importProgress.imported} of ${importProgress.total}…`
                        : "Importing…"
                    }
                  >
                    Import {selectedIds.size} selected
                  </Button>
                </div>
              )}
            </div>
            {total > 0 && (
              <CandidateTable
                rows={candidates}
                isSelected={isSelected}
                isDisabled={isDisabled}
                onToggle={onToggle}
                showAccount={showAccount}
              />
            )}
          </div>
        )}
      </div>
    </div>
  );
}
