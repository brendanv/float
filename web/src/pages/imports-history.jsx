import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { PackageOpen, FileText } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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

export function ImportsHistoryPage() {
  const router = useRouter();
  const [viewingBatchId, setViewingBatchId] = useState(null);

  const { data, isLoading, error } = useQuery({
    queryKey: queryKeys.imports(),
    queryFn: () => ledgerClient.listImports({}),
  });

  const imports = data?.imports ?? [];

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title="Import History" />

      <Card>
        <CardHeader>
          <CardTitle>Past Imports</CardTitle>
        </CardHeader>
        <CardContent>
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
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Date</TableHead>
                  <TableHead>Batch ID</TableHead>
                  <TableHead>Transactions</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {imports.map((imp) => (
                  <TableRow
                    key={imp.importBatchId}
                    className="cursor-pointer hover:bg-muted/50"
                    onClick={() => router.navigate({ to: "/transactions", search: { importBatchId: imp.importBatchId, account: "", payee: "" } })}
                  >
                    <TableCell className="whitespace-nowrap font-mono text-sm">{imp.date}</TableCell>
                    <TableCell className="font-mono text-sm">{imp.importBatchId}</TableCell>
                    <TableCell>{imp.transactionCount}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-2">
                        {imp.source === "csv" && (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-7 gap-1 px-2 text-xs text-muted-foreground hover:text-foreground"
                            onClick={(e) => {
                              e.stopPropagation();
                              setViewingBatchId(imp.importBatchId);
                            }}
                          >
                            <FileText size={13} />
                            View file
                          </Button>
                        )}
                        <span className="text-muted-foreground text-xs">View →</span>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <ImportFileDialog
        importBatchId={viewingBatchId}
        open={viewingBatchId !== null}
        onOpenChange={(open) => { if (!open) setViewingBatchId(null); }}
      />
    </div>
  );
}
