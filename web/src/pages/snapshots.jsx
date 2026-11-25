import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys, invalidateLedgerQueries } from "../query-keys.js";
import { FileDiff, History, RotateCcw } from "lucide-react";
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
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { cn, ROW_ACTION } from "@/lib/utils";

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

// RestoreDialog confirms a restore. By default the restore is recorded as a
// new snapshot (nothing is lost); the hard-reset option instead discards the
// `newerCount` snapshots above the target from history.
function RestoreDialog({ snapshot, newerCount, onOpenChange, onRestore, restoring }) {
  const [hardReset, setHardReset] = useState(false);
  const open = snapshot !== null;
  const plural = newerCount === 1 ? "" : "s";
  // Every confirmation starts from the safe default.
  useEffect(() => {
    if (open) setHardReset(false);
  }, [open]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>Restore this snapshot?</DialogTitle>
          <DialogDescription>
            Your data goes back to how it was after &ldquo;{snapshot?.message?.trim()}&rdquo;
            {snapshot && <> (<Timestamp value={snapshot.timestamp} absolute />)</>}.
            {" "}The restore is saved as a new snapshot, so this history stays intact.
          </DialogDescription>
        </DialogHeader>
        <Label className="items-start gap-2.5 font-normal leading-snug">
          <Checkbox
            className="mt-0.5"
            checked={hardReset}
            onCheckedChange={(checked) => setHardReset(!!checked)}
            disabled={restoring}
          />
          <span>
            <span className="block font-medium">Discard later history</span>
            <span className="block text-muted-foreground">
              Hard reset instead, permanently deleting the {newerCount} newer snapshot{plural}. This can&rsquo;t be undone.
            </span>
          </span>
        </Label>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={restoring}>
            Cancel
          </Button>
          <Button
            variant={hardReset ? "destructive" : "default"}
            isLoading={restoring}
            loadingText={hardReset ? "Resetting…" : "Restoring…"}
            onClick={() => onRestore({ hash: snapshot.hash, hardReset })}
          >
            {hardReset ? "Hard reset" : "Restore"}
          </Button>
        </DialogFooter>
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

  const [confirming, setConfirming] = useState(null);
  const [error, setError] = useState(null);
  const [viewingHash, setViewingHash] = useState(null);

  const restoreMutation = useMutation({
    mutationFn: ({ hash, hardReset }) => ledgerClient.restoreSnapshot({ hash, hardReset }),
    onSuccess: () => {
      setConfirming(null);
      invalidateLedgerQueries(queryClient);
    },
    onError: (err) => {
      setError(err);
      setConfirming(null);
    },
  });

  const snapshots = snapshotsData?.snapshots ?? [];
  const confirmingIndex = confirming ? snapshots.findIndex((s) => s.hash === confirming.hash) : -1;

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
                {snapshots.map((s, i) => (
                  <TableRow key={s.hash} data-slot="snapshot-row" className="group/row">
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
                      <div className={cn("flex items-center justify-end gap-1", ROW_ACTION)}>
                        <Button
                          variant="ghost"
                          size="xs"
                          onClick={() => setViewingHash(s.hash)}
                          aria-label="View diff"
                        >
                          <FileDiff data-icon="inline-start" className="sm:hidden" />
                          <span className="max-sm:hidden">View diff</span>
                        </Button>
                        {i === 0 ? (
                          // The newest snapshot is the current state; restoring it is a no-op.
                          <span className="hidden w-[4.5rem] text-center text-control text-faint sm:inline-block">Current</span>
                        ) : (
                          <Button
                            variant="ghost"
                            size="xs"
                            className="text-muted-foreground"
                            onClick={() => { setError(null); setConfirming(s); }}
                            aria-label="Restore"
                          >
                            <RotateCcw data-icon="inline-start" className="sm:hidden" />
                            <span className="max-sm:hidden">Restore</span>
                          </Button>
                        )}
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

      <RestoreDialog
        snapshot={confirming}
        newerCount={Math.max(confirmingIndex, 0)}
        restoring={restoreMutation.isPending}
        onOpenChange={(open) => { if (!open && !restoreMutation.isPending) setConfirming(null); }}
        onRestore={(vars) => restoreMutation.mutate(vars)}
      />

      <SnapshotDiffDialog
        hash={viewingHash}
        open={viewingHash !== null}
        onOpenChange={(open) => { if (!open) setViewingHash(null); }}
      />
    </Page>
  );
}
