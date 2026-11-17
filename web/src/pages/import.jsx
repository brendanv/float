import { useState, useMemo, useRef, useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Plus, Pencil, Trash2, Upload, ClipboardPaste } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys, invalidateLedgerQueries } from "../query-keys.js";
import { ErrorBanner } from "../components/error-banner.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { Page, Section } from "../components/page.jsx";
import { Segmented } from "../components/segmented.jsx";
import { CandidateTable } from "../components/import/candidate-table.jsx";
import {
  CreateProfileModal,
  DeleteProfileDialog,
  EditProfileModal,
} from "../components/import/profile-dialogs.jsx";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Notice } from "@/components/ui/alert";
import { Form, FormField, FormActions } from "@/components/ui/form";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

const RULE_FILTERS = [
  { value: "all", label: "All" },
  { value: "matched", label: "Rule matched" },
  { value: "unmatched", label: "No rule" },
];

function plural(n, one, many = `${one}s`) {
  return `${n} ${n === 1 ? one : many}`;
}

// ProfileIconButton is a ghost icon button with a tooltip, beside the
// bank-profile select.
function ProfileIconButton({ label, onClick, children }) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button type="button" variant="ghost" size="icon-sm" onClick={onClick} aria-label={label}>
            {children}
          </Button>
        }
      />
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

