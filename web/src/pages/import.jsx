import { useState, useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, CircleCheck, Pencil, Trash2, Tag, Loader2 } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

const COLUMN_TYPES = [
  { value: "date", label: "Date" },
  { value: "description", label: "Description" },
  { value: "amount", label: "Amount" },
  { value: "debit", label: "Debit (money out)" },
  { value: "credit", label: "Credit (money in)" },
  { value: "balance", label: "Balance" },
  { value: "ignore", label: "Ignore" },
];

function slugifyProfileName(name) {
  const slug = name.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, "");
  return slug ? `rules/${slug}.rules` : "";
}

function parseCsvRows(text) {
  const lines = text.trim().split(/\r?\n/).filter(Boolean);
  return lines.map((line) => {
    const row = [];
    let field = "";
    let inQuote = false;
    for (let i = 0; i < line.length; i++) {
      const ch = line[i];
      if (ch === '"') {
        if (inQuote && line[i + 1] === '"') { field += '"'; i++; }
        else inQuote = !inQuote;
      } else if (ch === "," && !inQuote) {
        row.push(field.trim());
        field = "";
      } else {
        field += ch;
      }
    }
    row.push(field.trim());
    return row;
  });
}

function guessColumnType(header) {
  const h = header.toLowerCase();
  if (/date|posted|trans.*date/.test(h)) return "date";
  if (/desc|narr|memo|detail|merchant|payee|ref/.test(h)) return "description";
  if (/^amount$|^amt$|^value$|transaction amount/.test(h)) return "amount";
  if (/debit|withdrawal/.test(h)) return "debit";
  if (/credit|deposit/.test(h)) return "credit";
  if (/balance|bal\.?$/.test(h)) return "balance";
  return "ignore";
}

function detectDateFormat(sample) {
  const s = sample.trim();
  if (/^\d{4}-\d{2}-\d{2}$/.test(s)) return "%Y-%m-%d";
  if (/^\d{4}\/\d{2}\/\d{2}$/.test(s)) return "%Y/%m/%d";
  if (/^\d{2}\/\d{2}\/\d{4}$/.test(s)) return "%m/%d/%Y";
  if (/^\d{1,2}\/\d{1,2}\/\d{4}$/.test(s)) return "%m/%d/%Y";
  if (/^\d{2}-\d{2}-\d{4}$/.test(s)) return "%m-%d-%Y";
  if (/^\d{2}\.\d{2}\.\d{4}$/.test(s)) return "%d.%m.%Y";
  if (/^\d{8}$/.test(s)) return "%Y%m%d";
  return "%Y-%m-%d";
}

function buildRulesContent({ account, columnMappings, dateFormat }) {
  let fieldsLine;
  if (columnMappings && columnMappings.length > 0) {
    const fieldNames = columnMappings.map((t) => {
      if (t === "date") return "date";
      if (t === "description") return "description";
      if (t === "amount") return "amount";
      if (t === "debit") return "amount-out";
      if (t === "credit") return "amount-in";
      if (t === "balance") return "balance";
      return "_";
    });
    fieldsLine = `fields ${fieldNames.join(", ")}`;
  } else {
    fieldsLine = "fields date, description, amount";
  }
  return [
    "skip 1",
    fieldsLine,
    `date-format ${dateFormat || "%Y-%m-%d"}`,
    `account1 ${account || "assets:checking"}`,
    "currency $",
    "",
    "# Add conditional rules to categorize transactions:",
    "# if AMAZON",
    "#   account2 expenses:shopping",
    "#",
    "# if PAYROLL",
    "#   account2 income:salary",
  ].join("\n");
}

