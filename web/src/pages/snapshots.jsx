import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys, invalidateLedgerQueries } from "../query-keys.js";
import { History } from "lucide-react";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { Page, Section } from "../components/page.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { Timestamp } from "../components/timestamp.jsx";
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
import { cn } from "@/lib/utils";

const CHANGE_TONE = {
  added: "text-inflow",
  deleted: "text-below",
  modified: "text-muted-foreground",
  renamed: "text-warning",
};

// ChangeLabel is a file's change type in the diff header, toned like its
// lines (added = inflow, deleted = below).
function ChangeLabel({ type }) {
  return (
    <span className={cn("text-label uppercase", CHANGE_TONE[type] ?? CHANGE_TONE.modified)}>
      {type || "modified"}
    </span>
  );
}

// DiffLine renders one unified-diff line: added lines in inflow, removed
// lines in below (each on its soft tint), hunk headers in ink-3.
function DiffLine({ line }) {
  let cls = "block px-3";
  if (line.startsWith("+++") || line.startsWith("---")) cls += " text-faint";
  else if (line.startsWith("@@")) cls += " text-faint";
  else if (line.startsWith("+")) cls += " bg-inflow-soft text-inflow";
  else if (line.startsWith("-")) cls += " bg-below-soft text-below";
  return <span className={cls}>{line || " "}</span>;
}

function FileDiffBlock({ file }) {
  const label =
    file.changeType === "renamed" && file.oldPath
      ? `${file.oldPath} → ${file.path}`
      : file.path;
  return (
    <div data-slot="file-diff" className="min-w-0 overflow-hidden rounded-lg border border-border">
      <div className="flex items-center gap-2.5 border-b border-border bg-muted px-3 py-1.5">
        <span className="min-w-0 truncate font-mono text-code">{label}</span>
        <span className="ml-auto shrink-0"><ChangeLabel type={file.changeType} /></span>
      </div>
      {file.isBinary ? (
        <p className="px-3 py-2 text-control text-muted-foreground">
          Binary file (no diff shown).
        </p>
      ) : file.patch ? (
        <pre className="overflow-x-auto py-1 font-mono text-code leading-relaxed">
          {file.patch.split("\n").map((line, i) => (
            <DiffLine key={i} line={line} />
          ))}
        </pre>
      ) : (
        <p className="px-3 py-2 text-control text-muted-foreground">No content changes.</p>
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
      <DialogContent size="xl" showCloseButton>
        <DialogHeader>
          <DialogTitle>
            Snapshot <span className="font-mono text-code font-normal text-muted-foreground">{hash?.slice(0, 12)}</span>
          </DialogTitle>
        </DialogHeader>
        <div className="max-h-[70vh] min-w-0 space-y-3 overflow-y-auto">
          {isLoading && <Loading />}
          {error && <ErrorBanner error={error} />}
          {data && data.files?.length === 0 && (
            <p className="text-control text-muted-foreground">No changes.</p>
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
      invalidateLedgerQueries(queryClient);
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

  const snapshots = snapshotsData?.snapshots ?? [];

  return (
    <Page>
      <Section title="History" description={snapshots.length > 0 ? snapshots.length : null}>
        {error && <ErrorBanner error={error} />}
        {isLoading && <Loading />}
        {fetchError && <ErrorBanner error={fetchError} />}
        {snapshotsData && (
          snapshots.length > 0 ? (
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead>Date</TableHead>
                  <TableHead>Message</TableHead>
                  <TableHead className="hidden md:table-cell">Snapshot</TableHead>
                  <TableHead><span className="sr-only">Actions</span></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {snapshots.map((s) => (
                  <TableRow key={s.hash} data-slot="snapshot-row">
                    <TableCell className="py-1 whitespace-nowrap text-muted-foreground">
                      <Timestamp value={s.timestamp} absolute />
                    </TableCell>
                    <TableCell className="py-1">
                      <div className="max-w-[10rem] truncate sm:max-w-md" title={s.message}>{s.message}</div>
                    </TableCell>
                    <TableCell className="hidden py-1 md:table-cell">
                      <span className="font-mono text-[12px] text-faint">{s.hash.slice(0, 8)}</span>
                    </TableCell>
                    <TableCell className="py-1 text-right">
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
                          className="text-warning hover:text-warning"
                          isLoading={restoring === s.hash}
                          loadingText="Restoring…"
                          onClick={() => handleRestore(s.hash)}
                        >
                          Restore
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <EmptyState
              icon={History}
              title="No snapshots yet"
              description="Snapshots are created automatically every time you write to the journal."
            />
          )
        )}
      </Section>

      <SnapshotDiffDialog
        hash={viewingHash}
        open={viewingHash !== null}
        onOpenChange={(open) => { if (!open) setViewingHash(null); }}
      />
    </Page>
  );
}
