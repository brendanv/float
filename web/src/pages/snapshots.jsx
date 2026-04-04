import { useState } from "preact/hooks";
import { ledgerClient } from "../client.js";
import { useRpc } from "../hooks/use-rpc.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";

export function SnapshotsPage() {
  const snapshots = useRpc(() => ledgerClient.listSnapshots({}), []);
  const [restoring, setRestoring] = useState(null);
  const [error, setError] = useState(null);

  async function handleRestore(hash) {
    if (!confirm(`Restore to snapshot ${hash.slice(0, 12)}? This will revert all journal files to that point in time.`)) {
      return;
    }
    setError(null);
    setRestoring(hash);
    try {
      await ledgerClient.restoreSnapshot({ hash });
      snapshots.refetch();
    } catch (err) {
      setError(err);
    } finally {
      setRestoring(null);
    }
  }

  return (
    <div class="space-y-6">
      <h2 class="text-2xl font-bold">Snapshots</h2>

      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h3 class="card-title text-base">History</h3>
          {error && <ErrorBanner error={error} />}
          {snapshots.loading && <Loading />}
          {snapshots.error && <ErrorBanner error={snapshots.error} />}
          {snapshots.data && (
            snapshots.data.snapshots?.length > 0 ? (
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
                    {snapshots.data.snapshots.map((s) => (
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
