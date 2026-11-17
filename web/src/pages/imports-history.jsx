import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useRouter } from "@tanstack/react-router";
import {
  useReactTable,
  getCoreRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  createColumnHelper,
  flexRender,
} from "@tanstack/react-table";
import { PackageOpen, FileText, Download, ChevronRight } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { TableSortHeader } from "../components/table-sort-header.jsx";
import { TablePagination } from "../components/table-pagination.jsx";
import { Page, Section } from "../components/page.jsx";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { downloadBlob } from "@/lib/download";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

function ImportFileDialog({ importBatchId, open, onOpenChange }) {
  const { data, isLoading, error } = useQuery({
    queryKey: ["importFile", importBatchId],
    queryFn: () => ledgerClient.getImportFile({ importBatchId }),
    enabled: open && !!importBatchId,
  });

  const csvText = data?.csvContent
    ? new TextDecoder().decode(data.csvContent)
    : null;
  const filename = data?.filename || `${importBatchId}.csv`;

  function handleDownload() {
    downloadBlob(data.csvContent, filename, "text/csv");
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="xl" showCloseButton>
        <DialogHeader>
          <DialogTitle className="font-mono text-code">{filename}</DialogTitle>
        </DialogHeader>
        <div className="min-w-0 overflow-hidden">
          {isLoading && <Loading />}
          {error && <ErrorBanner error={error} />}
          {csvText && (
            <pre className="max-h-[60vh] overflow-auto rounded-lg bg-muted p-3 font-mono text-code leading-relaxed whitespace-pre">
              {csvText}
            </pre>
          )}
        </div>
        {csvText && (
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={handleDownload}>
              <Download data-icon="inline-start" />
              Download CSV
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}

const SOURCE_LABEL = { csv: "CSV", stripe: "Stripe" };

function sourceLabel(source) {
  if (!source) return "—";
  return SOURCE_LABEL[source] ?? source;
}

// importSearch is the /transactions search that lists one import batch.
function importSearch(importBatchId) {
  return { importBatchId, account: "", payee: "" };
}

const colHelper = createColumnHelper();

const importColumns = [
  colHelper.accessor("date", {
    id: "date",
    header: ({ column }) => (
      <TableSortHeader column={column}>Date</TableSortHeader>
    ),
    cell: ({ getValue }) => (
      <span className="whitespace-nowrap text-muted-foreground">{getValue()}</span>
    ),
    sortingFn: "alphanumeric",
    meta: { headerClass: "w-28" },
  }),
  colHelper.accessor((row) => row.source ?? "", {
    id: "source",
    header: ({ column }) => (
      <TableSortHeader column={column}>Source</TableSortHeader>
    ),
    // A real link so keyboard users can open the batch; the row click is a
    // larger target for the same destination.
    cell: ({ getValue, row }) => (
      <Link
        to="/transactions"
        search={importSearch(row.original.importBatchId)}
        onClick={(e) => e.stopPropagation()}
        className="touch-target font-medium text-foreground hover:underline"
      >
        {sourceLabel(getValue())}
      </Link>
    ),
    sortingFn: "alphanumeric",
  }),
  colHelper.accessor("importBatchId", {
    id: "importBatchId",
    header: "Batch",
    cell: ({ getValue }) => (
      <span className="font-mono text-[12px] text-faint">{getValue()}</span>
    ),
    meta: { headerClass: "hidden sm:table-cell", cellClass: "hidden sm:table-cell" },
  }),
  colHelper.accessor((row) => Number(row.transactionCount ?? 0), {
    id: "transactionCount",
    header: ({ column }) => (
      <TableSortHeader column={column} align="right">Transactions</TableSortHeader>
    ),
    cell: ({ getValue }) => <span className="tabular-nums">{getValue()}</span>,
    meta: { headerClass: "w-32 text-right", cellClass: "text-right" },
  }),
  colHelper.display({
    id: "actions",
    header: () => <span className="sr-only">Actions</span>,
    cell: ({ row, table }) => {
      const { onViewFile } = table.options.meta;
      const imp = row.original;
      return (
        <div className="flex items-center justify-end gap-1">
          {imp.source === "csv" && (
            <Button
              variant="ghost"
              size="xs"
              className="text-muted-foreground"
              onClick={(e) => {
                e.stopPropagation();
                onViewFile(imp.importBatchId);
              }}
              aria-label="View file"
            >
              <FileText data-icon="inline-start" />
              <span className="hidden sm:inline">View file</span>
            </Button>
          )}
          <ChevronRight aria-hidden="true" className="size-3.5 text-faint" />
        </div>
      );
    },
    meta: { headerClass: "w-14 sm:w-28" },
  }),
];

function ImportsTable({ imports, onViewFile }) {
  const router = useRouter();
  const [sorting, setSorting] = useState([{ id: "date", desc: true }]);
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 25 });

  const table = useReactTable({
    data: imports,
    columns: importColumns,
    state: { sorting, pagination },
    onSortingChange: setSorting,
    onPaginationChange: setPagination,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    meta: { onViewFile },
  });

  return (
    <>
      <div>
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id} className="hover:bg-transparent">
                {headerGroup.headers.map((header) => (
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
              <TableRow
                key={row.id}
                data-slot="import-row"
                className="cursor-pointer"
                title="Show these transactions"
                onClick={() =>
                  router.navigate({ to: "/transactions", search: importSearch(row.original.importBatchId) })
                }
              >
                {row.getVisibleCells().map((cell) => (
                  <TableCell key={cell.id} className={cn("py-1", cell.column.columnDef.meta?.cellClass)}>
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <TablePagination table={table} />
    </>
  );
}

export function ImportsHistoryPage() {
  const [viewingBatchId, setViewingBatchId] = useState(null);

  const { data, isLoading, error } = useQuery({
    queryKey: queryKeys.imports(),
    queryFn: () => ledgerClient.listImports({}),
  });

  const imports = data?.imports ?? [];

  return (
    <Page>
      <Section title="Past imports" description={imports.length > 0 ? imports.length : null}>
        {isLoading && <Loading />}
        {error && <ErrorBanner error={error} />}
        {!isLoading && !error && imports.length === 0 && (
          <EmptyState
            icon={PackageOpen}
            title="No imports yet"
            description="Use the Import page to bring in transactions."
          />
        )}
        {imports.length > 0 && (
          <ImportsTable
            imports={imports}
            onViewFile={setViewingBatchId}
          />
        )}
      </Section>

      <ImportFileDialog
        importBatchId={viewingBatchId}
        open={viewingBatchId !== null}
        onOpenChange={(open) => {
          if (!open) setViewingBatchId(null);
        }}
      />
    </Page>
  );
}
