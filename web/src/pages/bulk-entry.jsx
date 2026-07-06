import { useState, useRef, Fragment } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearch, useNavigate } from "@tanstack/react-router";
import { Plus, Trash2, CircleCheck, LayoutGrid } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { todayStr } from "../format.js";
import { PageHeader } from "../components/page-header.jsx";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

function addOneMonth(dateStr) {
  if (!dateStr) return todayStr();
  const d = new Date(dateStr + "T12:00:00");
  const day = d.getDate();
  d.setDate(1);
  d.setMonth(d.getMonth() + 1);
  const lastDay = new Date(d.getFullYear(), d.getMonth() + 1, 0).getDate();
  d.setDate(Math.min(day, lastDay));
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

function makeRow(template, prevDate) {
  const amounts = {};
  if (template) {
    for (const p of template.postings) {
      amounts[p.account] = p.defaultQuantity ?? "";
    }
  }
  return {
    id: crypto.randomUUID(),
    date: prevDate ? addOneMonth(prevDate) : todayStr(),
    description: template
      ? [template.payee, template.note].filter(Boolean).join(" | ")
      : "",
    amounts,
    error: null,
    submitted: false,
  };
}

function RowCell({ value, onChange, onKeyDown, inputRef, placeholder, type = "text", className, readOnly, muted }) {
  return (
    <td className={cn("border-b px-1.5 py-1", className)}>
      {readOnly ? (
        <span className="block px-2 py-1 text-sm font-mono text-muted-foreground">∑ auto</span>
      ) : (
        <Input
          ref={inputRef}
          data-bulk-cell
          type={type}
          step={type === "number" ? "any" : undefined}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={onKeyDown}
          placeholder={placeholder}
          className={cn(
            "h-8 border-0 bg-transparent text-sm font-mono shadow-none focus-visible:ring-1",
            muted && "text-muted-foreground",
          )}
        />
      )}
    </td>
  );
}

export function BulkEntryPage() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const search = useSearch({ strict: false });
  const templateId = search?.templateId ?? null;

  const { data: templatesData, isLoading: templatesLoading } = useQuery({
    queryKey: queryKeys.templates(),
    queryFn: () => ledgerClient.listTemplates({}),
  });

  const templates = templatesData?.templates ?? [];
  const [selectedTemplateId, setSelectedTemplateId] = useState(templateId ?? "");

  const selectedTemplate = templates.find((t) => t.id === selectedTemplateId) ?? null;

  const [rows, setRows] = useState(() => [makeRow(null, null), makeRow(null, todayStr())]);
  const [submitting, setSubmitting] = useState(false);
  const [globalError, setGlobalError] = useState(null);
  const [allDone, setAllDone] = useState(false);

  // When template selection changes, reset rows to match the template structure
  const prevTemplateIdRef = useRef(selectedTemplateId);
  if (prevTemplateIdRef.current !== selectedTemplateId) {
    prevTemplateIdRef.current = selectedTemplateId;
    const tmpl = templates.find((t) => t.id === selectedTemplateId) ?? null;
    setRows([makeRow(tmpl, null), makeRow(tmpl, todayStr())]);
    setAllDone(false);
    setGlobalError(null);
  }

  function updateRow(rowId, patch) {
    setRows((rs) => rs.map((r) => (r.id === rowId ? { ...r, ...patch } : r)));
  }

  function updateAmount(rowId, account, value) {
    setRows((rs) =>
      rs.map((r) =>
        r.id === rowId ? { ...r, amounts: { ...r.amounts, [account]: value } } : r
      )
    );
  }

  function addRow() {
    const lastRow = rows[rows.length - 1];
    setRows((rs) => [...rs, makeRow(selectedTemplate, lastRow?.date ?? null)]);
  }

  function removeRow(rowId) {
    setRows((rs) => {
      const next = rs.filter((r) => r.id !== rowId);
      return next.length === 0 ? [makeRow(selectedTemplate, null)] : next;
    });
  }

  // Tab/Enter key handler for cell navigation
  function handleCellKey(e, rowIdx, colIdx, totalCols) {
    const isLastCol = colIdx === totalCols - 1;
    const isLastRow = rowIdx === rows.length - 1;
    if (e.key === "Tab" && !e.shiftKey && isLastCol && isLastRow) {
      e.preventDefault();
      addRow();
      setTimeout(() => {
        const inputs = document.querySelectorAll("[data-bulk-cell]");
        inputs[(rowIdx + 1) * totalCols]?.focus();
      }, 50);
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (isLastRow) {
        addRow();
        setTimeout(() => {
          const inputs = document.querySelectorAll("[data-bulk-cell]");
          inputs[(rowIdx + 1) * totalCols]?.focus();
        }, 50);
      } else {
        setTimeout(() => {
          const inputs = document.querySelectorAll("[data-bulk-cell]");
          inputs[(rowIdx + 1) * totalCols + colIdx]?.focus();
        }, 0);
      }
    }
  }

  async function handleSubmit() {
    if (submitting) return;
    setGlobalError(null);
    setSubmitting(true);

    const postings = selectedTemplate?.postings ?? [];

    // Validate all rows client-side first
    let anyError = false;
    const validatedRows = rows.map((row) => {
      if (!row.date || !row.description.trim()) {
        anyError = true;
        return { ...row, error: "Date and description are required." };
      }
      if (postings.length < 2) {
        anyError = true;
        return { ...row, error: "Select a template to define postings." };
      }
      return { ...row, error: null };
    });

    if (anyError) {
      setRows(validatedRows);
      setSubmitting(false);
      return;
    }

    // Build all transactions for a single batch RPC call
    const transactions = rows.map((row) => ({
      date: row.date,
      description: row.description.trim(),
      postings: postings.map((p) => ({
        account: p.account,
        commodity: row.amounts[p.account] ? (p.commodity || "USD") : "",
        quantity: row.amounts[p.account] ?? "",
      })),
    }));

    try {
      await ledgerClient.addTransactions({ transactions });
      setRows(rows.map((r) => ({ ...r, submitted: true, error: null })));
      queryClient.invalidateQueries({ queryKey: ["transactions"] });
      queryClient.invalidateQueries({ queryKey: ["accountRegister"] });
      queryClient.invalidateQueries({ queryKey: ["balances"] });
      queryClient.invalidateQueries({ queryKey: ["netWorthTimeseries"] });
      setAllDone(true);
    } catch (err) {
      setGlobalError(err.message || String(err));
    }

    setSubmitting(false);
  }

  const pendingCount = rows.filter((r) => !r.submitted).length;
  const doneCount = rows.filter((r) => r.submitted).length;

  if (templatesLoading) return <Loading />;

  const postings = selectedTemplate?.postings ?? [];

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Bulk Transaction Entry"
        description="Enter multiple similar transactions at once using a saved template."
      />

      {/* Template selector */}
      <Card>
        <CardHeader>
          <CardTitle>Template</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap items-center gap-3">
          <select
            className="h-9 rounded-md border bg-background px-3 text-sm shadow-sm focus:outline-none focus:ring-2 focus:ring-ring"
            value={selectedTemplateId}
            onChange={(e) => setSelectedTemplateId(e.target.value)}
          >
            <option value="">— Select a template —</option>
            {templates.map((t) => (
              <option key={t.id} value={t.id}>{t.name}</option>
            ))}
          </select>
          {templates.length === 0 && (
            <span className="text-sm text-muted-foreground">
              No templates yet.{" "}
              <button
                type="button"
                className="underline"
                onClick={() => navigate({ to: "/templates" })}
              >
                Create one
              </button>
            </span>
          )}
          {selectedTemplate && (
            <div className="flex flex-wrap gap-1">
              {selectedTemplate.postings.map((p) => (
                <Badge key={p.account} variant="secondary" className="font-mono text-xs">
                  {p.account}{p.defaultQuantity ? ` (${p.defaultQuantity})` : " (auto)"}
                </Badge>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Success state */}
      {allDone && (
        <Card>
          <CardContent className="flex flex-col items-center gap-2 py-8 text-center">
            <CircleCheck className="size-12 text-green-500" />
            <p className="font-medium">All {doneCount} transaction{doneCount !== 1 ? "s" : ""} added successfully!</p>
            <div className="flex gap-2">
              <Button variant="outline" onClick={() => {
                setAllDone(false);
                setRows([makeRow(selectedTemplate, null), makeRow(selectedTemplate, todayStr())]);
              }}>
                Add more
              </Button>
              <Button onClick={() => navigate({ to: "/transactions" })}>View transactions</Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Entry grid */}
      {!allDone && selectedTemplate && (
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle>Transactions</CardTitle>
            <Button size="sm" variant="outline" onClick={addRow}>
              <Plus data-icon="inline-start" /> Add row
            </Button>
          </CardHeader>
          <CardContent className="overflow-x-auto">
            {globalError && <ErrorBanner error={globalError} className="mb-4" />}
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left text-xs text-muted-foreground">
                  <th className="px-1.5 py-2 font-medium">Date</th>
                  <th className="px-1.5 py-2 font-medium">Description</th>
                  {postings.map((p) => (
                    <th key={p.account} className="px-1.5 py-2 font-medium font-mono">
                      {p.account.split(":").pop()}
                      {!p.defaultQuantity && (
                        <span className="ml-1 text-muted-foreground/60">(auto)</span>
                      )}
                    </th>
                  ))}
                  <th className="w-8" />
                </tr>
              </thead>
              <tbody>
                {rows.map((row, rowIdx) => {
                  const totalCols = 2 + postings.filter((p) => (p.defaultQuantity ?? "") !== "").length;
                  return (
                    <Fragment key={row.id}>
                      <tr
                        className={cn(
                          row.submitted && "opacity-40",
                          row.error && "bg-destructive/5",
                        )}
                      >
                        <RowCell
                          type="date"
                          value={row.date}
                          onChange={(v) => updateRow(row.id, { date: v })}
                          onKeyDown={(e) => handleCellKey(e, rowIdx, 0, totalCols)}
                          className="w-36"
                          inputRef={undefined}
                          placeholder=""
                        />
                        <RowCell
                          value={row.description}
                          onChange={(v) => updateRow(row.id, { description: v })}
                          onKeyDown={(e) => handleCellKey(e, rowIdx, 1, totalCols)}
                          placeholder="Description"
                          className="min-w-40"
                        />
                        {postings.map((p, colIdx) => {
                          const autoBalance = (p.defaultQuantity ?? "") === "";
                          return (
                            <RowCell
                              key={p.account}
                              type="number"
                              value={row.amounts[p.account] ?? ""}
                              onChange={(v) => updateAmount(row.id, p.account, v)}
                              onKeyDown={(e) => handleCellKey(e, rowIdx, 2 + colIdx, totalCols)}
                              placeholder={autoBalance ? "auto" : "0.00"}
                              readOnly={autoBalance}
                              className="w-32"
                            />
                          );
                        })}
                        <td className="border-b px-1">
                          {row.submitted ? (
                            <CircleCheck className="size-4 text-green-500" />
                          ) : (
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon-xs"
                              onClick={() => removeRow(row.id)}
                              className="text-muted-foreground hover:text-destructive"
                              title="Remove row"
                            >
                              <Trash2 className="size-3.5" />
                            </Button>
                          )}
                        </td>
                      </tr>
                      {row.error && (
                        <tr>
                          <td colSpan={3 + postings.length} className="border-b px-1.5 pb-1 text-xs text-destructive">
                            {row.error}
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  );
                })}
              </tbody>
            </table>
            <div className="mt-4 flex items-center justify-between">
              <Button variant="outline" size="sm" onClick={addRow}>
                <Plus data-icon="inline-start" /> Add row
              </Button>
              <Button
                onClick={handleSubmit}
                isLoading={submitting}
                loadingText={`Submitting… (${doneCount}/${rows.length})`}
                disabled={submitting || pendingCount === 0}
              >
                Submit {pendingCount} transaction{pendingCount !== 1 ? "s" : ""}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Prompt to select template */}
      {!allDone && !selectedTemplate && (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 py-10 text-center text-muted-foreground">
            <LayoutGrid className="size-10" />
            <p className="text-sm">Select a template above to start entering transactions.</p>
            {templates.length === 0 && (
              <Button variant="outline" size="sm" onClick={() => navigate({ to: "/templates" })}>
                Create a template
              </Button>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