function CreateProfileModal({ open, onCreated, onClose }) {
  const [name, setName] = useState("");
  const [account, setAccount] = useState("");
  const [sampleCsv, setSampleCsv] = useState("");
  const [parsedRows, setParsedRows] = useState([]);
  const [columnMappings, setColumnMappings] = useState([]);
  const [dateFormat, setDateFormat] = useState("%Y-%m-%d");
  const [rulesContent, setRulesContent] = useState(buildRulesContent({}));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (!open) return;
    setName("");
    setAccount("");
    setSampleCsv("");
    setParsedRows([]);
    setColumnMappings([]);
    setDateFormat("%Y-%m-%d");
    setRulesContent(buildRulesContent({}));
    setError(null);
  }, [open]);

  useEffect(() => {
    if (!sampleCsv.trim()) {
      setParsedRows([]);
      setColumnMappings([]);
      return;
    }
    const rows = parseCsvRows(sampleCsv);
    if (rows.length === 0) return;
    setParsedRows(rows);
    const mappings = rows[0].map(guessColumnType);
    setColumnMappings(mappings);
    if (rows.length > 1) {
      const dateIdx = mappings.findIndex((m) => m === "date");
      if (dateIdx >= 0 && rows[1][dateIdx]) {
        setDateFormat(detectDateFormat(rows[1][dateIdx]));
      }
    }
  }, [sampleCsv]);

  function handleGenerateRules() {
    setRulesContent(buildRulesContent({ account, columnMappings, dateFormat }));
  }

  const rulesFilePath = slugifyProfileName(name);
  const hasCsvColumns = parsedRows.length > 0 && columnMappings.length > 0;

  async function handleSubmit(e) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      const res = await ledgerClient.createBankProfile({
        name,
        rulesFile: rulesFilePath,
        rulesContent: new TextEncoder().encode(rulesContent),
      });
      onCreated(res.profile);
    } catch (err) {
      setError(err);
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Create Bank Profile</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          {/* Profile name */}
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="profile-name">Profile Name</Label>
            <Input
              id="profile-name"
              type="text"
              placeholder="e.g. Chase Checking"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
            {rulesFilePath && (
              <p className="text-xs text-muted-foreground font-mono">Saves to: {rulesFilePath}</p>
            )}
          </div>

          {/* Bank account */}
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="account1">Bank Account</Label>
            <Input
              id="account1"
              type="text"
              placeholder="e.g. assets:checking"
              value={account}
              onChange={(e) => setAccount(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              The hledger account that holds money from this bank
            </p>
          </div>

          {/* CSV column mapping */}
          <div className="flex flex-col gap-2">
            <div className="flex items-baseline justify-between">
              <Label>CSV Column Mapping</Label>
              <span className="text-xs text-muted-foreground">
                paste 2–3 rows from your CSV to auto-detect columns
              </span>
            </div>
            <Textarea
              className="h-20 font-mono text-xs"
              placeholder={"Date,Description,Amount\n2026-04-01,AMAZON,-45.00\n2026-04-02,PAYROLL,2000.00"}
              value={sampleCsv}
              onChange={(e) => setSampleCsv(e.target.value)}
            />
            {hasCsvColumns && (
              <div className="flex flex-col gap-3 rounded-md border p-3">
                <div className="flex flex-wrap gap-3">
                  {parsedRows[0].map((header, idx) => (
                    <div key={idx} className="flex flex-col gap-1">
                      <span
                        className="text-xs font-mono text-muted-foreground truncate max-w-28"
                        title={header || `Col ${idx + 1}`}
                      >
                        {header || `Col ${idx + 1}`}
                      </span>
                      <Select
                        value={columnMappings[idx]}
                        onValueChange={(v) => {
                          const next = [...columnMappings];
                          next[idx] = v;
                          setColumnMappings(next);
                        }}
                      >
                        <SelectTrigger size="sm" className="w-36">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {COLUMN_TYPES.map((ct) => (
                            <SelectItem key={ct.value} value={ct.value}>{ct.label}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  ))}
                </div>
                <div className="flex items-end gap-2">
                  <div className="flex flex-col gap-1">
                    <Label className="text-xs">Date Format</Label>
                    <Input
                      className="font-mono text-xs h-8 w-36"
                      value={dateFormat}
                      onChange={(e) => setDateFormat(e.target.value)}
                      placeholder="%Y-%m-%d"
                    />
                  </div>
                  <Button type="button" size="sm" variant="secondary" onClick={handleGenerateRules}>
                    Generate Rules
                  </Button>
                </div>
              </div>
            )}
            {!hasCsvColumns && account && (
              <Button
                type="button"
                size="sm"
                variant="secondary"
                className="self-start"
                onClick={handleGenerateRules}
              >
                Generate Rules from Account
              </Button>
            )}
          </div>

          {/* Raw rules textarea */}
          <div className="flex flex-col gap-1.5">
            <div className="flex items-baseline justify-between">
              <Label htmlFor="rules-content">Rules File</Label>
              <a
                href="https://hledger.org/hledger.html#csv-rules-files"
                target="_blank"
                rel="noopener noreferrer"
                className="text-xs text-muted-foreground underline"
              >
                hledger CSV rules docs
              </a>
            </div>
            <Textarea
              id="rules-content"
              className="h-48 font-mono text-xs"
              value={rulesContent}
              onChange={(e) => setRulesContent(e.target.value)}
            />
          </div>

          {error && <ErrorBanner error={error} />}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
            <Button type="submit" disabled={saving || !rulesFilePath}>
              {saving && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
              {saving ? "Creating…" : "Create Profile"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function EditProfileModal({ profile, open, onUpdated, onClose }) {
  const [name, setName] = useState(profile?.name ?? "");
  const [rulesContent, setRulesContent] = useState("");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (!open || !profile) return;
    setName(profile.name);
    setError(null);
    setLoading(true);
    ledgerClient.getBankProfileContent({ name: profile.name })
      .then((res) => setRulesContent(new TextDecoder().decode(res.rulesContent)))
      .catch((err) => setError(err))
      .finally(() => setLoading(false));
  }, [open, profile]);

  async function handleSubmit(e) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      const res = await ledgerClient.updateBankProfile({
        name: profile.name,
        newName: name !== profile.name ? name : "",
        rulesContent: new TextEncoder().encode(rulesContent),
      });
      onUpdated(res.profile);
    } catch (err) {
      setError(err);
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Edit Bank Profile</DialogTitle>
        </DialogHeader>
        {loading ? (
          <div className="py-8 text-center text-muted-foreground text-sm">Loading…</div>
        ) : (
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="edit-profile-name">Profile Name</Label>
              <Input
                id="edit-profile-name"
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <div className="flex items-baseline justify-between">
                <Label htmlFor="edit-rules-content">Rules File Content</Label>
                <span className="text-xs text-muted-foreground">{profile?.rulesFile}</span>
              </div>
              <Textarea
                id="edit-rules-content"
                className="h-64 font-mono text-xs"
                value={rulesContent}
                onChange={(e) => setRulesContent(e.target.value)}
              />
            </div>
            {error && <ErrorBanner error={error} />}
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
              <Button type="submit" disabled={saving}>
                {saving && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
                {saving ? "Saving…" : "Save Changes"}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}

function DeleteProfileDialog({ profile, open, onDeleted, onClose }) {
  const [deleteFile, setDeleteFile] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (open) { setDeleteFile(false); setError(null); }
  }, [open]);

  async function handleDelete() {
    setDeleting(true);
    setError(null);
    try {
      await ledgerClient.deleteBankProfile({ name: profile.name, deleteRulesFile: deleteFile });
      onDeleted(profile.name);
    } catch (err) {
      setError(err);
    } finally {
      setDeleting(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete Bank Profile</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <p className="text-sm">
            Are you sure you want to delete <strong>{profile?.name}</strong>?
          </p>
          <div className="flex items-center gap-2">
            <Checkbox
              id="delete-rules-file"
              checked={deleteFile}
              onCheckedChange={setDeleteFile}
            />
            <Label htmlFor="delete-rules-file" className="text-sm cursor-pointer">
              Also delete rules file ({profile?.rulesFile})
            </Label>
          </div>
          {error && <ErrorBanner error={error} />}
        </div>
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          <Button variant="destructive" onClick={handleDelete} disabled={deleting}>
            {deleting && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
            {deleting ? "Deleting…" : "Delete Profile"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function ImportPage() {
  const queryClient = useQueryClient();

  const { data: profilesData, isLoading: profilesLoading, error: profilesError } = useQuery({
    queryKey: queryKeys.bankProfiles(),
    queryFn: () => ledgerClient.listBankProfiles({}),
  });

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
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [editingProfile, setEditingProfile] = useState(null);
  const [deletingProfile, setDeletingProfile] = useState(null);

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

  function handleProfileCreated(profile) {
    queryClient.invalidateQueries({ queryKey: queryKeys.bankProfiles() });
    setSelectedProfile(profile.name);
    setShowCreateModal(false);
  }

  const newCount = candidates ? candidates.filter((c) => !c.isDuplicate).length : 0;

  return (
    <div className="flex flex-col gap-6">
      <h2 className="text-2xl font-bold">Import Transactions</h2>

      {/* Upload form */}
      <Card>
        <CardHeader>
          <CardTitle>Upload CSV</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handlePreview} className="flex flex-wrap items-end gap-3">
            <div className="flex w-full flex-col gap-1.5 sm:w-56">
              <Label>Bank Profile</Label>
              <div className="flex items-center gap-2">
                {profilesLoading ? (
                  <Select disabled value="">
                    <SelectTrigger size="sm" className="flex-1">
                      <SelectValue>Loading…</SelectValue>
                    </SelectTrigger>
                  </Select>
                ) : (
                  <Select
                    value={selectedProfile || undefined}
                    onValueChange={setSelectedProfile}
                  >
                    <SelectTrigger size="sm" className="flex-1">
                      <SelectValue placeholder="Select profile…">
                        {selectedProfile || "Select profile…"}
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      {(profilesData?.profiles ?? []).map((p) => (
                        <SelectItem key={p.name} value={p.name}>{p.name}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => setShowCreateModal(true)}
                      >
                        <Plus />
                      </Button>
                    }
                  />
                  <TooltipContent>Create new bank profile</TooltipContent>
                </Tooltip>
                {selectedProfile && (
                  <>
                    <Tooltip>
                      <TooltipTrigger
                        render={
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon-sm"
                            onClick={() => setEditingProfile(
                              profilesData?.profiles?.find((p) => p.name === selectedProfile) ?? null
                            )}
                          >
                            <Pencil />
                          </Button>
                        }
                      />
                      <TooltipContent>Edit bank profile</TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger
                        render={
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon-sm"
                            onClick={() => setDeletingProfile(
                              profilesData?.profiles?.find((p) => p.name === selectedProfile) ?? null
                            )}
                          >
                            <Trash2 />
                          </Button>
                        }
                      />
                      <TooltipContent>Delete bank profile</TooltipContent>
                    </Tooltip>
                  </>
                )}
              </div>
            </div>
            <div className="flex w-full flex-1 flex-col gap-1.5 sm:w-auto">
              <Label htmlFor="csv-file">CSV File</Label>
              <Input
                id="csv-file"
                type="file"
                accept=".csv,text/csv"
                onChange={(e) => setCsvFile(e.target.files[0] || null)}
                required
              />
            </div>
            <Button
              type="submit"
              disabled={previewing || !csvFile || !selectedProfile}
            >
              {previewing && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
              {previewing ? "Previewing…" : "Preview"}
            </Button>
          </form>
          {previewError && <div className="mt-3"><ErrorBanner error={previewError} /></div>}
          {profilesError && <div className="mt-3"><ErrorBanner error={profilesError} /></div>}
        </CardContent>
      </Card>

      {/* Import result */}
      {importResult && (
        <Alert>
          <CircleCheck className="size-4 text-success" />
          <AlertDescription>
            Imported {importResult.importedCount} transaction(s) successfully.
          </AlertDescription>
        </Alert>
      )}
      {importError && <ErrorBanner error={importError} />}

      {/* Preview table */}
      {candidates && (
        <Card>
          <CardHeader>
            <div className="flex flex-wrap items-center justify-between gap-4">
              <CardTitle>
                Preview — {candidates.length} transaction(s), {newCount} new
              </CardTitle>
              <div className="flex gap-2">
                <Button variant="ghost" size="sm" onClick={toggleAll}>
                  {selectedIndices.size === newCount ? "Deselect All" : "Select All New"}
                </Button>
                <Button
                  size="sm"
                  onClick={handleImport}
                  disabled={importing || selectedIndices.size === 0}
                >
                  {importing && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
                  {importing ? "Importing…" : `Import ${selectedIndices.size} Selected`}
                </Button>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead></TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Date</TableHead>
                  <TableHead>Description</TableHead>
                  <TableHead>Postings</TableHead>
                  <TableHead>Matched</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {candidates.map((c, i) => (
                  <TableRow key={i} className={cn(c.isDuplicate && "opacity-50")}>
                    <TableCell>
                      <Checkbox
                        checked={selectedIndices.has(i)}
                        disabled={c.isDuplicate}
                        onCheckedChange={() => toggleCandidate(i)}
                      />
                    </TableCell>
                    <TableCell>
                      <Badge variant={c.isDuplicate ? "secondary" : "default"}>
                        {c.isDuplicate ? "DUP" : "NEW"}
                      </Badge>
                    </TableCell>
                    <TableCell className="whitespace-nowrap">{c.transaction?.date}</TableCell>
                    <TableCell>{c.transaction?.description}</TableCell>
                    <TableCell className="text-xs">
                      {(c.transaction?.postings ?? []).map((p, j) => (
                        <div key={j}>
                          {p.account}
                          {p.amounts?.[0] && (
                            <span className="ml-1 text-muted-foreground">
                              {p.amounts[0].commodity}{p.amounts[0].quantity}
                            </span>
                          )}
                        </div>
                      ))}
                    </TableCell>
                    <TableCell>
                      {c.matchedRuleId
                        ? <Tag className="size-3.5 text-primary" title="Matched a rule" />
                        : <span className="text-muted-foreground">—</span>
                      }
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
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
    </div>
  );
}
