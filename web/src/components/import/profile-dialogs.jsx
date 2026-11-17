import { useState, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { ledgerClient } from "@/client.js";
import { queryKeys } from "@/query-keys.js";
import { ErrorBanner } from "@/components/error-banner.jsx";
import { Loading } from "@/components/loading.jsx";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import { Form, FormField, FormRow } from "@/components/ui/form";
import { Combobox } from "@/components/ui/combobox";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  ResponsiveDialog,
  ResponsiveDialogContent,
  ResponsiveDialogFooter,
  ResponsiveDialogHeader,
  ResponsiveDialogTitle,
} from "@/components/ui/responsive-dialog";

// Bank profile dialogs for the Import page: create (with the CSV column
// wizard that drafts an hledger CSV rules file), edit and delete. Rules
// content and CSV samples stay mono: they are literal hledger syntax.

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
    "currency USD",
    "",
    "# Add conditional rules to categorize transactions:",
    "# if AMAZON",
    "#   account2 expenses:shopping",
    "#",
    "# if PAYROLL",
    "#   account2 income:salary",
  ].join("\n");
}

export function CreateProfileModal({ open, onCreated, onClose }) {
  const [name, setName] = useState("");
  const [account, setAccount] = useState("");
  const [sampleCsv, setSampleCsv] = useState("");
  const [parsedRows, setParsedRows] = useState([]);
  const [columnMappings, setColumnMappings] = useState([]);
  const [dateFormat, setDateFormat] = useState("%Y-%m-%d");
  const [rulesContent, setRulesContent] = useState(buildRulesContent({}));
  const [skipRules, setSkipRules] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  const { data: declarationsData } = useQuery({
    queryKey: queryKeys.accountDeclarations(),
    queryFn: () => ledgerClient.listAccountDeclarations({}),
    enabled: open,
  });

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
        skipRules,
      });
      onCreated(res.profile);
    } catch (err) {
      setError(err);
    } finally {
      setSaving(false);
    }
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={(v) => { if (!v) onClose(); }}>
      <ResponsiveDialogContent size="lg">
        <ResponsiveDialogHeader>
          <ResponsiveDialogTitle>Create bank profile</ResponsiveDialogTitle>
        </ResponsiveDialogHeader>
        <Form onSubmit={handleSubmit}>
          {error && <ErrorBanner error={error} />}
          <FormRow cols={2}>
            <FormField
              label="Profile name"
              htmlFor="profile-name"
              description={rulesFilePath ? <>Saves to <span className="font-mono">{rulesFilePath}</span></> : null}
            >
              <Input
                id="profile-name"
                type="text"
                placeholder="e.g. Chase Checking"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </FormField>
            <FormField
              label="Bank account"
              description="The hledger account that holds money from this bank"
            >
              <Combobox
                id="account1"
                value={account}
                onChange={setAccount}
                options={(declarationsData?.declarations ?? []).map((d) => d.name)}
                placeholder="e.g. assets:checking"
                searchPlaceholder="Search accounts…"
                emptyMessage="No matching account."
                allowCustomValue
              />
            </FormField>
          </FormRow>

          <div className="flex items-center gap-2">
            <Checkbox id="skip-rules" checked={skipRules} onCheckedChange={setSkipRules} />
            <Label htmlFor="skip-rules" className="cursor-pointer font-normal">
              Skip float categorization rules on import
            </Label>
          </div>

          {/* CSV column mapping */}
          <FormField
            label="CSV column mapping"
            hint="paste 2–3 rows to auto-detect columns"
          >
            <Textarea
              className="h-20 font-mono text-code"
              placeholder={"Date,Description,Amount\n2026-04-01,AMAZON,-45.00\n2026-04-02,PAYROLL,2000.00"}
              value={sampleCsv}
              onChange={(e) => setSampleCsv(e.target.value)}
            />
          </FormField>
          {hasCsvColumns && (
            <div data-slot="csv-wizard" className="flex flex-col gap-3 rounded-lg bg-muted p-3">
              <div className="flex flex-wrap gap-x-3 gap-y-2">
                {parsedRows[0].map((header, idx) => (
                  <div key={idx} className="flex flex-col gap-1">
                    <span
                      className="max-w-36 truncate font-mono text-code text-muted-foreground"
                      title={header || `Col ${idx + 1}`}
                    >
                      {header || `Col ${idx + 1}`}
                    </span>
                    <NativeSelect
                      size="sm"
                      value={columnMappings[idx]}
                      onChange={(e) => {
                        const next = [...columnMappings];
                        next[idx] = e.target.value;
                        setColumnMappings(next);
                      }}
                      className="w-36 bg-background"
                    >
                      {COLUMN_TYPES.map((ct) => (
                        <NativeSelectOption key={ct.value} value={ct.value}>{ct.label}</NativeSelectOption>
                      ))}
                    </NativeSelect>
                  </div>
                ))}
              </div>
              <FormRow cols={2}>
                <FormField label="Date format">
                  <Input
                    className="bg-background font-mono"
                    value={dateFormat}
                    onChange={(e) => setDateFormat(e.target.value)}
                    placeholder="%Y-%m-%d"
                  />
                </FormField>
                <div className="flex items-end">
                  <Button type="button" size="sm" variant="outline" onClick={handleGenerateRules}>
                    Generate rules
                  </Button>
                </div>
              </FormRow>
            </div>
          )}
          {!hasCsvColumns && account && (
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="self-start"
              onClick={handleGenerateRules}
            >
              Generate rules from account
            </Button>
          )}

          <FormField
            label="Rules file"
            htmlFor="rules-content"
            hint={
              <a
                href="https://hledger.org/hledger.html#csv-rules-files"
                target="_blank"
                rel="noopener noreferrer"
                className="underline underline-offset-2 hover:text-foreground"
              >
                hledger CSV rules docs
              </a>
            }
          >
            <Textarea
              id="rules-content"
              className="h-48 font-mono text-code"
              value={rulesContent}
              onChange={(e) => setRulesContent(e.target.value)}
            />
          </FormField>

          <ResponsiveDialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
            <Button
              type="submit"
              disabled={!rulesFilePath}
              isLoading={saving}
              loadingText="Creating…"
            >
              Create profile
            </Button>
          </ResponsiveDialogFooter>
        </Form>
      </ResponsiveDialogContent>
    </ResponsiveDialog>
  );
}

