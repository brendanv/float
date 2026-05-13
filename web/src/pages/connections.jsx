import { useState, useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
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
  Loader2,
  Tag,
  ArrowUp,
  ArrowDown,
  ArrowUpDown,
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
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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

function LinkMappingDialog({ open, sessionId, fcAccounts, accountDeclarations, onComplete, onClose }) {
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
      await onComplete(sessionId, mappings);
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
        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <p className="text-sm text-muted-foreground">
            {fcAccounts.length === 1
              ? "1 account was linked. Choose the hledger account for it."
              : `${fcAccounts.length} accounts were linked. Choose the hledger account for each.`}
          </p>
          {fcAccounts.map((a) => (
            <div key={a.id} className="flex flex-col gap-3 rounded-md border p-3">
              <span className="text-xs text-muted-foreground font-mono">{a.id}</span>
              <div className="flex flex-col gap-1.5">
                <Label className="text-xs">Display Name</Label>
                <Input
                  value={mappings[a.id]?.displayName ?? ""}
                  onChange={(e) => setMapping(a.id, "displayName", e.target.value)}
                  placeholder={a.display_name ?? a.displayName ?? a.id}
                />
              </div>
              <div className="flex flex-col gap-1.5">
                <Label className="text-xs">hledger Account</Label>
                <Select
                  value={mappings[a.id]?.hledgerAccount || undefined}
                  onValueChange={(v) => setMapping(a.id, "hledgerAccount", v)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select account…" />
                  </SelectTrigger>
                  <SelectContent>
                    {(accountDeclarations ?? []).map((d) => (
                      <SelectItem key={d.name} value={d.name}>{d.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          ))}
          {error && <ErrorBanner error={error} />}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
            <Button type="submit" disabled={saving || !allMapped}>
              {saving && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
              {saving ? "Saving…" : "Save"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function AccountFetchPanel({ account, onImported }) {
  const [candidates, setCandidates] = useState(null);
  const [fetching, setFetching] = useState(false);
  const [fetchError, setFetchError] = useState(null);
  const [selectedIndices, setSelectedIndices] = useState(new Set());
  const [sorting, setSorting] = useState([]);
  const [importing, setImporting] = useState(false);
  const [importProgress, setImportProgress] = useState(null);
  const [importError, setImportError] = useState(null);
  const [importResult, setImportResult] = useState(null);

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
      res.candidates.forEach((c, i) => {
        if (!c.isDuplicate) autoSelected.add(i);
      });
      setSelectedIndices(autoSelected);
    } catch (err) {
      setFetchError(err);
    } finally {
      setFetching(false);
    }
  }

  function toggleCandidate(idx) {
    setSelectedIndices((prev) => {
      const next = new Set(prev);
      if (next.has(idx)) next.delete(idx);
      else next.add(idx);
      return next;
    });
  }

  function toggleAll() {
    if (!candidates) return;
    const allNew = candidates
      .map((c, i) => ({ c, i }))
      .filter(({ c }) => !c.isDuplicate)
      .map(({ i }) => i);
    if (selectedIndices.size === allNew.length) {
      setSelectedIndices(new Set());
    } else {
      setSelectedIndices(new Set(allNew));
    }
  }

  async function handleImport() {
    if (selectedIndices.size === 0) return;
    setImportError(null);
    setImporting(true);
    setImportProgress({ imported: 0, total: selectedIndices.size });
    try {
      for await (const res of ledgerClient.importStripeTransactions({
        stripeAccountId: account.stripeAccountId,
        candidateIndices: Array.from(selectedIndices),
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

  const columns = useMemo(
    () => [
      {
        id: "select",
        header: () => null,
        cell: ({ row }) => (
          <Checkbox
            checked={selectedIndices.has(row.original._originalIndex)}
            disabled={row.original.isDuplicate}
            onCheckedChange={() => toggleCandidate(row.original._originalIndex)}
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
    [selectedIndices]
  );

  const tableData = useMemo(
    () => (candidates ? candidates.map((c, i) => ({ ...c, _originalIndex: i })) : []),
    [candidates]
  );

  const table = useReactTable({
    data: tableData,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  return (
    <div className="flex flex-col gap-3 pt-3 border-t">
      <div>
        <Button size="sm" variant="secondary" onClick={handleFetch} disabled={fetching}>
          {fetching && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
          {fetching ? "Fetching…" : "Fetch Transactions"}
        </Button>
      </div>
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
                {selectedIndices.size === newCount ? "Deselect All" : "Select All New"}
              </Button>
              <Button
                size="sm"
                onClick={handleImport}
                disabled={importing || selectedIndices.size === 0}
              >
                {importing && (
                  <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />
                )}
                {importing
                  ? importProgress && importProgress.total > 0
                    ? `Importing ${importProgress.imported} of ${importProgress.total}…`
                    : "Importing…"
                  : `Import ${selectedIndices.size} Selected`}
              </Button>
            </div>
          </div>
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
        </div>
      )}
    </div>
  );
}

export function ConnectionsPage() {
  const queryClient = useQueryClient();
  const [linking, setLinking] = useState(false);
  const [linkError, setLinkError] = useState(null);
  const [pendingSession, setPendingSession] = useState(null);
  const [unlinkingId, setUnlinkingId] = useState(null);
  const [unlinkError, setUnlinkError] = useState(null);

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

  async function handleCompleteLinking(sessionId, mappings) {
    const accounts = pendingSession.accounts.map((a) => ({
      stripeAccountId: a.id,
      hledgerAccount: mappings[a.id].hledgerAccount,
      displayName:
        mappings[a.id].displayName ||
        a.display_name ||
        a.displayName ||
        a.id,
    }));
    await ledgerClient.completeStripeLinking({ sessionId, accounts });
    setPendingSession(null);
    queryClient.invalidateQueries({ queryKey: queryKeys.stripeLinkedAccounts() });
    queryClient.invalidateQueries({ queryKey: queryKeys.stripeConfig() });
  }

  async function handleUnlink(stripeAccountId) {
    if (
      !confirm(
        "Unlink this account? Existing imported transactions will not be removed."
      )
    )
      return;
    setUnlinkingId(stripeAccountId);
    setUnlinkError(null);
    try {
      await ledgerClient.unlinkStripeAccount({ stripeAccountId });
      queryClient.invalidateQueries({ queryKey: queryKeys.stripeLinkedAccounts() });
      queryClient.invalidateQueries({ queryKey: queryKeys.stripeConfig() });
    } catch (err) {
      setUnlinkError(err);
    } finally {
      setUnlinkingId(null);
    }
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
      {unlinkError && <ErrorBanner error={unlinkError} />}
      {linkedError && <ErrorBanner error={linkedError} />}

      {linkedLoading && <Loading />}

      {!linkedLoading && accounts.length === 0 && (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            No linked accounts. Click <strong>Link Account</strong> to connect your bank.
          </CardContent>
        </Card>
      )}

      {accounts.map((account) => (
        <Card key={account.stripeAccountId}>
          <CardHeader>
            <div className="flex items-start justify-between gap-4">
              <div className="flex flex-col gap-1">
                <CardTitle className="text-base">{account.displayName}</CardTitle>
                <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
                  <span className="font-mono">{account.hledgerAccount}</span>
                  {account.lastFetchedAt && (
                    <>
                      <span>·</span>
                      <span>Last fetched {account.lastFetchedAt}</span>
                    </>
                  )}
                </div>
              </div>
              <Button
                variant="ghost"
                size="sm"
                className="text-destructive hover:text-destructive shrink-0"
                disabled={unlinkingId === account.stripeAccountId}
                onClick={() => handleUnlink(account.stripeAccountId)}
              >
                {unlinkingId === account.stripeAccountId ? (
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

      {pendingSession && (
        <LinkMappingDialog
          key={pendingSession.id}
          open={true}
          sessionId={pendingSession.id}
          fcAccounts={pendingSession.accounts}
          accountDeclarations={declarationsData?.declarations ?? []}
          onComplete={handleCompleteLinking}
          onClose={() => setPendingSession(null)}
        />
      )}
    </div>
  );
}
