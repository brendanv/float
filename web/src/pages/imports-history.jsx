import { useQuery } from "@tanstack/react-query";
import { useRouter, useParams } from "@tanstack/react-router";
import { ArrowLeft, PackageOpen } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

function ImportTransactionsList({ batchId }) {
  const router = useRouter();

  const { data, isLoading, error } = useQuery({
    queryKey: queryKeys.importedTransactions(batchId),
    queryFn: () => ledgerClient.getImportedTransactions({ importBatchId: batchId }),
    enabled: !!batchId,
  });

  const txns = data?.transactions ?? [];

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => router.navigate({ to: "/imports" })}
          className="gap-1.5"
        >
          <ArrowLeft size={16} />
          All Imports
        </Button>
        <h2 className="text-2xl font-bold">Import {batchId}</h2>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>
            {isLoading ? "Loading…" : `${txns.length} transaction(s)`}
          </CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading && <Loading />}
          {error && <ErrorBanner error={error} />}
          {!isLoading && !error && txns.length === 0 && (
            <p className="text-sm text-muted-foreground">No transactions found for this import.</p>
          )}
          {txns.length > 0 && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Date</TableHead>
                  <TableHead>Description</TableHead>
                  <TableHead>Postings</TableHead>
                  <TableHead>Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {txns.map((t) => (
                  <TableRow key={t.fid || t.description + t.date}>
                    <TableCell className="whitespace-nowrap font-mono text-sm">{t.date}</TableCell>
                    <TableCell>{t.description}</TableCell>
                    <TableCell className="text-xs">
                      {(t.postings ?? []).map((p, i) => (
                        <div key={i}>
                          {p.account}
                          {p.amounts?.[0] && (
                            <span className="ml-1 text-muted-foreground">
                              {p.amounts[0].commodity}{p.amounts[0].quantity}
                            </span>
                          )}
                        </div>
                      ))}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {t.status || "Unmarked"}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

export function ImportsHistoryPage() {
  const router = useRouter();

  const { data, isLoading, error } = useQuery({
    queryKey: queryKeys.imports(),
    queryFn: () => ledgerClient.listImports({}),
  });

  const imports = data?.imports ?? [];

  return (
    <div className="space-y-6">
      <h2 className="text-2xl font-bold">Import History</h2>

      <Card>
        <CardHeader>
          <CardTitle>Past Imports</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading && <Loading />}
          {error && <ErrorBanner error={error} />}
          {!isLoading && !error && imports.length === 0 && (
            <div className="flex flex-col items-center gap-2 py-8 text-muted-foreground">
              <PackageOpen size={32} />
              <p className="text-sm">No imports yet. Use the Import page to bring in transactions.</p>
            </div>
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
                    onClick={() => router.navigate({ to: "/imports/$batchId", params: { batchId: imp.importBatchId } })}
                  >
                    <TableCell className="whitespace-nowrap font-mono text-sm">{imp.date}</TableCell>
                    <TableCell className="font-mono text-sm">{imp.importBatchId}</TableCell>
                    <TableCell>{imp.transactionCount}</TableCell>
                    <TableCell className="text-right text-muted-foreground text-xs">View →</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

export function ImportDetailPage() {
  const { batchId } = useParams({ strict: false });
  return <ImportTransactionsList batchId={batchId} />;
}
