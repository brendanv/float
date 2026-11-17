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
import { ArrowUpRight, Sparkles, ChevronDown, Users } from "lucide-react";
import { cn } from "@/lib/utils";
import { ledgerClient } from "../client.js";
import { queryKeys, invalidateLedgerQueries } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { SuggestRulesWizard } from "../components/suggest-rules-wizard.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { Page, Section } from "../components/page.jsx";
import { TableSortHeader } from "../components/table-sort-header.jsx";
import { TablePagination } from "../components/table-pagination.jsx";
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
import { dayLabel } from "@/lib/transactions.js";

// Row actions show on row hover or keyboard focus on devices that can
// hover; touch devices always show them.
const ROW_ACTION =
  "[@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-hover/row:opacity-100 [@media(hover:hover)]:group-focus-within/row:opacity-100 focus-visible:opacity-100 transition-opacity";

// SectionToggle is a collapsible section heading: the title plus a chevron.
function SectionToggle({ open, children }) {
  return (
    <CollapsibleTrigger className="inline-flex items-center gap-1.5 rounded-sm text-left">
      {children}
      <ChevronDown className={cn("size-3.5 shrink-0 text-faint transition-transform duration-200", !open && "-rotate-90")} />
    </CollapsibleTrigger>
  );
}

// DataTable renders a TanStack table with the shared dense table styles.
function DataTable({ table }) {
  return (
    <Table>
      <TableHeader>
        {table.getHeaderGroups().map((hg) => (
          <TableRow key={hg.id} className="hover:bg-transparent">
            {hg.headers.map((header) => (
              <TableHead key={header.id} className={header.column.columnDef.meta?.headerClass}>
                {header.isPlaceholder
                  ? null
                  : flexRender(header.column.columnDef.header, header.getContext())}
              </TableHead>
            ))}
          </TableRow>
        ))}
      </TableHeader>
      <TableBody>
        {table.getRowModel().rows.map((row) => (
          <TableRow key={row.id} className="group/row">
            {row.getVisibleCells().map((cell) => (
              <TableCell key={cell.id} className={cn("py-1", cell.column.columnDef.meta?.cellClass)}>
                {flexRender(cell.column.columnDef.cell, cell.getContext())}
              </TableCell>
            ))}
          </TableRow>
        ))}
      </TableBody>
    </Table>
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
        header: ({ column }) => <TableSortHeader column={column}>Payee</TableSortHeader>,
        cell: ({ getValue }) => <span className="font-medium">{getValue()}</span>,
        meta: { cellClass: "w-full" },
      },
      {
        id: "actions",
        header: "",
        enableSorting: false,
        cell: ({ row }) => (
          <div className="flex justify-end">
            <Button
              variant="ghost"
              size="xs"
              className={cn("text-muted-foreground", ROW_ACTION)}
              onClick={() =>
                navigate({ to: "/transactions", search: { payee: row.original.name } })
              }
            >
              View transactions
              <ArrowUpRight data-icon="inline-end" />
            </Button>
          </div>
        ),
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
      if (!map.has(tx.description)) {
        map.set(tx.description, { fids: [], hasUncategorized: false, lastSeen: "" });
      }
      const entry = map.get(tx.description);
      entry.fids.push(tx.fid);
      if (tx.date > entry.lastSeen) entry.lastSeen = tx.date;
      if (!entry.hasUncategorized) {
        entry.hasUncategorized = (tx.postings ?? []).some((p) => p.account === "expenses:unknown");
      }
    }
    return [...map.entries()].map(([description, { fids, hasUncategorized, lastSeen }]) => ({
      description,
      fids,
      count: fids.length,
      lastSeen,
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
      invalidateLedgerQueries(queryClient);
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
        cell: ({ getValue }) => (
          <span className="block max-w-[28rem] truncate" title={getValue()}>{getValue()}</span>
        ),
        meta: { cellClass: "w-full" },
      },
      {
        id: "count",
        accessorKey: "count",
        header: ({ column }) => <TableSortHeader column={column} align="right">Count</TableSortHeader>,
        cell: ({ getValue }) => <span className="block text-right tabular-nums">{getValue()}</span>,
        meta: { headerClass: "text-right" },
      },
      {
        id: "lastSeen",
        accessorKey: "lastSeen",
        header: ({ column }) => <TableSortHeader column={column} align="right">Last seen</TableSortHeader>,
        cell: ({ getValue }) => (
          <span className="block text-right text-muted-foreground" title={getValue()}>
            {getValue() ? dayLabel(getValue()).day : "—"}
          </span>
        ),
        meta: { headerClass: "hidden text-right sm:table-cell", cellClass: "hidden sm:table-cell" },
      },
      {
        id: "actions",
        header: "",
        enableSorting: false,
        cell: ({ row }) => {
          const { description, fids } = row.original;
          return activeDesc === description ? (
            <div className="flex flex-col gap-1">
              <div className="flex items-center gap-1.5">
                <Input
                  autoFocus
                  placeholder="Payee name"
                  value={newPayee}
                  onChange={(e) => setNewPayee(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") confirmSetPayee(fids);
                    if (e.key === "Escape") cancelSetPayee();
                  }}
                  className="h-7 w-44"
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
                <p className="text-[11px] text-below">{setPayeeError}</p>
              )}
            </div>
          ) : (
            <div className="flex items-center justify-end gap-1">
              <Button
                variant="ghost"
                size="xs"
                className={ROW_ACTION}
                onClick={() => openSetPayee(description)}
              >
                Set payee
              </Button>
              <Button
                variant="ghost"
                size="xs"
                className={cn("text-muted-foreground", ROW_ACTION)}
                onClick={() =>
                  navigate({ to: "/transactions", search: { search: description } })
                }
              >
                View
                <ArrowUpRight data-icon="inline-end" />
              </Button>
            </div>
          );
        },
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

  const payeeCount = payeeTable.getFilteredRowModel().rows.length;
  const descCount = descTable.getFilteredRowModel().rows.length;

  return (
    <Page>
      <PageHeader title="Payees" />

      <Collapsible open={payeesOpen} onOpenChange={setPayeesOpen}>
        <Section
          title={<SectionToggle open={payeesOpen}>Payees</SectionToggle>}
          description={payeeRows.length > 0 ? payeeRows.length : null}
          action={
            payeesOpen && (
              <Input
                placeholder="Filter payees…"
                aria-label="Filter payees"
                value={payeeFilter}
                onChange={(e) => payeeTable.setGlobalFilter(e.target.value)}
                className="h-7 w-40 sm:w-56"
              />
            )
          }
        >
          <CollapsibleContent className="flex flex-col gap-3">
            {payeeCount === 0 ? (
              <p className="text-control text-muted-foreground">No payees found.</p>
            ) : (
              <>
                <DataTable table={payeeTable} />
                <TablePagination table={payeeTable} />
              </>
            )}
          </CollapsibleContent>
        </Section>
      </Collapsible>

      <Collapsible open={descOpen} onOpenChange={setDescOpen}>
        <Section
          title={<SectionToggle open={descOpen}>Descriptions without a payee</SectionToggle>}
          description={descRows.length > 0 ? descRows.length : null}
          action={
            descRows.length > 0 && (
              <Button variant="outline" size="sm" onClick={() => setWizardOpen(true)}>
                <Sparkles data-icon="inline-start" />
                Suggest Rules
              </Button>
            )
          }
        >
          <CollapsibleContent className="flex flex-col gap-3">
            {descRows.length === 0 ? (
              <EmptyState
                icon={Users}
                title="All transactions have a payee assigned"
              />
            ) : (
              <>
                <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
                  <Input
                    placeholder="Filter descriptions…"
                    aria-label="Filter descriptions"
                    value={descFilter}
                    onChange={(e) => descTable.setGlobalFilter(e.target.value)}
                    className="h-7 w-full sm:w-56"
                  />
                  <Label className="flex cursor-pointer items-center gap-2 text-control font-normal">
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
                {descCount === 0 ? (
                  <p className="text-control text-muted-foreground">
                    No descriptions match your filter.
                  </p>
                ) : (
                  <>
                    <DataTable table={descTable} />
                    <TablePagination table={descTable} />
                  </>
                )}
              </>
            )}
          </CollapsibleContent>
        </Section>
      </Collapsible>

      <SuggestRulesWizard
        open={wizardOpen}
        onOpenChange={setWizardOpen}
        accounts={accountsData?.accounts ?? []}
        initialSourceType="nopayee"
        onRulesAdded={() => queryClient.invalidateQueries({ queryKey: queryKeys.rules() })}
      />
    </Page>
  );
}
