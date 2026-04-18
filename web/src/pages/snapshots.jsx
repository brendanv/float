import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
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
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";

export function SnapshotsPage() {
  const queryClient = useQueryClient();

  const { data: snapshotsData, isLoading, error: fetchError } = useQuery({
    queryKey: queryKeys.snapshots(),
    queryFn: () => ledgerClient.listSnapshots({}),
  });

  const [restoring, setRestoring] = useState(null);
  const [error, setError] = useState(null);

  const restoreMutation = useMutation({
    mutationFn: ({ hash }) => ledgerClient.restoreSnapshot({ hash }),
    onSuccess: () => {
      setRestoring(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.snapshots() });
    },
    onError: (err) => {
      setError(err);
      setRestoring(null);
    },
  });

  function handleRestore(hash) {
    if (!confirm(`Restore to snapshot ${hash.slice(0, 12)}? This will revert all journal files to that point in time.`)) {
      return;
    }
    setError(null);
    setRestoring(hash);
    restoreMutation.mutate({ hash });
  }

  return (
    <div className="flex flex-col gap-6">
      <h2 className="text-2xl font-bold">Snapshots</h2>

      <Card>
        <CardHeader>
          <CardTitle>History</CardTitle>
        </CardHeader>
        <CardContent>
          {error && <ErrorBanner error={error} />}
          {isLoading && <Loading />}
          {fetchError && <ErrorBanner error={fetchError} />}
          {snapshotsData && (
            snapshotsData.snapshots?.length > 0 ? (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Hash</TableHead>
                    <TableHead>Date</TableHead>
                    <TableHead>Message</TableHead>
                    <TableHead></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {snapshotsData.snapshots.map((s) => (
                    <TableRow key={s.hash}>
                      <TableCell className="font-mono">{s.hash.slice(0, 12)}</TableCell>
                      <TableCell className="font-mono">{s.timestamp}</TableCell>
                      <TableCell>{s.message}</TableCell>
                      <TableCell>
                        <Button
                          variant="ghost"
                          size="xs"
                          className="text-warning"
                          disabled={restoring === s.hash}
                          onClick={() => handleRestore(s.hash)}
                        >
                          {restoring === s.hash && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
                          {restoring === s.hash ? "Restoring…" : "Restore"}
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            ) : (
              <p className="text-sm text-muted-foreground">No snapshots yet.</p>
            )
          )}
        </CardContent>
      </Card>
    </div>
  );
}
