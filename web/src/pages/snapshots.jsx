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
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

function ChangeBadge({ type }) {
  const styles = {
    added:    "bg-success/15 text-success",
    deleted:  "bg-destructive/15 text-destructive",
    modified: "bg-muted text-foreground",
    renamed:  "bg-warning/15 text-warning",
  };
  return (
    <span
      className={`rounded px-1.5 py-0.5 text-[10px] uppercase tracking-wide ${styles[type] ?? styles.modified}`}
    >
      {type || "modified"}
    </span>
  );
}

function DiffLine({ line }) {
  let cls = "block px-3";
  if (line.startsWith("+++") || line.startsWith("---")) cls += " text-muted-foreground";
  else if (line.startsWith("@@")) cls += " bg-muted/60 text-muted-foreground";
  else if (line.startsWith("+")) cls += " bg-success/10 text-success";
  else if (line.startsWith("-")) cls += " bg-destructive/10 text-destructive";
  return <span className={cls}>{line || " "}</span>;
}

function FileDiffBlock({ file }) {
  const label =
    file.changeType === "renamed" && file.oldPath
      ? `${file.oldPath} → ${file.path}`
      : file.path;
  return (
    <div className="rounded-md border">
      <div className="flex items-center gap-2 border-b bg-muted/40 px-3 py-2 text-xs font-mono">
        <ChangeBadge type={file.changeType} />
        <span className="truncate">{label}</span>
      </div>
      {file.isBinary ? (
        <p className="px-3 py-2 text-xs text-muted-foreground">
          Binary file (no diff shown).
        </p>
      ) : file.patch ? (
        <pre className="overflow-x-auto p-0 py-1 text-xs font-mono leading-relaxed">
          {file.patch.split("\n").map((line, i) => (
            <DiffLine key={i} line={line} />
          ))}
        </pre>
      ) : (
        <p className="px-3 py-2 text-xs text-muted-foreground">No content changes.</p>
      )}
    </div>
  );
}

function SnapshotDiffDialog({ hash, open, onOpenChange }) {
  const { data, isLoading, error } = useQuery({
    queryKey: queryKeys.snapshotDiff(hash),
    queryFn: () => ledgerClient.getSnapshotDiff({ hash }),
    enabled: open && !!hash,
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[90vw] max-w-5xl sm:max-w-5xl" showCloseButton>
        <DialogHeader>
          <DialogTitle className="font-mono text-sm">
            Diff {hash?.slice(0, 12)}
          </DialogTitle>
        </DialogHeader>
        <div className="mt-2 flex max-h-[70vh] min-w-0 flex-col gap-4 overflow-y-auto">
          {isLoading && <Loading />}
          {error && <ErrorBanner error={error} />}
          {data && data.files?.length === 0 && (
            <p className="text-sm text-muted-foreground">No changes.</p>
          )}
          {data?.files?.map((f, i) => (
            <FileDiffBlock key={`${f.oldPath ?? ""}:${f.path}:${i}`} file={f} />
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}

export function SnapshotsPage() {
  const queryClient = useQueryClient();

  const { data: snapshotsData, isLoading, error: fetchError } = useQuery({
    queryKey: queryKeys.snapshots(),
    queryFn: () => ledgerClient.listSnapshots({}),
  });

  const [restoring, setRestoring] = useState(null);
  const [error, setError] = useState(null);
  const [viewingHash, setViewingHash] = useState(null);

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
                      <TableCell className="text-right">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="xs"
                            onClick={() => setViewingHash(s.hash)}
                          >
                            View diff
                          </Button>
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
                        </div>
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

      <SnapshotDiffDialog
        hash={viewingHash}
        open={viewingHash !== null}
        onOpenChange={(open) => { if (!open) setViewingHash(null); }}
      />
    </div>
  );
}
