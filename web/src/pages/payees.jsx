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
import { Loader2, ExternalLink, ArrowUpDown, ArrowUp, ArrowDown, ChevronLeftIcon, ChevronRightIcon, Sparkles, ChevronDown } from "lucide-react";
import { cn } from "@/lib/utils";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { SuggestRulesWizard } from "../components/suggest-rules-wizard.jsx";
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
import { Badge } from "@/components/ui/badge";
import {
  Pagination,
  PaginationContent,
  PaginationItem,
} from "@/components/ui/pagination";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

function SortHeader({ column, children }) {
  const sorted = column.getIsSorted();
  return (
    <Button
      variant="ghost"
      size="sm"
      className="-ml-3 h-8 gap-1"
      onClick={column.getToggleSortingHandler()}
    >
      {children}
      {sorted === "asc" ? (
        <ArrowUp className="size-3.5" />
      ) : sorted === "desc" ? (
        <ArrowDown className="size-3.5" />
      ) : (
        <ArrowUpDown className="size-3.5 opacity-40" />
      )}
    </Button>
  );
}

function TablePagination({ table }) {
  const { pageIndex, pageSize } = table.getState().pagination;
  const total = table.getFilteredRowModel().rows.length;
  if (total === 0) return null;
  const from = pageIndex * pageSize + 1;
  const to = Math.min((pageIndex + 1) * pageSize, total);
  return (
    <div className="mt-3 flex w-full flex-wrap items-center justify-between gap-2">
      <div className="flex items-center gap-2">
        <Label className="whitespace-nowrap text-sm text-muted-foreground">Rows per page:</Label>
        <Select
          value={String(pageSize)}
          onValueChange={(val) => {
            table.setPageSize(Number(val));
            table.setPageIndex(0);
          }}
        >
          <SelectTrigger className="h-8 w-16">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="10">10</SelectItem>
            <SelectItem value="25">25</SelectItem>
            <SelectItem value="50">50</SelectItem>
            <SelectItem value="100">100</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div className="flex items-center gap-2">
        <span className="whitespace-nowrap text-sm text-muted-foreground">
          {from}–{to} of {total}
        </span>
        <Pagination>
          <PaginationContent>
            <PaginationItem>
              <Button
                aria-label="Go to previous page"
                size="icon"
                variant="ghost"
                className="size-8"
                onClick={() => table.previousPage()}
                disabled={!table.getCanPreviousPage()}
              >
                <ChevronLeftIcon className="size-4" />
              </Button>
            </PaginationItem>
            <PaginationItem>
              <Button
                aria-label="Go to next page"
                size="icon"
                variant="ghost"
                className="size-8"
                onClick={() => table.nextPage()}
                disabled={!table.getCanNextPage()}
              >
                <ChevronRightIcon className="size-4" />
              </Button>
            </PaginationItem>
          </PaginationContent>
        </Pagination>
      </div>
    </div>
  );
}

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
        header: ({ column }) => <SortHeader column={column}>Payee</SortHeader>,
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
      if (!map.has(tx.description)) map.set(tx.description, []);
      map.get(tx.description).push(tx.fid);
    }
    return [...map.entries()].map(([description, fids]) => ({
      description,
      fids,
      count: fids.length,
    }));
  }, [txData]);

  const [descFilter, setDescFilter] = useState("");
  const [descSorting, setDescSorting] = useState([{ id: "count", desc: true }]);
  const [descPagination, setDescPagination] = useState({ pageIndex: 0, pageSize: 25 });
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
        header: ({ column }) => <SortHeader column={column}>Description</SortHeader>,
        cell: ({ getValue }) => <span className="font-mono text-sm">{getValue()}</span>,
      },
      {
        id: "count",
        accessorKey: "count",
        header: ({ column }) => <SortHeader column={column}>Count</SortHeader>,
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
                  disabled={settingPayee || !newPayee.trim()}
                  onClick={() => confirmSetPayee(fids)}
                >
                  {settingPayee ? <Loader2 className="size-3 animate-spin" /> : "Set"}
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
    data: descRows,
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
      {/* Section 1: Explicit payees */}
      <Collapsible open={payeesOpen} onOpenChange={setPayeesOpen}>
        <Card>
          <CardHeader>
            <CollapsibleTrigger className="flex w-full items-center justify-between gap-2 text-left">
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
                <p className="text-sm text-muted-foreground">
                  All transactions have a payee assigned.
                </p>
              ) : (
                <>
                  <Input
                    placeholder="Filter descriptions…"
                    value={descFilter}
                    onChange={(e) => descTable.setGlobalFilter(e.target.value)}
                    className="max-w-sm"
                  />
                  {descTable.getFilteredRowModel().rows.length === 0 ? (
                    <p className="text-sm text-muted-foreground">
                      No descriptions match your filter.
                    </p>
                  ) : (
                    <>
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
