import { useState } from "preact/hooks";
import { ledgerClient } from "../client.js";
import { useRpc } from "../hooks/use-rpc.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";

function shortHash(hash) {
  return hash.slice(0, 8);
}

function formatDate(ts) {
  if (!ts) return "";
  return new Date(ts).toLocaleString();
}

export function SnapshotsPage() {
  const snapshots = useRpc(() => ledgerClient.listSnapshots({ limit: 50 }), []);
  const [restoring, setRestoring] = useState(null);
  const [actionError, setActionError] = useState(null);
  const [successMsg, setSuccessMsg] = useState(null);

  async function handleRestore(hash) {
    const confirmed = window.confirm(
      `Restore to snapshot ${shortHash(hash)}?\n\nThis will discard all changes made after this snapshot. This action cannot be undone.`
    );
    if (!confirmed) return;

    setRestoring(hash);
    setActionError(null);
    setSuccessMsg(null);
    try {
      await ledgerClient.restoreSnapshot({ hash });
      setSuccessMsg(`Restored to ${shortHash(hash)}. Reload the page to see the updated data.`);
      snapshots.refetch();
    } catch (err) {
      setActionError(err);
    } finally {
      setRestoring(null);
    }
  }

  return (
    <div class="space-y-6">
      <h2 class="text-2xl font-bold">Change History</h2>
      <p class="text-base-content/60 text-sm">
        Each successful write creates a snapshot. You can restore to any previous state.
      </p>

      {actionError && <ErrorBanner error={actionError} />}
      {successMsg && (
        <div class="alert alert-success">
          <span>{successMsg}</span>
        </div>
      )}

      <div class="card bg-base-100 shadow-sm">
        <div class="card-body p-0">
          {snapshots.loading && <div class="p-4"><Loading /></div>}
          {snapshots.error && <div class="p-4"><ErrorBanner error={snapshots.error} /></div>}
          {snapshots.data && (
            snapshots.data.snapshots?.length > 0 ? (
              <div class="overflow-x-auto">
                <table class="table table-sm">
                  <thead>
                    <tr>
                      <th>Hash</th>
                      <th>When</th>
                      <th>Message</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {snapshots.data.snapshots.map((s, i) => (
                      <tr key={s.hash} class={i === 0 ? "font-semibold" : ""}>
                        <td class="font-mono text-xs">{shortHash(s.hash)}</td>
                        <td class="text-sm whitespace-nowrap">{formatDate(s.timestamp)}</td>
                        <td class="text-sm">{s.message?.trim()}</td>
                        <td>
                          {i > 0 && (
                            <button
                              class="btn btn-ghost btn-xs text-warning"
                              disabled={restoring === s.hash}
                              onClick={() => handleRestore(s.hash)}
                            >
                              {restoring === s.hash ? "Restoring…" : "Restore"}
                            </button>
                          )}
                          {i === 0 && (
                            <span class="badge badge-neutral badge-sm">current</span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <p class="p-4 text-base-content/50 text-sm">No snapshots yet. Snapshots are created automatically on each write.</p>
            )
          )}
        </div>
      </div>
    </div>
  );
}