export function EditProfileModal({ profile, open, onUpdated, onClose }) {
  const [name, setName] = useState(profile?.name ?? "");
  const [rulesContent, setRulesContent] = useState("");
  const [skipRules, setSkipRules] = useState(profile?.skipRules ?? false);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (!open || !profile) return;
    setName(profile.name);
    setSkipRules(profile.skipRules ?? false);
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
        skipRules,
      });
      onUpdated(res.profile);
    } catch (err) {
      setError(err);
    } finally {
      setSaving(false);
    }
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={(v) => { if (!v) onClose(); }}>
      <ResponsiveDialogContent size="lg">
        <ResponsiveDialogHeader>
          <ResponsiveDialogTitle>Edit bank profile</ResponsiveDialogTitle>
        </ResponsiveDialogHeader>
        {loading ? (
          <Loading />
        ) : (
          <Form onSubmit={handleSubmit}>
            {error && <ErrorBanner error={error} />}
            <FormField label="Profile name" htmlFor="edit-profile-name">
              <Input
                id="edit-profile-name"
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </FormField>
            <FormField
              label="Rules file"
              htmlFor="edit-rules-content"
              hint={<span className="font-mono">{profile?.rulesFile}</span>}
            >
              <Textarea
                id="edit-rules-content"
                className="h-64 font-mono text-code"
                value={rulesContent}
                onChange={(e) => setRulesContent(e.target.value)}
              />
            </FormField>
            <div className="flex items-center gap-2">
              <Checkbox id="edit-skip-rules" checked={skipRules} onCheckedChange={setSkipRules} />
              <Label htmlFor="edit-skip-rules" className="cursor-pointer font-normal">
                Skip float categorization rules on import
              </Label>
            </div>
            <ResponsiveDialogFooter>
              <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
              <Button type="submit" isLoading={saving} loadingText="Saving…">
                Save changes
              </Button>
            </ResponsiveDialogFooter>
          </Form>
        )}
      </ResponsiveDialogContent>
    </ResponsiveDialog>
  );
}

export function DeleteProfileDialog({ profile, open, onDeleted, onClose }) {
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
          <DialogTitle>Delete bank profile</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <p>
            Delete the <span className="font-medium">{profile?.name}</span> profile?
          </p>
          <div className="flex items-center gap-2">
            <Checkbox
              id="delete-rules-file"
              checked={deleteFile}
              onCheckedChange={setDeleteFile}
            />
            <Label htmlFor="delete-rules-file" className="cursor-pointer font-normal">
              Also delete the rules file <span className="font-mono text-code text-muted-foreground">{profile?.rulesFile}</span>
            </Label>
          </div>
          {error && <ErrorBanner error={error} />}
        </div>
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          <Button
            variant="destructive"
            onClick={handleDelete}
            isLoading={deleting}
            loadingText="Deleting…"
          >
            Delete profile
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
