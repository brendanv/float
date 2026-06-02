import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import {
  useReactTable,
  getCoreRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  createColumnHelper,
  flexRender,
} from "@tanstack/react-table";
import { PackageOpen, FileText } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { TableSortHeader } from "../components/table-sort-header.jsx";
import { TablePagination } from "../components/table-pagination.jsx";
import { Page, PageCard } from "../components/page.jsx";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
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

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="xl" showCloseButton>
        <DialogHeader>
          <DialogTitle className="font-mono text-sm">
            {data?.filename ?? importBatchId + ".csv"}
          </DialogTitle>
        </DialogHeader>
        <div className="mt-2 min-w-0 overflow-hidden">
          {isLoading && <Loading />}
          {error && <ErrorBanner error={error} />}
          {csvText && (
            <pre className="max-h-[60vh] overflow-x-auto overflow-y-auto rounded-md bg-muted p-4 text-xs font-mono whitespace-pre leading-relaxed">
              {csvText}
            </pre>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

const colHelper = createColumnHelper();

const importColumns = [
  colHelper.accessor("date", {
    id: "date",
    header: ({ column }) => (
      <TableSortHeader column={column}>Date</TableSortHeader>
    ),
    cell: ({ getValue }) => (
      <span className="whitespace-nowrap font-mono text-sm">{getValue()}</span>
    ),
    sortingFn: "alphanumeric",
  }),
  colHelper.accessor("importBatchId", {
    id: "importBatchId",
    header: "Batch ID",
    cell: ({ getValue }) => (
      <span className="font-mono text-sm">{getValue()}</span>
    ),
  }),
  colHelper.accessor((row) => Number(row.transactionCount ?? 0), {
    id: "transactionCount",
    header: ({ column }) => (
      <TableSortHeader column={column}>Transactions</TableSortHeader>
    ),
    cell: ({ getValue }) => getValue(),
  }),
  colHelper.display({
    id: "actions",
    header: "",
    cell: ({ row, table }) => {
      const { onViewFile } = table.options.meta;
      const imp = row.original;
      return (
        <div className="flex items-center justify-end gap-2">
          {imp.source === "csv" && (
            <Button
              variant="ghost"
              size="sm"
              className="text-muted-foreground hover:text-foreground"
              onClick={(e) => {
                e.stopPropagation();
                onViewFile(imp.importBatchId);
              }}
            >
              <FileText data-icon="inline-start" />
              View file
            </Button>
          )}
          <span className="text-xs text-muted-foreground">View</span>
        </div>
      );
    },
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
      <div className="overflow-x-auto">
      <Table>
        <TableHeader>
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
              {headerGroup.headers.map((header) => (
                <TableHead key={header.id}>
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
              className="cursor-pointer hover:bg-muted/50"
              onClick={() =>
                router.navigate({
                  to: "/transactions",
                  search: {
                    importBatchId: row.original.importBatchId,
                    account: "",
                    payee: "",
                  },
                })
              }
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
      <PageHeader title="Import History" />

      <PageCard title="Past Imports">
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
      </PageCard>

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
