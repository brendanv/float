import { useQuery } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { PackageOpen } from "lucide-react";
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

export function ImportsHistoryPage() {
  const router = useRouter();

  const { data, isLoading, error } = useQuery({
    queryKey: queryKeys.imports(),
    queryFn: () => ledgerClient.listImports({}),
  });

  const imports = data?.imports ?? [];

  return (
    <div className="flex flex-col gap-6">
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
                    onClick={() => router.navigate({ to: "/transactions", search: { importBatchId: imp.importBatchId, account: "", payee: "" } })}
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
