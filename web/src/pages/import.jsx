import { useState } from "preact/hooks";
import { ledgerClient } from "../client.js";
import { useRpc } from "../hooks/use-rpc.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";

export function ImportPage() {
  const profiles = useRpc(() => ledgerClient.listBankProfiles({}), []);

  const [selectedProfile, setSelectedProfile] = useState("");
  const [csvFile, setCsvFile] = useState(null);
  const [candidates, setCandidates] = useState(null);
  const [csvData, setCsvData] = useState(null);
  const [previewing, setPreviewing] = useState(false);
  const [previewError, setPreviewError] = useState(null);
  const [selectedIndices, setSelectedIndices] = useState(new Set());
  const [importing, setImporting] = useState(false);
  const [importError, setImportError] = useState(null);
  const [importResult, setImportResult] = useState(null);

  async function handlePreview(e) {
    e.preventDefault();
    if (!csvFile || !selectedProfile) return;
    setPreviewError(null);
    setImportResult(null);
    setCandidates(null);
    setPreviewing(true);
    try {
      const bytes = await csvFile.arrayBuffer();
      const csvBytes = new Uint8Array(bytes);
      setCsvData(csvBytes);
      const res = await ledgerClient.previewImport({
        csvData: csvBytes,
        profileName: selectedProfile,
      });
      setCandidates(res.candidates);
      // Pre-select all non-duplicate candidates.
      const autoSelected = new Set();
      res.candidates.forEach((c, i) => {
        if (!c.isDuplicate) autoSelected.add(i);
      });
      setSelectedIndices(autoSelected);
    } catch (err) {
      setPreviewError(err);
    } finally {
      setPreviewing(false);
    }
  }

  function toggleCandidate(idx) {
    setSelectedIndices((prev) => {
      const next = new Set(prev);
      if (next.has(idx)) {
        next.delete(idx);
      } else {
        next.add(idx);
      }
      return next;
    });
  }

  function toggleAll() {
    if (!candidates) return;
    const allNew = candidates
      .map((c, i) => ({ c, i }))
      .filter(({ c }) => !c.isDuplicate)
      .map(({ i }) => i);
    if (selectedIndices.size === allNew.length) {
      setSelectedIndices(new Set());
    } else {
      setSelectedIndices(new Set(allNew));
    }
  }

  async function handleImport() {
    if (selectedIndices.size === 0 || !csvData || !selectedProfile) return;
    setImportError(null);
    setImporting(true);
    try {
      const res = await ledgerClient.importTransactions({
        candidateIndices: Array.from(selectedIndices),
        csvData,
        profileName: selectedProfile,
      });
      setImportResult(res);
      setCandidates(null);
      setCsvData(null);
      setCsvFile(null);
    } catch (err) {
      setImportError(err);
    } finally {
      setImporting(false);
    }
  }

  const newCount = candidates ? candidates.filter((c) => !c.isDuplicate).length : 0;

  return (
    <div class="space-y-6">
      <h2 class="text-2xl font-bold">Import Transactions</h2>

      {/* Upload form */}
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h3 class="card-title text-base">Upload CSV</h3>
          <form onSubmit={handlePreview} class="flex flex-wrap gap-3 items-end">
            <label class="form-control w-full sm:w-56">
              <div class="label"><span class="label-text">Bank Profile</span></div>
              {profiles.loading ? (
                <select class="select select-bordered select-sm" disabled>
                  <option>Loading…</option>
                </select>
              ) : (
                <select
                  class="select select-bordered select-sm"
                  value={selectedProfile}
                  onChange={(e) => setSelectedProfile(e.target.value)}
                  required
                >
                  <option value="">Select profile…</option>
                  {(profiles.data?.profiles ?? []).map((p) => (
                    <option key={p.name} value={p.name}>{p.name}</option>
                  ))}
                </select>
              )}
            </label>
            <label class="form-control w-full sm:w-auto flex-1">
              <div class="label"><span class="label-text">CSV File</span></div>
              <input
                type="file"
                accept=".csv,text/csv"
                class="file-input file-input-bordered file-input-sm"
                onChange={(e) => setCsvFile(e.target.files[0] || null)}
                required
              />
            </label>
            <button
              type="submit"
              class="btn btn-primary btn-sm"
              disabled={previewing || !csvFile || !selectedProfile}
            >
              {previewing ? "Previewing…" : "Preview"}
            </button>
          </form>
          {previewError && <ErrorBanner error={previewError} />}
          {profiles.error && <ErrorBanner error={profiles.error} />}
        </div>
      </div>

      {/* Import result */}
      {importResult && (
        <div class="alert alert-success">
          Imported {importResult.importedCount} transaction(s) successfully.
        </div>
      )}
      {importError && <ErrorBanner error={importError} />}

      {/* Preview table */}
      {candidates && (
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body">
            <div class="flex items-center justify-between gap-4 flex-wrap">
              <h3 class="card-title text-base">
                Preview — {candidates.length} transaction(s), {newCount} new
              </h3>
              <div class="flex gap-2">
                <button class="btn btn-ghost btn-sm" onClick={toggleAll}>
                  {selectedIndices.size === newCount ? "Deselect All" : "Select All New"}
                </button>
                <button
                  class="btn btn-primary btn-sm"
                  onClick={handleImport}
                  disabled={importing || selectedIndices.size === 0}
                >
                  {importing ? "Importing…" : `Import ${selectedIndices.size} Selected`}
                </button>
              </div>
            </div>

            <div class="overflow-x-auto">
              <table class="table table-sm">
                <thead>
                  <tr>
                    <th></th>
                    <th>Status</th>
                    <th>Date</th>
                    <th>Description</th>
                    <th>Postings</th>
                    <th>Rule</th>
                  </tr>
                </thead>
                <tbody>
                  {candidates.map((c, i) => (
                    <tr key={i} class={c.isDuplicate ? "opacity-50" : ""}>
                      <td>
                        <input
                          type="checkbox"
                          class="checkbox checkbox-sm"
                          checked={selectedIndices.has(i)}
                          disabled={c.isDuplicate}
                          onChange={() => toggleCandidate(i)}
                        />
                      </td>
                      <td>
                        <span class={`badge badge-sm ${c.isDuplicate ? "badge-neutral" : "badge-success"}`}>
                          {c.isDuplicate ? "DUP" : "NEW"}
                        </span>
                      </td>
                      <td class="whitespace-nowrap">{c.transaction?.date}</td>
                      <td>{c.transaction?.description}</td>
                      <td class="text-xs">
                        {(c.transaction?.postings ?? []).map((p, j) => (
                          <div key={j}>
                            {p.account}
                            {p.amounts?.[0] && (
                              <span class="text-base-content/60 ml-1">
                                {p.amounts[0].commodity}{p.amounts[0].quantity}
                              </span>
                            )}
                          </div>
                        ))}
                      </td>
                      <td class="text-xs text-base-content/60">
                        {c.matchedRuleId || "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