export function ImportPage() {
  const queryClient = useQueryClient();

  const { data: profilesData, isLoading: profilesLoading, error: profilesError } = useQuery({
    queryKey: queryKeys.bankProfiles(),
    queryFn: () => ledgerClient.listBankProfiles({}),
  });

  const fileInputRef = useRef(null);
  const [isDragging, setIsDragging] = useState(false);
  const [selectedProfile, setSelectedProfile] = useState("");
  const [csvFile, setCsvFile] = useState(null);
  const [showPasteInput, setShowPasteInput] = useState(false);
  const [csvPasteText, setCsvPasteText] = useState("");
  const [candidates, setCandidates] = useState(null);
  const [csvData, setCsvData] = useState(null);
  const [previewing, setPreviewing] = useState(false);
  const [previewError, setPreviewError] = useState(null);
  const [selectedIndices, setSelectedIndices] = useState(new Set());
  const [sorting, setSorting] = useState([{ id: "date", desc: true }]);
  const [ruleFilter, setRuleFilter] = useState("all");
  const [importing, setImporting] = useState(false);
  const [importProgress, setImportProgress] = useState(null);
  const [importError, setImportError] = useState(null);
  const [importResult, setImportResult] = useState(null);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [editingProfile, setEditingProfile] = useState(null);
  const [deletingProfile, setDeletingProfile] = useState(null);

  const sortedProfiles = useMemo(() =>
    [...(profilesData?.profiles ?? [])].sort((a, b) =>
      a.name.localeCompare(b.name, undefined, { sensitivity: "base" })
    ),
    [profilesData?.profiles]
  );

  function handleProfileUpdated(profile) {
    queryClient.invalidateQueries({ queryKey: queryKeys.bankProfiles() });
    if (selectedProfile === editingProfile?.name) {
      setSelectedProfile(profile.name);
    }
    setEditingProfile(null);
  }

  function handleProfileDeleted(name) {
    queryClient.invalidateQueries({ queryKey: queryKeys.bankProfiles() });
    if (selectedProfile === name) setSelectedProfile("");
    setDeletingProfile(null);
  }

  async function handlePreview(e) {
    e.preventDefault();
    if ((!csvFile && !csvPasteText) || !selectedProfile) return;
    setPreviewError(null);
    setImportResult(null);
    setCandidates(null);
    setPreviewing(true);
    try {
      let csvBytes;
      if (showPasteInput && csvPasteText) {
        csvBytes = new TextEncoder().encode(csvPasteText);
      } else {
        const bytes = await csvFile.arrayBuffer();
        csvBytes = new Uint8Array(bytes);
      }
      setCsvData(csvBytes);
      const res = await ledgerClient.previewImport({
        csvData: csvBytes,
        profileName: selectedProfile,
      });
      setCandidates(res.candidates);
      setSorting([]);
      setRuleFilter("all");
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

  const toggleCandidate = useCallback((idx) => {
    setSelectedIndices((prev) => {
      const next = new Set(prev);
      if (next.has(idx)) {
        next.delete(idx);
      } else {
        next.add(idx);
      }
      return next;
    });
  }, []);

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
    setImportProgress({ imported: 0, total: selectedIndices.size });
    try {
      for await (const res of ledgerClient.importTransactions({
        candidateIndices: Array.from(selectedIndices),
        csvData,
        profileName: selectedProfile,
      })) {
        if (res.payload.case === "progress") {
          setImportProgress({ imported: res.payload.value.imported, total: res.payload.value.total });
        } else if (res.payload.case === "result") {
          setImportResult(res.payload.value);
          invalidateLedgerQueries(queryClient);
          setCandidates(null);
          setCsvData(null);
          setCsvFile(null);
        }
      }
    } catch (err) {
      setImportError(err);
    } finally {
      setImporting(false);
      setImportProgress(null);
    }
  }

  function handleProfileCreated(profile) {
    queryClient.invalidateQueries({ queryKey: queryKeys.bankProfiles() });
    setSelectedProfile(profile.name);
    setShowCreateModal(false);
  }

  const newCount = candidates ? candidates.filter((c) => !c.isDuplicate).length : 0;

  const tableData = useMemo(() => {
    if (!candidates) return [];
    const withIndex = candidates.map((c, i) => ({ ...c, _originalIndex: i }));
    if (ruleFilter === "matched") return withIndex.filter((c) => !!c.matchedRuleId);
    if (ruleFilter === "unmatched") return withIndex.filter((c) => !c.matchedRuleId);
    return withIndex;
  }, [candidates, ruleFilter]);

  const duplicateCount = candidates ? candidates.length - newCount : 0;
  const isSelected = useCallback((row) => selectedIndices.has(row._originalIndex), [selectedIndices]);
  const isDuplicate = useCallback((row) => row.isDuplicate, []);
  const toggleRow = useCallback((row) => toggleCandidate(row._originalIndex), [toggleCandidate]);

  const selectedProfileObj = sortedProfiles.find((p) => p.name === selectedProfile) ?? null;

  return (
    <Page>
      <PageHeader title="Import" />

      <Section title="Upload CSV">
        <Form onSubmit={handlePreview} className="max-w-2xl">
          {previewError && <ErrorBanner error={previewError} />}
          {profilesError && <ErrorBanner error={profilesError} />}
          <FormField label="Bank profile">
            <div className="flex items-center gap-1">
              <NativeSelect
                size="sm"
                value={selectedProfile || ""}
                onChange={(e) => setSelectedProfile(e.target.value || undefined)}
                disabled={profilesLoading}
                className="flex-1"
              >
                <NativeSelectOption value="">{profilesLoading ? "Loading…" : "Select profile…"}</NativeSelectOption>
                {sortedProfiles.map((p) => (
                  <NativeSelectOption key={p.name} value={p.name}>{p.name}</NativeSelectOption>
                ))}
              </NativeSelect>
              <ProfileIconButton label="Create new bank profile" onClick={() => setShowCreateModal(true)}>
                <Plus />
              </ProfileIconButton>
              {selectedProfile && (
                <>
                  <ProfileIconButton label="Edit bank profile" onClick={() => setEditingProfile(selectedProfileObj)}>
                    <Pencil />
                  </ProfileIconButton>
                  <ProfileIconButton label="Delete bank profile" onClick={() => setDeletingProfile(selectedProfileObj)}>
                    <Trash2 />
                  </ProfileIconButton>
                </>
              )}
            </div>
          </FormField>
          {!showPasteInput ? (
            <div
              data-slot="csv-dropzone"
              className={cn(
                "flex cursor-pointer flex-col items-center justify-center gap-1 rounded-lg border border-dashed border-input px-6 py-8 text-center transition-colors",
                isDragging
                  ? "border-primary bg-accent"
                  : csvFile
                  ? "border-primary/50"
                  : "hover:border-primary/50 hover:bg-muted/50"
              )}
              onClick={() => fileInputRef.current?.click()}
              onDragOver={(e) => { e.preventDefault(); setIsDragging(true); }}
              onDragLeave={(e) => { if (!e.currentTarget.contains(e.relatedTarget)) setIsDragging(false); }}
              onDrop={(e) => {
                e.preventDefault();
                setIsDragging(false);
                const dropped = Array.from(e.dataTransfer.files).find(
                  (f) => f.name.endsWith(".csv") || f.type === "text/csv"
                );
                if (dropped) setCsvFile(dropped);
              }}
            >
              <Upload className="mb-1 size-5 text-faint" aria-hidden="true" />
              {csvFile ? (
                <>
                  <p className="max-w-xs truncate font-medium">{csvFile.name}</p>
                  <p className="text-control text-muted-foreground">
                    {Math.round(csvFile.size / 1024)} KB · <span className="text-primary">Choose another file</span>
                  </p>
                </>
              ) : (
                <>
                  <p className="font-medium">Drop a CSV file here</p>
                  <p className="text-control text-muted-foreground">
                    or <span className="font-medium text-primary">browse</span>
                  </p>
                </>
              )}
              <input
                ref={fileInputRef}
                id="csv-file"
                type="file"
                accept=".csv,text/csv"
                className="hidden"
                onChange={(e) => setCsvFile(e.target.files[0] || null)}
              />
            </div>
          ) : (
            <Textarea
              aria-label="CSV content"
              placeholder="Paste CSV content here…"
              className="min-h-48 resize-y font-mono text-code"
              value={csvPasteText}
              onChange={(e) => setCsvPasteText(e.target.value)}
            />
          )}
          <FormActions align="between">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => {
                setShowPasteInput((v) => !v);
                setCsvFile(null);
                setCsvPasteText("");
              }}
              className="text-muted-foreground"
            >
              {showPasteInput ? (
                <><Upload data-icon="inline-start" /> Upload a file instead</>
              ) : (
                <><ClipboardPaste data-icon="inline-start" /> Paste CSV instead</>
              )}
            </Button>
            <Button
              type="submit"
              disabled={(!csvFile && !csvPasteText) || !selectedProfile}
              isLoading={previewing}
              loadingText="Previewing…"
            >
              Preview
            </Button>
          </FormActions>
        </Form>
      </Section>

      {importResult && (
        <Notice
          variant="info"
          action={importResult.importBatchId ? (
            <Link
              to="/transactions"
              search={{ importBatchId: importResult.importBatchId, account: "", payee: "" }}
            >
              Review
            </Link>
          ) : null}
        >
          Imported {plural(importResult.importedCount, "transaction")}
          {importResult.skippedDuplicateCount > 0 && `, skipped ${importResult.skippedDuplicateCount} already on disk`}.
        </Notice>
      )}
      {importError && <ErrorBanner error={importError} />}

      {candidates && (
        <Section
          title="Preview"
          description={`${plural(candidates.length, "transaction")} · ${newCount} new`}
          action={
            <div className="flex flex-wrap items-center gap-2">
              <Segmented
                label="Filter by rule match"
                options={RULE_FILTERS}
                value={ruleFilter}
                onChange={setRuleFilter}
              />
              <Button variant="ghost" size="sm" onClick={toggleAll}>
                {selectedIndices.size === newCount ? "Deselect all" : "Select all new"}
              </Button>
              <Button
                size="sm"
                onClick={handleImport}
                disabled={selectedIndices.size === 0}
                isLoading={importing}
                loadingText={
                  importProgress && importProgress.total > 0
                    ? `Importing ${importProgress.imported} of ${importProgress.total}…`
                    : "Importing…"
                }
              >
                Import {selectedIndices.size} selected
              </Button>
            </div>
          }
        >
          {duplicateCount > 0 && (
            <Notice variant="info">
              {plural(duplicateCount, "transaction")} {duplicateCount === 1 ? "is" : "are"} already in the journal and will be skipped.
            </Notice>
          )}
          {tableData.length === 0 ? (
            <p className="py-2 text-control text-muted-foreground">No transactions match this filter.</p>
          ) : (
            <CandidateTable
              rows={tableData}
              isSelected={isSelected}
              isDisabled={isDuplicate}
              onToggle={toggleRow}
              sorting={sorting}
              onSortingChange={setSorting}
            />
          )}
        </Section>
      )}

      <CreateProfileModal
        open={showCreateModal}
        onCreated={handleProfileCreated}
        onClose={() => setShowCreateModal(false)}
      />
      {editingProfile && (
        <EditProfileModal
          profile={editingProfile}
          open={!!editingProfile}
          onUpdated={handleProfileUpdated}
          onClose={() => setEditingProfile(null)}
        />
      )}
      {deletingProfile && (
        <DeleteProfileDialog
          profile={deletingProfile}
          open={!!deletingProfile}
          onDeleted={handleProfileDeleted}
          onClose={() => setDeletingProfile(null)}
        />
      )}
    </Page>
  );
}
