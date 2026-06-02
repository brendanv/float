import { useState, useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  flexRender,
} from "@tanstack/react-table";
import { ExternalLink, Sparkles, ChevronDown, Users } from "lucide-react";
import { cn } from "@/lib/utils";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { SuggestRulesWizard } from "../components/suggest-rules-wizard.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { TableSortHeader } from "../components/table-sort-header.jsx";
import { TablePagination } from "../components/table-pagination.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@/components/ui/collapsible";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Badge } from "@/components/ui/badge";

export function PayeesPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const {
    data: payeesData,
    isLoading: payeesLoading,
    error: payeesError,
  } = useQuery({
    queryKey: queryKeys.payees(),
    queryFn: () => ledgerClient.listPayees({}),
  });

  const {
    data: txData,
    isLoading: txLoading,
    error: txError,
  } = useQuery({
    queryKey: queryKeys.noPayeeTransactions(),
    queryFn: () => ledgerClient.listTransactions({ query: ["not:desc:.*[|].*"] }),
  });

  const { data: accountsData } = useQuery({
    queryKey: queryKeys.accounts(),
    queryFn: () => ledgerClient.listAccounts({}),
  });

  const [wizardOpen, setWizardOpen] = useState(false);
  const [payeesOpen, setPayeesOpen] = useState(true);
  const [descOpen, setDescOpen] = useState(true);

  // ── Payees table ──────────────────────────────────────────────────────────

  const payeeRows = useMemo(
    () => (payeesData?.payees ?? []).map((name) => ({ name })),
    [payeesData],
  );

  const [payeeFilter, setPayeeFilter] = useState("");
  const [payeeSorting, setPayeeSorting] = useState([{ id: "name", desc: false }]);
  const [payeePagination, setPayeePagination] = useState({ pageIndex: 0, pageSize: 25 });

  const payeeColumns = useMemo(
    () => [
      {
        id: "name",
        accessorKey: "name",
        header: ({ column }) => <TableSortHeader column={column}>Payee</TableSortHeader>,
        cell: ({ getValue }) => <span className="font-medium">{getValue()}</span>,
      },
      {
        id: "actions",
        header: "",
        enableSorting: false,
        cell: ({ row }) => (
          <div className="flex justify-end">
            <Button
              variant="ghost"
              size="sm"
              className="gap-1.5"
              onClick={() =>
                navigate({ to: "/transactions", search: { payee: row.original.name } })
              }
            >
              <ExternalLink className="size-3.5" />
              View transactions
            </Button>
          </div>
        ),
        meta: { headerClass: "w-44" },
      },
    ],
    [navigate],
  );

  const payeeTable = useReactTable({
    data: payeeRows,
    columns: payeeColumns,
    state: { sorting: payeeSorting, globalFilter: payeeFilter, pagination: payeePagination },
    onSortingChange: setPayeeSorting,
    onGlobalFilterChange: (filter) => {
      setPayeeFilter(filter);
      setPayeePagination((p) => ({ ...p, pageIndex: 0 }));
    },
    onPaginationChange: setPayeePagination,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    globalFilterFn: "includesString",
  });

  // ── Descriptions table ────────────────────────────────────────────────────

  const descRows = useMemo(() => {
    const map = new Map();
    for (const tx of txData?.transactions ?? []) {
      if (!map.has(tx.description)) map.set(tx.description, { fids: [], hasUncategorized: false });
      const entry = map.get(tx.description);
      entry.fids.push(tx.fid);
      if (!entry.hasUncategorized) {
        entry.hasUncategorized = (tx.postings ?? []).some((p) => p.account === "expenses:unknown");
      }
    }
    return [...map.entries()].map(([description, { fids, hasUncategorized }]) => ({
      description,
      fids,
      count: fids.length,
      hasUncategorized,
    }));
  }, [txData]);

  const [descFilter, setDescFilter] = useState("");
  const [descSorting, setDescSorting] = useState([{ id: "count", desc: true }]);
  const [descPagination, setDescPagination] = useState({ pageIndex: 0, pageSize: 25 });
  const [showUncategorizedOnly, setShowUncategorizedOnly] = useState(false);

  const filteredDescRows = useMemo(
    () => (showUncategorizedOnly ? descRows.filter((r) => r.hasUncategorized) : descRows),
    [descRows, showUncategorizedOnly],
  );
  const [activeDesc, setActiveDesc] = useState(null);
  const [newPayee, setNewPayee] = useState("");
  const [settingPayee, setSettingPayee] = useState(false);
  const [setPayeeError, setSetPayeeError] = useState(null);

  function openSetPayee(desc) {
    setActiveDesc(desc);
    setNewPayee("");
    setSetPayeeError(null);
  }

  function cancelSetPayee() {
    setActiveDesc(null);
    setNewPayee("");
    setSetPayeeError(null);
  }

  async function confirmSetPayee(fids) {
    if (!newPayee.trim()) return;
    setSettingPayee(true);
    setSetPayeeError(null);
    try {
      await ledgerClient.bulkEditTransactions({
        fids,
        operations: [
          { operation: { case: "setPayee", value: { payee: newPayee.trim() } } },
        ],
      });
      setActiveDesc(null);
      setNewPayee("");
      queryClient.invalidateQueries({ queryKey: ["transactions"] });
      queryClient.invalidateQueries({ queryKey: ["accountRegister"] });
      queryClient.invalidateQueries({ queryKey: queryKeys.noPayeeTransactions() });
      queryClient.invalidateQueries({ queryKey: queryKeys.payees() });
    } catch (err) {
      setSetPayeeError(err.message || String(err));
    } finally {
      setSettingPayee(false);
    }
  }

  const descColumns = useMemo(
    () => [
      {
        id: "description",
        accessorKey: "description",
        header: ({ column }) => <TableSortHeader column={column}>Description</TableSortHeader>,
        cell: ({ getValue }) => <span className="font-mono text-sm">{getValue()}</span>,
      },
      {
        id: "count",
        accessorKey: "count",
        header: ({ column }) => <TableSortHeader column={column}>Count</TableSortHeader>,
        cell: ({ getValue }) => <Badge variant="secondary">{getValue()}</Badge>,
        meta: { headerClass: "w-28" },
      },
      {
        id: "actions",
        header: "",
        enableSorting: false,
        cell: ({ row }) => {
          const { description, fids } = row.original;
          return activeDesc === description ? (
            <div className="flex flex-col gap-2">
              <div className="flex items-center gap-2">
                <Input
                  autoFocus
                  placeholder="Payee name"
                  value={newPayee}
                  onChange={(e) => setNewPayee(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") confirmSetPayee(fids);
                    if (e.key === "Escape") cancelSetPayee();
                  }}
                  className="h-7 text-sm"
                />
                <Button
                  size="xs"
                  disabled={!newPayee.trim()}
                  isLoading={settingPayee}
                  onClick={() => confirmSetPayee(fids)}
                >
                  Set
                </Button>
                <Button
                  variant="ghost"
                  size="xs"
                  disabled={settingPayee}
                  onClick={cancelSetPayee}
                >
                  Cancel
                </Button>
              </div>
              {setPayeeError && (
                <p className="text-xs text-destructive">{setPayeeError}</p>
              )}
            </div>
          ) : (
            <div className="flex items-center justify-end gap-2">
              <Button variant="ghost" size="sm" onClick={() => openSetPayee(description)}>
                Set payee
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="gap-1.5"
                onClick={() =>
                  navigate({ to: "/transactions", search: { search: description } })
                }
              >
                <ExternalLink className="size-3.5" />
                View
              </Button>
            </div>
          );
        },
        meta: { headerClass: "w-72" },
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [navigate, activeDesc, newPayee, settingPayee, setPayeeError],
  );

  const descTable = useReactTable({
    data: filteredDescRows,
    columns: descColumns,
    state: { sorting: descSorting, globalFilter: descFilter, pagination: descPagination },
    onSortingChange: setDescSorting,
    onGlobalFilterChange: (filter) => {
      setDescFilter(filter);
      setDescPagination((p) => ({ ...p, pageIndex: 0 }));
    },
    onPaginationChange: setDescPagination,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    globalFilterFn: "includesString",
  });

  // ─────────────────────────────────────────────────────────────────────────

  const isLoading = payeesLoading || txLoading;
  const error = payeesError || txError;

  if (isLoading) return <Loading />;
  if (error) return <ErrorBanner error={error} />;

  return (
    <div className="flex flex-col gap-8">
      <PageHeader title="Payees" />
      {/* Section 1: Explicit payees */}
      <Collapsible open={payeesOpen} onOpenChange={setPayeesOpen}>
        <Card>
          <CardHeader>
            <CollapsibleTrigger className="flex items-center gap-2 text-left">
              <CardTitle>Payees</CardTitle>
              <ChevronDown className={cn("size-4 shrink-0 text-muted-foreground transition-transform duration-200", payeesOpen && "rotate-180")} />
            </CollapsibleTrigger>
          </CardHeader>
          <CollapsibleContent>
            <CardContent className="flex flex-col gap-4">
              <Input
                placeholder="Filter payees…"
                value={payeeFilter}
                onChange={(e) => payeeTable.setGlobalFilter(e.target.value)}
                className="max-w-sm"
              />
              {payeeTable.getFilteredRowModel().rows.length === 0 ? (
                <p className="text-sm text-muted-foreground">No payees found.</p>
              ) : (
                <>
                  <div className="overflow-x-auto">
                  <Table>
                    <TableHeader>
                      {payeeTable.getHeaderGroups().map((hg) => (
                        <TableRow key={hg.id}>
                          {hg.headers.map((header) => (
                            <TableHead
                              key={header.id}
                              className={header.column.columnDef.meta?.headerClass}
                            >
                              {header.isPlaceholder
                                ? null
                                : flexRender(header.column.columnDef.header, header.getContext())}
                            </TableHead>
                          ))}
                        </TableRow>
                      ))}
                    </TableHeader>
                    <TableBody>
                      {payeeTable.getRowModel().rows.map((row) => (
                        <TableRow key={row.id}>
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
                  <TablePagination table={payeeTable} />
                </>
              )}
            </CardContent>
          </CollapsibleContent>
        </Card>
      </Collapsible>

      {/* Section 2: Common descriptions without payees */}
      <Collapsible open={descOpen} onOpenChange={setDescOpen}>
        <Card>
          <CardHeader>
            <div className="flex flex-wrap items-center justify-between gap-4">
              <CollapsibleTrigger className="flex items-center gap-2 text-left">
                <CardTitle>Common descriptions without a payee</CardTitle>
                <ChevronDown className={cn("size-4 shrink-0 text-muted-foreground transition-transform duration-200", descOpen && "rotate-180")} />
              </CollapsibleTrigger>
              {descRows.length > 0 && (
                <Button variant="outline" size="sm" onClick={() => setWizardOpen(true)}>
                  <Sparkles data-icon="inline-start" className="size-3.5" />
                  Suggest Rules
                </Button>
              )}
            </div>
          </CardHeader>
          <CollapsibleContent>
            <CardContent className="flex flex-col gap-4">
              {descRows.length === 0 ? (
                <EmptyState
                  icon={Users}
                  title="All transactions have a payee assigned"
                />
              ) : (
                <>
                  <div className="flex flex-wrap items-center gap-4">
                    <Input
                      placeholder="Filter descriptions…"
                      value={descFilter}
                      onChange={(e) => descTable.setGlobalFilter(e.target.value)}
                      className="max-w-sm"
                    />
                    <Label className="flex cursor-pointer items-center gap-2 text-sm">
                      <Checkbox
                        checked={showUncategorizedOnly}
                        onCheckedChange={(checked) => {
                          setShowUncategorizedOnly(!!checked);
                          setDescPagination((p) => ({ ...p, pageIndex: 0 }));
                        }}
                      />
                      Uncategorized only
                    </Label>
                  </div>
                  {descTable.getFilteredRowModel().rows.length === 0 ? (
                    <p className="text-sm text-muted-foreground">
                      No descriptions match your filter.
                    </p>
                  ) : (
                    <>
                      <div className="overflow-x-auto">
                      <Table>
                        <TableHeader>
                          {descTable.getHeaderGroups().map((hg) => (
                            <TableRow key={hg.id}>
                              {hg.headers.map((header) => (
                                <TableHead
                                  key={header.id}
                                  className={header.column.columnDef.meta?.headerClass}
                                >
                                  {header.isPlaceholder
                                    ? null
                                    : flexRender(
                                        header.column.columnDef.header,
                                        header.getContext(),
                                      )}
                                </TableHead>
                              ))}
                            </TableRow>
                          ))}
                        </TableHeader>
                        <TableBody>
                          {descTable.getRowModel().rows.map((row) => (
                            <TableRow key={row.id}>
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
                      <TablePagination table={descTable} />
                    </>
                  )}
                </>
              )}
            </CardContent>
          </CollapsibleContent>
        </Card>
      </Collapsible>

      <SuggestRulesWizard
        open={wizardOpen}
        onOpenChange={setWizardOpen}
        accounts={accountsData?.accounts ?? []}
        initialSourceType="nopayee"
        onRulesAdded={() => queryClient.invalidateQueries({ queryKey: queryKeys.rules() })}
      />
    </div>
  );
}
