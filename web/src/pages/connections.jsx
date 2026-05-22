import { useState, useMemo } from "react";
import { useQuery, useQueryClient, useMutation } from "@tanstack/react-query";
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  flexRender,
} from "@tanstack/react-table";
import {
  Link2,
  Link2Off,
  CircleCheck,
  Clock,
  Loader2,
  RefreshCw,
  Tag,
  ArrowUp,
  ArrowDown,
  ArrowUpDown,
  Calendar,
} from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Form, FormField } from "@/components/ui/form";
import { Combobox } from "@/components/ui/combobox";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

function LinkMappingDialog({ open, fcAccounts, accountDeclarations, onComplete, onClose }) {
  const [mappings, setMappings] = useState(() =>
    Object.fromEntries(
      fcAccounts.map((a) => [
        a.id,
        { hledgerAccount: "", displayName: a.display_name ?? a.displayName ?? a.id },
      ])
    )
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  function setMapping(fcId, field, value) {
    setMappings((prev) => ({ ...prev, [fcId]: { ...prev[fcId], [field]: value } }));
  }

  const allMapped = fcAccounts.every((a) => mappings[a.id]?.hledgerAccount);

  async function handleSubmit(e) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      await onComplete(mappings);
    } catch (err) {
      setError(err);
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Map Linked Accounts</DialogTitle>
        </DialogHeader>
        <Form onSubmit={handleSubmit}>
          {error && <ErrorBanner error={error} />}
          <p className="text-xs text-muted-foreground">
            {fcAccounts.length === 1
              ? "1 account was linked. Choose the hledger account for it."
              : `${fcAccounts.length} accounts were linked. Choose the hledger account for each.`}
          </p>
          {fcAccounts.map((a) => (
            <div key={a.id} className="flex flex-col gap-3 rounded-md border p-3">
              <span className="text-[10px] text-muted-foreground font-mono">{a.id}</span>
              <FormField label="Display Name">
                <Input
                  value={mappings[a.id]?.displayName ?? ""}
                  onChange={(e) => setMapping(a.id, "displayName", e.target.value)}
                  placeholder={a.display_name ?? a.displayName ?? a.id}
                />
              </FormField>
              <FormField label="hledger Account">
                <Combobox
                  value={mappings[a.id]?.hledgerAccount || ""}
                  onChange={(v) => setMapping(a.id, "hledgerAccount", v)}
                  options={(accountDeclarations ?? []).map((d) => d.name)}
                  placeholder="Select account…"
                  searchPlaceholder="Search accounts…"
                  emptyMessage="No matching account."
                />
              </FormField>
            </div>
          ))}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
            <Button type="submit" disabled={saving || !allMapped}>
              {saving && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
              {saving ? "Saving…" : "Save"}
            </Button>
          </DialogFooter>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

function CandidatesTable({ candidates, selectedIds, onToggle, showAccount }) {
  const [sorting, setSorting] = useState([]);

  const columns = useMemo(
    () => [
      {
        id: "select",
        header: () => null,
        cell: ({ row }) => (
          <Checkbox
            checked={selectedIds.has(row.original.sourceId)}
            disabled={row.original.isDuplicate || !row.original.sourceId}
            onCheckedChange={() => onToggle(row.original.sourceId)}
          />
        ),
        enableSorting: false,
      },
      {
        id: "status",
        header: "Status",
        accessorFn: (row) => row.isDuplicate,
        cell: ({ getValue }) => (
          <Badge variant={getValue() ? "secondary" : "default"}>
            {getValue() ? "DUP" : "NEW"}
          </Badge>
        ),
        enableSorting: false,
      },
      ...(showAccount
        ? [
            {
              id: "account",
              header: "Account",
              accessorFn: (row) => row._accountName ?? "",
              cell: ({ getValue }) => (
                <span className="text-xs text-muted-foreground whitespace-nowrap">{getValue()}</span>
              ),
            },
          ]
        : []),
      {
        id: "date",
        header: "Date",
        accessorFn: (row) => row.transaction?.date ?? "",
        cell: ({ getValue }) => <span className="whitespace-nowrap">{getValue()}</span>,
      },
      {
        id: "description",
        header: "Description",
        accessorFn: (row) => row.transaction?.description ?? "",
      },
      {
        id: "postings",
        header: "Postings",
        cell: ({ row }) => (
          <div className="text-xs">
            {(row.original.transaction?.postings ?? []).map((p, j) => (
              <div key={j}>
                {p.account}
                {p.amounts?.[0] && (
                  <span className="ml-1 text-muted-foreground">
                    {p.amounts[0].commodity}{p.amounts[0].quantity}
                  </span>
                )}
              </div>
            ))}
          </div>
        ),
        enableSorting: false,
      },
      {
        id: "matched",
        header: "Rule",
        accessorFn: (row) => row.matchedRuleId ?? "",
        cell: ({ getValue }) =>
          getValue() ? (
            <Tag className="size-3.5 text-primary" title="Matched a rule" />
          ) : (
            <span className="text-muted-foreground">—</span>
          ),
        enableSorting: false,
      },
    ],
    [selectedIds, onToggle, showAccount]
  );

  const table = useReactTable({
    data: candidates,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  return (
    <Table>
      <TableHeader>
        {table.getHeaderGroups().map((headerGroup) => (
          <TableRow key={headerGroup.id}>
            {headerGroup.headers.map((header) => {
              const canSort = header.column.getCanSort();
              const sorted = header.column.getIsSorted();
              return (
                <TableHead key={header.id}>
                  {canSort ? (
                    <button
                      type="button"
                      className="flex items-center gap-1 hover:text-foreground"
                      onClick={header.column.getToggleSortingHandler()}
                    >
                      {flexRender(header.column.columnDef.header, header.getContext())}
                      {sorted === "asc" ? (
                        <ArrowUp className="size-3" />
                      ) : sorted === "desc" ? (
                        <ArrowDown className="size-3" />
                      ) : (
                        <ArrowUpDown className="size-3 opacity-40" />
                      )}
                    </button>
                  ) : (
                    flexRender(header.column.columnDef.header, header.getContext())
                  )}
                </TableHead>
              );
            })}
          </TableRow>
        ))}
      </TableHeader>
      <TableBody>
        {table.getRowModel().rows.map((row) => (
          <TableRow
            key={row.id}
            className={cn(row.original.isDuplicate && "opacity-50")}
          >
            {row.getVisibleCells().map((cell) => (
              <TableCell key={cell.id}>
                {flexRender(cell.column.columnDef.cell, cell.getContext())}
              </TableCell>
            ))}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function FetchAllPanel({ configuredAccounts, onImported }) {
  const [accountCandidates, setAccountCandidates] = useState(null);
  const [fetching, setFetching] = useState(false);
  const [fetchError, setFetchError] = useState(null);
  // selectedIds is a Set of sourceId strings (unique Stripe transaction IDs across all accounts)
  const [selectedIds, setSelectedIds] = useState(new Set());
  const [importing, setImporting] = useState(false);
  const [importProgress, setImportProgress] = useState(null);
  const [importError, setImportError] = useState(null);
  const [importResult, setImportResult] = useState(null);
  const [refreshing, setRefreshing] = useState(false);
  // refreshAccountStatus: Map<stripeAccountId, {status, elapsedSeconds, attempt, message, succeeded?, error?}>
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
      const autoSelected = new Set();
      res.accountCandidates.forEach((ac) => {
        ac.candidates.forEach((c) => {
          if (!c.isDuplicate && c.sourceId) autoSelected.add(c.sourceId);
        });
      });
      setSelectedIds(autoSelected);
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

  const accountNameById = useMemo(() => {
    const m = new Map();
    configuredAccounts.forEach((a) => {
      m.set(a.stripeAccountId, a.displayName || a.stripeAccountId);
    });
    return m;
  }, [configuredAccounts]);

  function toggleCandidate(sourceId) {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(sourceId)) next.delete(sourceId);
      else next.add(sourceId);
      return next;
    });
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

  const newCount = allCandidates.filter((c) => !c.isDuplicate).length;

  function toggleAll() {
    const allNewIds = allCandidates
      .filter((c) => !c.isDuplicate && c.sourceId)
      .map((c) => c.sourceId);
    if (selectedIds.size === allNewIds.length) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(allNewIds));
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

  const totalCount = allCandidates.length;

  const refreshRows = Array.from(refreshAccountStatus.entries());

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-4">
          <div>
            <CardTitle className="text-base">Fetch All Accounts</CardTitle>
            <CardDescription>
              Pull new transactions from all {configuredAccounts.length} linked account{configuredAccounts.length !== 1 ? "s" : ""} at once.
            </CardDescription>
          </div>
          <div className="flex gap-2">
            <Button size="sm" onClick={handleRefreshAll} disabled={refreshing || fetching}>
              {refreshing ? (
                <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />
              ) : (
                <RefreshCw data-icon="inline-start" className="size-3.5" />
              )}
              {refreshing ? "Refreshing…" : "Refresh & Fetch All"}
            </Button>
            <Button size="sm" variant="secondary" onClick={handleFetchAll} disabled={fetching || refreshing}>
              {fetching && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
              {fetching ? "Fetching…" : "Fetch All"}
            </Button>
          </div>
        </div>
      </CardHeader>
      {(refreshError || refreshRows.length > 0 || fetchError || importResult || importError || accountCandidates) && (
        <CardContent className="flex flex-col gap-3">
          {refreshError && <ErrorBanner error={refreshError} />}
          {refreshRows.length > 0 && (
            <div className="flex flex-col gap-1 rounded-md border bg-muted/30 p-3 text-xs">
              {refreshRows.map(([accountId, st]) => {
                const name = accountNameById.get(accountId) ?? accountId;
                let line;
                if (st.throttled) {
                  const next = st.nextRefreshAvailableAt
                    ? new Date(st.nextRefreshAvailableAt * 1000).toLocaleString()
                    : "later";
                  line = `throttled — next refresh at ${next}`;
                } else if (st.succeeded === true) {
                  line = "succeeded";
                } else if (st.succeeded === false) {
                  line = `failed: ${st.error ?? "unknown error"}`;
                } else if (st.status === "polling") {
                  line = `polling (${st.elapsedSeconds}s, attempt ${st.attempt})`;
                } else {
                  line = st.message || st.status || "starting";
                }
                const tone = st.succeeded === false
                  ? "text-destructive"
                  : st.throttled
                    ? "text-amber-600 dark:text-amber-500"
                    : "text-muted-foreground";
                return (
                  <div key={accountId} className="flex justify-between gap-3 font-mono">
                    <span>{name}</span>
                    <span className={tone}>{line}</span>
                  </div>
                );
              })}
            </div>
          )}
          {fetchError && <ErrorBanner error={fetchError} />}
          {importResult && (
            <Alert>
              <CircleCheck className="size-4 text-success" />
              <AlertDescription>
                Imported {importResult.importedCount} transaction(s) across {configuredAccounts.length} account{configuredAccounts.length !== 1 ? "s" : ""}.
              </AlertDescription>
            </Alert>
          )}
          {importError && <ErrorBanner error={importError} />}
          {accountCandidates && (
            <div className="flex flex-col gap-2">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="text-sm text-muted-foreground">
                  {totalCount} transaction(s), {newCount} new
                </span>
                <div className="flex gap-2">
                  <Button variant="ghost" size="sm" onClick={toggleAll}>
                    {selectedIds.size === newCount ? "Deselect All" : "Select All New"}
                  </Button>
                  <Button
                    size="sm"
                    onClick={handleImport}
                    disabled={importing || selectedIds.size === 0}
                  >
                    {importing && (
                      <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />
                    )}
                    {importing
                      ? importProgress && importProgress.total > 0
                        ? `Importing ${importProgress.imported} of ${importProgress.total}…`
                        : "Importing…"
                      : `Import ${selectedIds.size} Selected`}
                  </Button>
                </div>
              </div>
              {allCandidates.length === 0 ? (
                <p className="text-sm text-muted-foreground py-2">No new transactions found.</p>
              ) : (
                <CandidatesTable
                  candidates={allCandidates}
                  selectedIds={selectedIds}
                  onToggle={toggleCandidate}
                  showAccount={true}
                />
              )}
            </div>
          )}
        </CardContent>
      )}
    </Card>
  );
}

function AccountFetchPanel({ account, onImported }) {
  const [candidates, setCandidates] = useState(null);
  const [fetching, setFetching] = useState(false);
  const [fetchError, setFetchError] = useState(null);
  const [selectedIds, setSelectedIds] = useState(new Set());
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
      const autoSelected = new Set();
      res.candidates.forEach((c) => {
        if (!c.isDuplicate && c.sourceId) autoSelected.add(c.sourceId);
      });
      setSelectedIds(autoSelected);
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

  function toggleCandidate(sourceId) {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(sourceId)) next.delete(sourceId);
      else next.add(sourceId);
      return next;
    });
  }

  function toggleAll() {
    if (!candidates) return;
    const allNewIds = candidates
      .filter((c) => !c.isDuplicate && c.sourceId)
      .map((c) => c.sourceId);
    if (selectedIds.size === allNewIds.length) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(allNewIds));
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

  const newCount = candidates ? candidates.filter((c) => !c.isDuplicate).length : 0;

  return (
    <div className="flex flex-col gap-3 pt-3 border-t">
      <div className="flex flex-wrap gap-2">
        <Button size="sm" onClick={handleRefresh} disabled={refreshing || fetching}>
          {refreshing ? (
            <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />
          ) : (
            <RefreshCw data-icon="inline-start" className="size-3.5" />
          )}
          {refreshing
            ? refreshStatus
              ? `Refreshing… (${refreshStatus.elapsedSeconds}s, attempt ${refreshStatus.attempt})`
              : "Refreshing…"
            : "Refresh & Fetch"}
        </Button>
        <Button size="sm" variant="secondary" onClick={handleFetch} disabled={fetching || refreshing}>
          {fetching && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
          {fetching ? "Fetching…" : "Fetch Transactions"}
        </Button>
      </div>
      {refreshError && <ErrorBanner error={refreshError} />}
      {refreshThrottledAt > 0 && (
        <Alert>
          <Clock className="size-4" />
          <AlertDescription>
            Stripe throttled this refresh. Next refresh available at{" "}
            {new Date(refreshThrottledAt * 1000).toLocaleString()}. Showing existing transactions.
          </AlertDescription>
        </Alert>
      )}
      {fetchError && <ErrorBanner error={fetchError} />}
      {importResult && (
        <Alert>
          <CircleCheck className="size-4 text-success" />
          <AlertDescription>
            Imported {importResult.importedCount} transaction(s).
          </AlertDescription>
        </Alert>
      )}
      {importError && <ErrorBanner error={importError} />}
      {candidates && (
        <div className="flex flex-col gap-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-sm text-muted-foreground">
              {candidates.length} transaction(s), {newCount} new
            </span>
            <div className="flex gap-2">
              <Button variant="ghost" size="sm" onClick={toggleAll}>
                {selectedIds.size === newCount ? "Deselect All" : "Select All New"}
              </Button>
              <Button
                size="sm"
                onClick={handleImport}
                disabled={importing || selectedIds.size === 0}
              >
                {importing && (
                  <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />
                )}
                {importing
                  ? importProgress && importProgress.total > 0
                    ? `Importing ${importProgress.imported} of ${importProgress.total}…`
                    : "Importing…"
                  : `Import ${selectedIds.size} Selected`}
              </Button>
            </div>
          </div>
          <CandidatesTable
            candidates={candidates}
            selectedIds={selectedIds}
            onToggle={toggleCandidate}
            showAccount={false}
          />
        </div>
      )}
    </div>
  );
}

function UpdateFetchDateDialog({ account, open, onClose, onUpdated }) {
  const [dateValue, setDateValue] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  function handleOpenChange(v) {
    if (!v) {
      setDateValue("");
      setError(null);
      onClose();
    }
  }

  async function handleSave(e) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      // Convert local date to RFC3339 UTC midnight
      const lastFetchedAt = dateValue ? new Date(dateValue + "T00:00:00").toISOString() : "";
      await ledgerClient.updateStripeAccountLastFetchedAt({
        stripeAccountId: account.stripeAccountId,
        lastFetchedAt,
      });
      onUpdated();
      onClose();
    } catch (err) {
      setError(err);
    } finally {
      setSaving(false);
    }
  }

  async function handleClear() {
    setSaving(true);
    setError(null);
    try {
      await ledgerClient.updateStripeAccountLastFetchedAt({
        stripeAccountId: account.stripeAccountId,
        lastFetchedAt: "",
      });
      onUpdated();
      onClose();
    } catch (err) {
      setError(err);
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Update Fetch Date</DialogTitle>
        </DialogHeader>
        <Form onSubmit={handleSave}>
          {error && <ErrorBanner error={error} />}
          <p className="text-sm text-muted-foreground">
            Set or clear the last fetched date for <strong>{account.displayName}</strong>.
            The next fetch will retrieve transactions starting from this date.
            Clear it to fetch all available history.
          </p>
          <div className="flex flex-col gap-1.5">
            <label className="text-sm font-medium">Fetch from date</label>
            <Input
              type="date"
              value={dateValue}
              onChange={(e) => setDateValue(e.target.value)}
              placeholder="Leave empty to fetch all history"
            />
          </div>
          <DialogFooter className="flex-col gap-2 sm:flex-row">
            <Button
              type="button"
              variant="outline"
              onClick={handleClear}
              disabled={saving}
              className="text-destructive hover:text-destructive"
            >
              Clear (fetch all history)
            </Button>
            <Button type="submit" disabled={saving || !dateValue}>
              {saving && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
              {saving ? "Saving…" : "Set date"}
            </Button>
          </DialogFooter>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

export function ConnectionsPage() {
  const queryClient = useQueryClient();
  const [linking, setLinking] = useState(false);
  const [linkError, setLinkError] = useState(null);
  const [pendingSession, setPendingSession] = useState(null);
  const [pendingConfigureAccount, setPendingConfigureAccount] = useState(null);
  const [updateFetchDateAccount, setUpdateFetchDateAccount] = useState(null);

  const {
    data: configData,
    isLoading: configLoading,
    error: configError,
  } = useQuery({
    queryKey: queryKeys.stripeConfig(),
    queryFn: () => ledgerClient.getStripeConfig({}),
  });

  const {
    data: linkedData,
    isLoading: linkedLoading,
    error: linkedError,
  } = useQuery({
    queryKey: queryKeys.stripeLinkedAccounts(),
    queryFn: () => ledgerClient.listStripeLinkedAccounts({}),
    enabled: !!configData?.enabled,
  });

  const { data: declarationsData } = useQuery({
    queryKey: queryKeys.accountDeclarations(),
    queryFn: () => ledgerClient.listAccountDeclarations({}),
    enabled: !!configData?.enabled,
  });

  async function handleLinkAccount() {
    setLinkError(null);
    setLinking(true);
    try {
      const sessionRes = await ledgerClient.createStripeLinkSession({});
      const { loadStripe } = await import("@stripe/stripe-js");
      const stripe = await loadStripe(configData.publishableKey);
      if (!stripe) throw new Error("Failed to load Stripe.js");
      const result = await stripe.collectFinancialConnectionsAccounts({
        clientSecret: sessionRes.clientSecret,
      });
      if (result.error) {
        setLinkError(new Error(result.error.message));
        return;
      }
      const session = result.financialConnectionsSession;
      if (!session || !session.accounts || session.accounts.length === 0) {
        setLinkError(new Error("No accounts were selected."));
        return;
      }
      setPendingSession({ id: session.id, accounts: session.accounts });
    } catch (err) {
      setLinkError(err);
    } finally {
      setLinking(false);
    }
  }

  async function handleCompleteLinking(mappings) {
    const accounts = pendingSession.accounts.map((a) => ({
      stripeAccountId: a.id,
      hledgerAccount: mappings[a.id].hledgerAccount,
      displayName:
        mappings[a.id].displayName ||
        a.display_name ||
        a.displayName ||
        a.id,
    }));
    await ledgerClient.completeStripeLinking({ accounts });
    setPendingSession(null);
    queryClient.invalidateQueries({ queryKey: queryKeys.stripeLinkedAccounts() });
    queryClient.invalidateQueries({ queryKey: queryKeys.stripeConfig() });
  }

  const unlinkMutation = useMutation({
    mutationFn: (stripeAccountId) =>
      ledgerClient.unlinkStripeAccount({ stripeAccountId }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.stripeLinkedAccounts() });
      queryClient.invalidateQueries({ queryKey: queryKeys.stripeConfig() });
    },
  });

  function handleUnlink(stripeAccountId) {
    if (!confirm("Unlink this account? Existing imported transactions will not be removed.")) return;
    unlinkMutation.mutate(stripeAccountId);
  }

  async function handleConfigureComplete(mappings) {
    const a = pendingConfigureAccount;
    await ledgerClient.completeStripeLinking({
      accounts: [{
        stripeAccountId: a.id,
        hledgerAccount: mappings[a.id].hledgerAccount,
        displayName: mappings[a.id].displayName || a.display_name || a.id,
      }],
    });
    setPendingConfigureAccount(null);
    queryClient.invalidateQueries({ queryKey: queryKeys.stripeLinkedAccounts() });
    queryClient.invalidateQueries({ queryKey: queryKeys.stripeConfig() });
  }

  if (configLoading) return <Loading />;
  if (configError) return <ErrorBanner error={configError} />;

  if (!configData?.enabled) {
    return (
      <div className="flex flex-col gap-6">
        <h2 className="text-2xl font-bold">Connections</h2>
        <Card>
          <CardHeader>
            <CardTitle>Stripe Integration Not Enabled</CardTitle>
            <CardDescription>
              Set the following environment variables when starting floatd to enable bank
              account linking via Stripe Financial Connections.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <div className="rounded-md bg-muted p-3 font-mono text-sm leading-relaxed">
              <div>STRIPE_SECRET_KEY=sk_live_…</div>
              <div>STRIPE_PUBLISHABLE_KEY=pk_live_…</div>
            </div>
            <p className="text-sm text-muted-foreground">
              Restart floatd after setting the variables. Use test keys (
              <code className="font-mono text-xs">sk_test_…</code> /{" "}
              <code className="font-mono text-xs">pk_test_…</code>) for development.
            </p>
          </CardContent>
        </Card>
      </div>
    );
  }

  const accounts = linkedData?.accounts ?? [];
  const configuredAccounts = accounts.filter((a) => a.hledgerAccount);
  const unconfiguredAccounts = accounts.filter((a) => !a.hledgerAccount);

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-bold">Connections</h2>
        <Button onClick={handleLinkAccount} disabled={linking}>
          {linking ? (
            <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />
          ) : (
            <Link2 data-icon="inline-start" />
          )}
          {linking ? "Connecting…" : "Link Account"}
        </Button>
      </div>

      {linkError && <ErrorBanner error={linkError} />}
      {unlinkMutation.error && <ErrorBanner error={unlinkMutation.error} />}
      {linkedError && <ErrorBanner error={linkedError} />}

      {linkedLoading && <Loading />}

      {!linkedLoading && accounts.length === 0 && (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            No linked accounts. Click <strong>Link Account</strong> to connect your bank.
          </CardContent>
        </Card>
      )}

      {configuredAccounts.length > 0 && (
        <FetchAllPanel
          configuredAccounts={configuredAccounts}
          onImported={() =>
            queryClient.invalidateQueries({ queryKey: queryKeys.stripeLinkedAccounts() })
          }
        />
      )}

      {configuredAccounts.map((account) => (
        <Card key={account.stripeAccountId}>
          <CardHeader>
            <div className="flex items-start justify-between gap-4">
              <div className="flex flex-col gap-1">
                <CardTitle className="text-base">{account.displayName}</CardTitle>
                <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
                  {account.institutionName && (
                    <span>{account.institutionName}</span>
                  )}
                  {account.institutionName && <span>·</span>}
                  <span className="font-mono">{account.hledgerAccount}</span>
                  <span>·</span>
                  <button
                    type="button"
                    className="flex items-center gap-1 hover:text-foreground transition-colors"
                    title="Update last fetched date"
                    onClick={() => setUpdateFetchDateAccount(account)}
                  >
                    <Calendar className="size-3" />
                    {account.lastFetchedAt
                      ? <>Last fetched {account.lastFetchedAt}</>
                      : <>Set fetch date</>
                    }
                  </button>
                </div>
              </div>
              <Button
                variant="ghost"
                size="sm"
                className="text-destructive hover:text-destructive shrink-0"
                disabled={unlinkMutation.isPending && unlinkMutation.variables === account.stripeAccountId}
                onClick={() => handleUnlink(account.stripeAccountId)}
              >
                {unlinkMutation.isPending && unlinkMutation.variables === account.stripeAccountId ? (
                  <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />
                ) : (
                  <Link2Off data-icon="inline-start" />
                )}
                Unlink
              </Button>
            </div>
          </CardHeader>
          <CardContent>
            <AccountFetchPanel
              account={account}
              onImported={() =>
                queryClient.invalidateQueries({
                  queryKey: queryKeys.stripeLinkedAccounts(),
                })
              }
            />
          </CardContent>
        </Card>
      ))}

      {unconfiguredAccounts.map((account) => (
        <Card key={account.stripeAccountId}>
          <CardHeader>
            <div className="flex items-start justify-between gap-4">
              <div className="flex flex-col gap-1">
                <CardTitle className="text-base">
                  {account.displayName || account.stripeAccountId}
                </CardTitle>
                <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
                  {account.institutionName && <span>{account.institutionName}</span>}
                  <span className="font-mono text-xs">{account.stripeAccountId}</span>
                </div>
              </div>
              <div className="flex gap-2 shrink-0">
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => setPendingConfigureAccount({ id: account.stripeAccountId, display_name: account.displayName })}
                >
                  Configure
                </Button>
                <Button
                  variant="destructive"
                  size="sm"
                  onClick={() => handleUnlink(account.stripeAccountId)}
                  disabled={unlinkMutation.isPending}
                >
                  Disconnect
                </Button>
              </div>
            </div>
          </CardHeader>
        </Card>
      ))}

      {pendingSession && (
        <LinkMappingDialog
          key={pendingSession.id}
          open={true}
          fcAccounts={pendingSession.accounts}
          accountDeclarations={declarationsData?.declarations ?? []}
          onComplete={handleCompleteLinking}
          onClose={() => setPendingSession(null)}
        />
      )}

      {pendingConfigureAccount && (
        <LinkMappingDialog
          open={true}
          fcAccounts={[pendingConfigureAccount]}
          accountDeclarations={declarationsData?.declarations ?? []}
          onComplete={handleConfigureComplete}
          onClose={() => setPendingConfigureAccount(null)}
        />
      )}

      {updateFetchDateAccount && (
        <UpdateFetchDateDialog
          account={updateFetchDateAccount}
          open={true}
          onClose={() => setUpdateFetchDateAccount(null)}
          onUpdated={() => queryClient.invalidateQueries({ queryKey: queryKeys.stripeLinkedAccounts() })}
        />
      )}
    </div>
  );
}
