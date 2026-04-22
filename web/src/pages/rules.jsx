import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "@tanstack/react-form";
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  getFilteredRowModel,
  createColumnHelper,
  flexRender,
} from "@tanstack/react-table";
import { CircleCheck, Loader2, ArrowUpDown, ArrowUp, ArrowDown } from "lucide-react";
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

function SortHeader({ column, children }) {
  const sorted = column.getIsSorted();
  const Icon = sorted === "asc" ? ArrowUp : sorted === "desc" ? ArrowDown : ArrowUpDown;
  return (
    <button
      className="flex items-center gap-1 hover:text-foreground"
      onClick={column.getToggleSortingHandler()}
    >
      {children}
      <Icon className="size-3.5" />
    </button>
  );
}

const columnHelper = createColumnHelper();

const rulesColumns = [
  columnHelper.accessor("priority", {
    header: ({ column }) => <SortHeader column={column}>P</SortHeader>,
    cell: ({ getValue }) => (
      <Badge variant="secondary" className="font-mono">{getValue()}</Badge>
    ),
    sortingFn: "basic",
    meta: { headerClass: "w-10 text-center" },
  }),
  columnHelper.accessor("pattern", {
    header: ({ column }) => <SortHeader column={column}>Pattern</SortHeader>,
    cell: ({ getValue }) => (
      <span className="max-w-xs truncate font-mono text-xs" title={getValue()}>{getValue()}</span>
    ),
    filterFn: "includesString",
  }),
  columnHelper.accessor("payee", {
    header: ({ column }) => <SortHeader column={column}>Payee</SortHeader>,
    cell: ({ getValue }) =>
      getValue() || <span className="text-muted-foreground/60">—</span>,
    filterFn: "includesString",
  }),
  columnHelper.accessor("account", {
    header: ({ column }) => <SortHeader column={column}>Account</SortHeader>,
    cell: ({ getValue }) =>
      getValue() ? (
        <span className="font-mono text-xs">{getValue()}</span>
      ) : (
        <span className="text-muted-foreground/60">—</span>
      ),
    filterFn: "includesString",
  }),
  columnHelper.accessor((row) => tagsToString(row.tags), {
    id: "tags",
    header: "Tags",
    cell: ({ getValue }) =>
      getValue() || <span className="text-muted-foreground/60">—</span>,
    meta: { headerClass: "text-xs" },
    enableSorting: false,
  }),
  columnHelper.accessor("autoReviewed", {
    header: "Auto-reviewed",
    cell: ({ getValue }) =>
      getValue() ? (
        <CircleCheck className="size-4 text-success" />
      ) : (
        <span className="text-muted-foreground/60">—</span>
      ),
    enableSorting: false,
    enableColumnFilter: false,
  }),
  columnHelper.display({
    id: "actions",
    header: "",
    cell: () => null, // rendered inline via meta
  }),
];

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

  const [editingId, setEditingId] = useState(null);
  const [formError, setFormError] = useState(null);

  const form = useForm({
    defaultValues: emptyForm(),
    onSubmit: async ({ value }) => {
      setFormError(null);
      const payload = {
        pattern: value.pattern,
        payee: value.payee,
        account: value.account,
        priority: parseInt(value.priority, 10) || 0,
        tags: tagsFromString(value.tags),
        autoReviewed: value.autoReviewed,
      };
      saveRuleMutation.mutate(payload);
    },
  });

  // Table state
  const [sorting, setSorting] = useState([]);
  const [globalFilter, setGlobalFilter] = useState("");

  const rules = useMemo(() => rulesData?.rules ?? [], [rulesData]);

  const table = useReactTable({
    data: rules,
    columns: rulesColumns,
    state: { sorting, globalFilter },
    onSortingChange: setSorting,
    onGlobalFilterChange: setGlobalFilter,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getRowId: (row) => row.id,
    globalFilterFn: "includesString",
  });

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
      form.reset();
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

  function startEdit(rule) {
    setEditingId(rule.id);
    form.reset();
    form.setFieldValue("pattern", rule.pattern);
    form.setFieldValue("payee", rule.payee);
    form.setFieldValue("account", rule.account);
    form.setFieldValue("priority", String(rule.priority));
    form.setFieldValue("tags", tagsToString(rule.tags));
    form.setFieldValue("autoReviewed", rule.autoReviewed ?? false);
    setFormError(null);
  }

  function cancelEdit() {
    setEditingId(null);
    form.reset();
    setFormError(null);
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
          <form
            onSubmit={(e) => {
              e.preventDefault();
              e.stopPropagation();
              form.handleSubmit();
            }}
            className="flex flex-col gap-3"
          >
            <form.Field
              name="pattern"
              validators={{
                onChange: ({ value }) => (!value ? "Pattern is required" : undefined),
              }}
              children={(field) => (
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="rule-pattern">Pattern (regex)</Label>
                  <Input
                    id="rule-pattern"
                    type="text"
                    className="font-mono"
                    placeholder="AMAZON|amazon\.com"
                    value={field.state.value}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                  />
                  {field.state.meta.isTouched && !field.state.meta.isValid && (
                    <p className="text-xs text-destructive">{field.state.meta.errors.join(", ")}</p>
                  )}
                </div>
              )}
            />
            <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap">
              <form.Field
                name="payee"
                children={(field) => (
                  <div className="flex flex-1 flex-col gap-1.5 min-w-0 sm:min-w-40">
                    <Label htmlFor="rule-payee">Set Payee</Label>
                    <Input
                      id="rule-payee"
                      type="text"
                      placeholder="Amazon"
                      value={field.state.value}
                      onBlur={field.handleBlur}
                      onChange={(e) => field.handleChange(e.target.value)}
                    />
                  </div>
                )}
              />
              <form.Field
                name="account"
                children={(field) => (
                  <div className="flex flex-1 flex-col gap-1.5 min-w-0 sm:min-w-40">
                    <Label>Set Category Account</Label>
                    <AccountInput
                      value={field.state.value}
                      onChange={(v) => field.handleChange(v)}
                      accounts={accountsData?.accounts ?? []}
                      placeholder="expenses:shopping"
                    />
                  </div>
                )}
              />
              <form.Field
                name="tags"
                children={(field) => (
                  <div className="flex flex-1 flex-col gap-1.5 min-w-0 sm:min-w-40">
                    <Label htmlFor="rule-tags">
                      Add Tags <span className="text-xs text-muted-foreground">key=val, key2</span>
                    </Label>
                    <Input
                      id="rule-tags"
                      type="text"
                      className="font-mono"
                      placeholder="source=import"
                      value={field.state.value}
                      onBlur={field.handleBlur}
                      onChange={(e) => field.handleChange(e.target.value)}
                    />
                  </div>
                )}
              />
              <form.Field
                name="priority"
                children={(field) => (
                  <div className="flex flex-1 flex-col gap-1.5 min-w-0 sm:min-w-24 sm:flex-none">
                    <Label htmlFor="rule-priority">Priority</Label>
                    <Input
                      id="rule-priority"
                      type="number"
                      value={field.state.value}
                      onBlur={field.handleBlur}
                      onChange={(e) => field.handleChange(e.target.value)}
                    />
                  </div>
                )}
              />
            </div>
            <form.Field
              name="autoReviewed"
              children={(field) => (
                <div className="flex items-center gap-2">
                  <Checkbox
                    id="rule-auto-reviewed"
                    checked={field.state.value}
                    onCheckedChange={(v) => field.handleChange(v)}
                  />
                  <Label htmlFor="rule-auto-reviewed">Auto-mark as reviewed on import</Label>
                </div>
              )}
            />
            <form.Subscribe
              selector={(state) => state.canSubmit}
              children={(canSubmit) => (
                <div className="flex gap-2">
                  <Button type="submit" size="sm" disabled={!canSubmit || saveRuleMutation.isPending}>
                    {saveRuleMutation.isPending && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
                    {saveRuleMutation.isPending ? "Saving…" : editingId ? "Update Rule" : "Add Rule"}
                  </Button>
                  {editingId && (
                    <Button type="button" variant="ghost" size="sm" onClick={cancelEdit}>
                      Cancel
                    </Button>
                  )}
                </div>
              )}
            />
          </form>
          {formError && <div className="mt-3"><ErrorBanner error={formError} /></div>}
        </CardContent>
      </Card>

      {/* Card 2: Rules list + Apply section */}
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-4">
            <CardTitle>Rules</CardTitle>
            <div className="flex w-full items-center gap-2 sm:w-auto">
              <Input
                type="text"
                placeholder="Filter rules…"
                className="h-8 min-w-0 flex-1 sm:w-48 sm:flex-none"
                value={globalFilter ?? ""}
                onChange={(e) => setGlobalFilter(e.target.value)}
              />
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
          </div>
        </CardHeader>
        <CardContent>
          {rulesLoading && <Loading />}
          {rulesError && <ErrorBanner error={rulesError} />}
          {rulesData && rules.length === 0 && (
            <p className="text-muted-foreground">No rules yet. Add one above.</p>
          )}
          {rulesData && rules.length > 0 && (
              <Table>
                <TableHeader>
                  {table.getHeaderGroups().map((headerGroup) => (
                    <TableRow key={headerGroup.id}>
                      {headerGroup.headers.map((header) => (
                        <TableHead key={header.id}>
                          {header.isPlaceholder
                            ? null
                            : flexRender(header.column.columnDef.header, header.getContext())}
                        </TableHead>
                      ))}
                    </TableRow>
                  ))}
                </TableHeader>
                <TableBody>
                  {table.getRowModel().rows.map((row) => (
                    <TableRow key={row.id} className={cn(editingId === row.original.id && "bg-primary/10")}>
                      {row.getVisibleCells().map((cell) =>
                        cell.column.id === "actions" ? (
                          <TableCell key={cell.id}>
                            <div className="flex gap-1">
                              <Button variant="ghost" size="xs" onClick={() => startEdit(row.original)}>
                                Edit
                              </Button>
                              <Button
                                variant="ghost"
                                size="xs"
                                className="text-destructive"
                                onClick={() => handleDelete(row.original.id)}
                              >
                                Delete
                              </Button>
                            </div>
                          </TableCell>
                        ) : (
                          <TableCell key={cell.id}>
                            {flexRender(cell.column.columnDef.cell, cell.getContext())}
                          </TableCell>
                        )
                      )}
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
