import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { CircleCheck, Loader2 } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { AccountInput } from "../components/posting-fields.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { Checkbox } from "@/components/ui/checkbox";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";

function emptyForm() {
  return { pattern: "", payee: "", account: "", priority: "0", tags: "", autoReviewed: true };
}

function tagsFromString(str) {
  const tags = {};
  str.split(",").forEach((part) => {
    const [k, ...rest] = part.trim().split("=");
    if (k) tags[k.trim()] = rest.join("=").trim();
  });
  return tags;
}

function tagsToString(tags) {
  if (!tags) return "";
  return Object.entries(tags)
    .map(([k, v]) => (v ? `${k}=${v}` : k))
    .join(", ");
}

export function RulesPage() {
  const queryClient = useQueryClient();

  const { data: rulesData, isLoading: rulesLoading, error: rulesError } = useQuery({
    queryKey: queryKeys.rules(),
    queryFn: () => ledgerClient.listRules({}),
  });

  const { data: accountsData } = useQuery({
    queryKey: queryKeys.accounts(),
    queryFn: () => ledgerClient.listAccounts({}),
  });

  const [form, setForm] = useState(emptyForm());
  const [editingId, setEditingId] = useState(null);
  const [formError, setFormError] = useState(null);

  // Pattern test
  const [testDesc, setTestDesc] = useState("");

  // Apply rules
  const [applyPreviews, setApplyPreviews] = useState(null);
  const [applyLoading, setApplyLoading] = useState(false);
  const [applyError, setApplyError] = useState(null);
  const [selectedFids, setSelectedFids] = useState(new Set());
  const [applying, setApplying] = useState(false);
  const [applyResult, setApplyResult] = useState(null);

  const saveRuleMutation = useMutation({
    mutationFn: (payload) =>
      editingId
        ? ledgerClient.updateRule({ id: editingId, ...payload })
        : ledgerClient.addRule(payload),
    onSuccess: () => {
      setEditingId(null);
      setForm(emptyForm());
      setFormError(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.rules() });
    },
    onError: (err) => setFormError(err),
  });

  const deleteRuleMutation = useMutation({
    mutationFn: ({ id }) => ledgerClient.deleteRule({ id }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.rules() }),
    onError: (err) => setFormError(err),
  });

  function setField(name, value) {
    setForm((f) => ({ ...f, [name]: value }));
  }

  function startEdit(rule) {
    setEditingId(rule.id);
    setForm({
      pattern: rule.pattern,
      payee: rule.payee,
      account: rule.account,
      priority: String(rule.priority),
      tags: tagsToString(rule.tags),
      autoReviewed: rule.autoReviewed ?? false,
    });
    setFormError(null);
  }

  function cancelEdit() {
    setEditingId(null);
    setForm(emptyForm());
    setFormError(null);
  }

  function handleSubmit(e) {
    e.preventDefault();
    setFormError(null);
    const payload = {
      pattern: form.pattern,
      payee: form.payee,
      account: form.account,
      priority: parseInt(form.priority, 10) || 0,
      tags: tagsFromString(form.tags),
      autoReviewed: form.autoReviewed,
    };
    saveRuleMutation.mutate(payload);
  }

  function handleDelete(id) {
    deleteRuleMutation.mutate({ id });
  }

  // Find which rule matches the test description.
  function getMatchingRule() {
    if (!testDesc || !rulesData?.rules) return null;
    for (const r of rulesData.rules) {
      if (!r.pattern) continue;
      try {
        if (new RegExp(r.pattern, "i").test(testDesc)) return r;
      } catch {}
    }
    return null;
  }

  async function handlePreviewApply() {
    setApplyError(null);
    setApplyLoading(true);
    setApplyPreviews(null);
    setApplyResult(null);
    try {
      const res = await ledgerClient.previewApplyRules({ ruleIds: [], query: [] });
      setApplyPreviews(res.previews);
      setSelectedFids(new Set(res.previews.map((p) => p.fid)));
    } catch (err) {
      setApplyError(err);
    } finally {
      setApplyLoading(false);
    }
  }

  function toggleFid(fid) {
    setSelectedFids((prev) => {
      const next = new Set(prev);
      if (next.has(fid)) next.delete(fid);
      else next.add(fid);
      return next;
    });
  }

  async function handleApply() {
    if (selectedFids.size === 0) return;
    setApplyError(null);
    setApplying(true);
    try {
      const res = await ledgerClient.applyRules({
        fids: Array.from(selectedFids),
        ruleIds: [],
        query: [],
      });
      setApplyResult(res.appliedCount);
      setApplyPreviews(null);
    } catch (err) {
      setApplyError(err);
    } finally {
      setApplying(false);
    }
  }

  const matchingRule = getMatchingRule();

  return (
    <div className="flex flex-col gap-6">
      <h2 className="text-2xl font-bold">Categorization Rules</h2>

      {/* Card 1: Rule Editor */}
      <Card>
        <CardHeader>
          <CardTitle>{editingId ? "Edit Rule" : "Add Rule"}</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="flex flex-col gap-3">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="rule-pattern">Pattern (regex)</Label>
              <Input
                id="rule-pattern"
                type="text"
                className="font-mono"
                placeholder="AMAZON|amazon\.com"
                value={form.pattern}
                onChange={(e) => setField("pattern", e.target.value)}
                required
              />
            </div>
            <div className="flex flex-wrap gap-3">
              <div className="min-w-40 flex-1 flex flex-col gap-1.5">
                <Label htmlFor="rule-payee">Set Payee</Label>
                <Input
                  id="rule-payee"
                  type="text"
                  placeholder="Amazon"
                  value={form.payee}
                  onChange={(e) => setField("payee", e.target.value)}
                />
              </div>
              <div className="min-w-40 flex-1 flex flex-col gap-1.5">
                <Label>Set Category Account</Label>
                <AccountInput
                  value={form.account}
                  onChange={(v) => setField("account", v)}
                  accounts={accountsData?.accounts ?? []}
                  placeholder="expenses:shopping"
                />
              </div>
              <div className="min-w-40 flex-1 flex flex-col gap-1.5">
                <Label htmlFor="rule-tags">
                  Add Tags <span className="text-xs text-muted-foreground">key=val, key2</span>
                </Label>
                <Input
                  id="rule-tags"
                  type="text"
                  className="font-mono"
                  placeholder="source=import"
                  value={form.tags}
                  onChange={(e) => setField("tags", e.target.value)}
                />
              </div>
              <div className="w-24 flex flex-col gap-1.5">
                <Label htmlFor="rule-priority">Priority</Label>
                <Input
                  id="rule-priority"
                  type="number"
                  value={form.priority}
                  onChange={(e) => setField("priority", e.target.value)}
                />
              </div>
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id="rule-auto-reviewed"
                checked={form.autoReviewed}
                onCheckedChange={(v) => setField("autoReviewed", v)}
              />
              <Label htmlFor="rule-auto-reviewed">Auto-mark as reviewed on import</Label>
            </div>
            <div className="flex gap-2">
              <Button type="submit" size="sm" disabled={saveRuleMutation.isPending}>
                {saveRuleMutation.isPending && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
                {saveRuleMutation.isPending ? "Saving…" : editingId ? "Update Rule" : "Add Rule"}
              </Button>
              {editingId && (
                <Button type="button" variant="ghost" size="sm" onClick={cancelEdit}>
                  Cancel
                </Button>
              )}
            </div>
          </form>
          {formError && <div className="mt-3"><ErrorBanner error={formError} /></div>}
        </CardContent>
      </Card>

      {/* Card 2: Rules list + Apply section */}
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-4">
            <CardTitle>Rules</CardTitle>
            <Button
              variant="outline"
              size="sm"
              onClick={handlePreviewApply}
              disabled={applyLoading}
            >
              {applyLoading && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
              {applyLoading ? "Previewing…" : "Preview Changes"}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {rulesLoading && <Loading />}
          {rulesError && <ErrorBanner error={rulesError} />}
          {rulesData && rulesData.rules.length === 0 && (
            <p className="text-muted-foreground">No rules yet. Add one above.</p>
          )}
          {rulesData && rulesData.rules.length > 0 && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Priority</TableHead>
                  <TableHead>Pattern</TableHead>
                  <TableHead>Payee</TableHead>
                  <TableHead>Account</TableHead>
                  <TableHead>Tags</TableHead>
                  <TableHead>Auto-reviewed</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rulesData.rules.map((r) => (
                  <TableRow key={r.id} className={cn(editingId === r.id && "bg-primary/10")}>
                    <TableCell>
                      <Badge variant="secondary" className="font-mono">{r.priority}</Badge>
                    </TableCell>
                    <TableCell className="max-w-xs truncate font-mono text-xs" title={r.pattern}>{r.pattern}</TableCell>
                    <TableCell>{r.payee || <span className="text-muted-foreground/60">—</span>}</TableCell>
                    <TableCell className="font-mono text-xs">
                      {r.account || <span className="text-muted-foreground/60">—</span>}
                    </TableCell>
                    <TableCell className="text-xs">
                      {r.tags && Object.keys(r.tags).length > 0
                        ? tagsToString(r.tags)
                        : <span className="text-muted-foreground/60">—</span>}
                    </TableCell>
                    <TableCell>
                      {r.autoReviewed
                        ? <CircleCheck className="size-4 text-success" />
                        : <span className="text-muted-foreground/60">—</span>}
                    </TableCell>
                    <TableCell>
                      <div className="flex gap-1">
                        <Button
                          variant="ghost"
                          size="xs"
                          onClick={() => startEdit(r)}
                        >
                          Edit
                        </Button>
                        <Button
                          variant="ghost"
                          size="xs"
                          className="text-destructive"
                          onClick={() => handleDelete(r.id)}
                        >
                          Delete
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
          <Separator className="my-4" />
          <div className="flex items-center gap-3">
            <span className="whitespace-nowrap text-sm text-muted-foreground">Test description:</span>
            <Input
              type="text"
              className="flex-1 font-mono"
              placeholder="AMAZON.COM PURCHASE"
              value={testDesc}
              onChange={(e) => setTestDesc(e.target.value)}
            />
            {testDesc && (
              matchingRule ? (
                <Badge className="shrink-0 bg-success text-success-foreground">
                  {matchingRule.payee || matchingRule.account}
                </Badge>
              ) : (
                <Badge variant="outline" className="shrink-0">No match</Badge>
              )
            )}
          </div>
          {applyError && <div className="mt-3"><ErrorBanner error={applyError} /></div>}
          {applyResult !== null && (
            <Alert className="mt-3">
              <CircleCheck className="size-4 text-success" />
              <AlertDescription>
                Applied changes to {applyResult} transaction(s).
              </AlertDescription>
            </Alert>
          )}
          {applyPreviews && applyPreviews.length === 0 && (
            <p className="mt-3 text-muted-foreground">No transactions match any rules.</p>
          )}
          {applyPreviews && applyPreviews.length > 0 && (
            <div className="mt-3 flex flex-col gap-3">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>
                      <Checkbox
                        checked={selectedFids.size === applyPreviews.length}
                        onCheckedChange={() => {
                          if (selectedFids.size === applyPreviews.length) {
                            setSelectedFids(new Set());
                          } else {
                            setSelectedFids(new Set(applyPreviews.map((p) => p.fid)));
                          }
                        }}
                      />
                    </TableHead>
                    <TableHead>Description</TableHead>
                    <TableHead>Account</TableHead>
                    <TableHead>Payee</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {applyPreviews.map((p) => (
                    <TableRow key={p.fid}>
                      <TableCell>
                        <Checkbox
                          checked={selectedFids.has(p.fid)}
                          onCheckedChange={() => toggleFid(p.fid)}
                        />
                      </TableCell>
                      <TableCell>{p.description}</TableCell>
                      <TableCell className="text-xs">
                        {p.newAccount ? (
                          <span>
                            <span className="text-muted-foreground/60 line-through">{p.currentAccount}</span>
                            {" → "}
                            <span className="text-success">{p.newAccount}</span>
                          </span>
                        ) : (
                          <span className="text-muted-foreground/60">—</span>
                        )}
                      </TableCell>
                      <TableCell className="text-xs">
                        {p.newPayee ? (
                          <span>
                            <span className="text-muted-foreground/60 line-through">{p.currentPayee}</span>
                            {" → "}
                            <span className="text-success">{p.newPayee}</span>
                          </span>
                        ) : (
                          <span className="text-muted-foreground/60">—</span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              <Button
                size="sm"
                onClick={handleApply}
                disabled={applying || selectedFids.size === 0}
              >
                {applying && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
                {applying ? "Applying…" : `Apply to ${selectedFids.size} Transaction(s)`}
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
