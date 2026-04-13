import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";

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
    <div class="space-y-6">
      <h2 class="text-2xl font-bold">Snapshots</h2>

      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h3 class="card-title text-base">History</h3>
          {error && <ErrorBanner error={error} />}
          {isLoading && <Loading />}
          {fetchError && <ErrorBanner error={fetchError} />}
          {snapshotsData && (
            snapshotsData.snapshots?.length > 0 ? (
              <div class="overflow-x-auto">
                <table class="table table-sm">
                  <thead>
                    <tr>
                      <th>Hash</th>
                      <th>Date</th>
                      <th>Message</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {snapshotsData.snapshots.map((s) => (
                      <tr key={s.hash}>
                        <td class="font-mono">{s.hash.slice(0, 12)}</td>
                        <td class="font-mono">{s.timestamp}</td>
                        <td>{s.message}</td>
                        <td>
                          <button
                            class="btn btn-ghost btn-xs text-warning"
                            disabled={restoring === s.hash}
                            onClick={() => handleRestore(s.hash)}
                          >
                            {restoring === s.hash ? "Restoring…" : "Restore"}
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <p class="text-base-content/50 text-sm">No snapshots yet.</p>
            )
          )}
        </div>
      </div>
    </div>
  );
}
