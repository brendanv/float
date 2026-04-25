import { useState, useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  flexRender,
} from "@tanstack/react-table";
import { Loader2, ExternalLink, ArrowUpDown, ArrowUp, ArrowDown } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";

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
    queryFn: () => ledgerClient.listTransactions({ query: ["not:payee:.+"] }),
  });

  // Group unassigned transactions by description
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

  // ── Payees table ──────────────────────────────────────────────────────────

  const [payeeFilter, setPayeeFilter] = useState("");
  const [payeeSorting, setPayeeSorting] = useState([{ id: "name", desc: false }]);

  const payeeRows = useMemo(() => {
    const q = payeeFilter.trim().toLowerCase();
    const all = (payeesData?.payees ?? []).map((name) => ({ name }));
    return q ? all.filter((r) => r.name.toLowerCase().includes(q)) : all;
  }, [payeesData, payeeFilter]);

  const payeeColumns = useMemo(
    () => [
      {
        id: "name",
        accessorKey: "name",
        header: ({ column }) => <SortHeader column={column}>Payee</SortHeader>,
        cell: ({ getValue }) => (
          <span className="font-medium">{getValue()}</span>
        ),
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
    [navigate]
  );

  const payeeTable = useReactTable({
    data: payeeRows,
    columns: payeeColumns,
    state: { sorting: payeeSorting },
    onSortingChange: setPayeeSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  // ── Descriptions table ────────────────────────────────────────────────────

  const [descFilter, setDescFilter] = useState("");
  const [descSorting, setDescSorting] = useState([{ id: "count", desc: true }]);
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
        header: ({ column }) => (
          <SortHeader column={column}>Description</SortHeader>
        ),
        cell: ({ getValue }) => (
          <span className="font-mono text-sm">{getValue()}</span>
        ),
      },
      {
        id: "count",
        accessorKey: "count",
        header: ({ column }) => (
          <SortHeader column={column}>Count</SortHeader>
        ),
        cell: ({ getValue }) => (
          <Badge variant="secondary">{getValue()}</Badge>
        ),
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
                  {settingPayee ? (
                    <Loader2 className="size-3 animate-spin" />
                  ) : (
                    "Set"
                  )}
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
              <Button
                variant="ghost"
                size="sm"
                onClick={() => openSetPayee(description)}
              >
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
    [navigate, activeDesc, newPayee, settingPayee, setPayeeError]
  );

  const filteredDescRows = useMemo(() => {
    const q = descFilter.trim().toLowerCase();
    return q ? descRows.filter((r) => r.description.toLowerCase().includes(q)) : descRows;
  }, [descRows, descFilter]);

  const descTable = useReactTable({
    data: filteredDescRows,
    columns: descColumns,
    state: { sorting: descSorting },
    onSortingChange: setDescSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  // ─────────────────────────────────────────────────────────────────────────

  const isLoading = payeesLoading || txLoading;
  const error = payeesError || txError;

  if (isLoading) return <Loading />;
  if (error) return <ErrorBanner error={error} />;

  return (
    <div className="flex flex-col gap-8">
      {/* Section 1: Explicit payees */}
      <Card>
        <CardHeader>
          <CardTitle>Payees</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <Input
            placeholder="Filter payees…"
            value={payeeFilter}
            onChange={(e) => setPayeeFilter(e.target.value)}
            className="max-w-sm"
          />
          {payeeTable.getRowModel().rows.length === 0 ? (
            <p className="text-sm text-muted-foreground">No payees found.</p>
          ) : (
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
          )}
        </CardContent>
      </Card>

      {/* Section 2: Common descriptions without payees */}
      <Card>
        <CardHeader>
          <CardTitle>Common descriptions without a payee</CardTitle>
        </CardHeader>
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
                onChange={(e) => setDescFilter(e.target.value)}
                className="max-w-sm"
              />
              {descTable.getRowModel().rows.length === 0 ? (
                <p className="text-sm text-muted-foreground">No descriptions match your filter.</p>
              ) : (
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
                          : flexRender(header.column.columnDef.header, header.getContext())}
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
              )}
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
