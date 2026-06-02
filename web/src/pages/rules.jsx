import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "@tanstack/react-form";
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  createColumnHelper,
  flexRender,
} from "@tanstack/react-table";
import { CircleCheck, ListFilter, Sparkles } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { AccountInput } from "../components/posting-fields.jsx";
import { SuggestRulesWizard } from "../components/suggest-rules-wizard.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { TableSortHeader } from "../components/table-sort-header.jsx";
import { TablePagination } from "../components/table-pagination.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Form, FormField, FormRow, FormActions } from "@/components/ui/form";
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
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@/components/ui/drawer";
import { cn } from "@/lib/utils";

function emptyForm() {
  return { pattern: "", payee: "", account: "", matchAccount: "", priority: "0", tags: "", autoReviewed: true };
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

const columnHelper = createColumnHelper();

const rulesColumns = [
  columnHelper.accessor("priority", {
    header: ({ column }) => <TableSortHeader column={column}>P</TableSortHeader>,
    cell: ({ getValue }) => (
      <Badge variant="secondary" className="font-mono">{getValue()}</Badge>
    ),
    sortingFn: "basic",
    meta: { headerClass: "w-10 text-center" },
  }),
  columnHelper.accessor("pattern", {
    header: ({ column }) => <TableSortHeader column={column}>Pattern</TableSortHeader>,
    cell: ({ getValue }) => (
      <span className="max-w-xs truncate font-mono text-xs" title={getValue()}>{getValue()}</span>
    ),
    filterFn: "includesString",
  }),
  columnHelper.accessor("payee", {
    header: ({ column }) => <TableSortHeader column={column}>Payee</TableSortHeader>,
    cell: ({ getValue }) =>
      getValue() || <span className="text-muted-foreground/60">—</span>,
    filterFn: "includesString",
  }),
  columnHelper.accessor("account", {
    header: ({ column }) => <TableSortHeader column={column}>Account</TableSortHeader>,
    cell: ({ getValue }) =>
      getValue() ? (
        <span className="font-mono text-xs">{getValue()}</span>
      ) : (
        <span className="text-muted-foreground/60">—</span>
      ),
    filterFn: "includesString",
  }),
  columnHelper.accessor("matchAccount", {
    header: ({ column }) => <TableSortHeader column={column}>Source Account</TableSortHeader>,
    cell: ({ getValue }) =>
      getValue() ? (
        <span className="font-mono text-xs">{getValue()}</span>
      ) : (
        <span className="text-muted-foreground/60">all</span>
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

  const [wizardOpen, setWizardOpen] = useState(false);
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
        matchAccount: value.matchAccount,
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
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 25 });

  const rules = useMemo(() => rulesData?.rules ?? [], [rulesData]);

  const table = useReactTable({
    data: rules,
    columns: rulesColumns,
    state: { sorting, globalFilter, pagination },
    onSortingChange: setSorting,
    onGlobalFilterChange: (filter) => {
      setGlobalFilter(filter);
      setPagination((p) => ({ ...p, pageIndex: 0 }));
    },
    onPaginationChange: setPagination,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
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
  const [applyProgress, setApplyProgress] = useState(null);
  const [applyResult, setApplyResult] = useState(null);
  const [drawerOpen, setDrawerOpen] = useState(false);

  const saveRuleMutation = useMutation({
    mutationFn: (payload) =>
      editingId
        ? ledgerClient.updateRule({ id: editingId, ...payload })
        : ledgerClient.addRule({ rules: [payload] }),
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
    form.setFieldValue("matchAccount", rule.matchAccount ?? "");
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
      setDrawerOpen(true);
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
    setApplyProgress({ applied: 0, total: selectedFids.size });
    try {
      for await (const res of ledgerClient.applyRules({
        fids: Array.from(selectedFids),
        ruleIds: [],
        query: [],
      })) {
        if (res.payload.case === "progress") {
          setApplyProgress({
            applied: res.payload.value.applied,
            total: res.payload.value.total,
          });
        } else if (res.payload.case === "result") {
          setApplyResult(res.payload.value.appliedCount);
          setApplyPreviews(null);
          setDrawerOpen(false);
        }
      }
    } catch (err) {
      setApplyError(err);
    } finally {
      setApplying(false);
      setApplyProgress(null);
    }
  }

  const matchingRule = getMatchingRule();

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title="Categorization Rules" />

      {/* Card 1: Rule Editor */}
      <Card>
        <CardHeader>
          <CardTitle>{editingId ? "Edit Rule" : "Add Rule"}</CardTitle>
        </CardHeader>
        <CardContent>
          <Form
            onSubmit={(e) => {
              e.preventDefault();
              e.stopPropagation();
              form.handleSubmit();
            }}
          >
            {formError && <ErrorBanner error={formError} />}
            <form.Field
              name="pattern"
              validators={{
                onChange: ({ value }) => (!value ? "Pattern is required" : undefined),
              }}
              children={(field) => (
                <FormField
                  label="Pattern (regex)"
                  htmlFor="rule-pattern"
                  error={
                    field.state.meta.isTouched && !field.state.meta.isValid
                      ? field.state.meta.errors.join(", ")
                      : null
                  }
                >
                  <Input
                    id="rule-pattern"
                    type="text"
                    className="font-mono"
                    placeholder="AMAZON|amazon\.com"
                    value={field.state.value}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                  />
                </FormField>
              )}
            />
            <FormRow cols={3}>
              <form.Field
                name="payee"
                children={(field) => (
                  <FormField label="Set Payee" htmlFor="rule-payee">
                    <Input
                      id="rule-payee"
                      type="text"
                      placeholder="Amazon"
                      value={field.state.value}
                      onBlur={field.handleBlur}
                      onChange={(e) => field.handleChange(e.target.value)}
                    />
                  </FormField>
                )}
              />
              <form.Field
                name="account"
                children={(field) => (
                  <FormField label="Set Category Account">
                    <AccountInput
                      value={field.state.value}
                      onChange={(v) => field.handleChange(v)}
                      accounts={accountsData?.accounts ?? []}
                      includeAccountPrefixes={["expenses", "income"]}
                      placeholder="expenses:shopping"
                    />
                  </FormField>
                )}
              />
              <form.Field
                name="matchAccount"
                children={(field) => (
                  <FormField label="Match Source Account" hint="optional">
                    <AccountInput
                      value={field.state.value}
                      onChange={(v) => field.handleChange(v)}
                      accounts={accountsData?.accounts ?? []}
                      includeAccountPrefixes={["assets", "liabilities"]}
                      placeholder="all accounts"
                    />
                  </FormField>
                )}
              />
            </FormRow>
            <FormRow cols={2}>
              <form.Field
                name="tags"
                children={(field) => (
                  <FormField label="Add Tags" htmlFor="rule-tags" hint="key=val, key2">
                    <Input
                      id="rule-tags"
                      type="text"
                      className="font-mono"
                      placeholder="source=import"
                      value={field.state.value}
                      onBlur={field.handleBlur}
                      onChange={(e) => field.handleChange(e.target.value)}
                    />
                  </FormField>
                )}
              />
              <form.Field
                name="priority"
                children={(field) => (
                  <FormField label="Priority" htmlFor="rule-priority">
                    <Input
                      id="rule-priority"
                      type="number"
                      value={field.state.value}
                      onBlur={field.handleBlur}
                      onChange={(e) => field.handleChange(e.target.value)}
                    />
                  </FormField>
                )}
              />
            </FormRow>
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
                <FormActions align={editingId ? "between" : "end"}>
                  {editingId && (
                    <Button type="button" variant="ghost" size="sm" onClick={cancelEdit}>
                      Cancel
                    </Button>
                  )}
                  <Button
                    type="submit"
                    size="sm"
                    disabled={!canSubmit}
                    isLoading={saveRuleMutation.isPending}
                    loadingText="Saving…"
                  >
                    {editingId ? "Update Rule" : "Add Rule"}
                  </Button>
                </FormActions>
              )}
            />
          </Form>
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
                onClick={() => setWizardOpen(true)}
              >
                <Sparkles data-icon="inline-start" className="size-3.5" />
                Suggest Rules
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={handlePreviewApply}
                isLoading={applyLoading}
                loadingText="Previewing…"
              >
                Preview Changes
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {rulesLoading && <Loading />}
          {rulesError && <ErrorBanner error={rulesError} />}
          {rulesData && rules.length === 0 && (
            <EmptyState
              icon={ListFilter}
              title="No rules yet"
              description="Add one above to start automatically categorizing transactions during import."
            />
          )}
          {rulesData && rules.length > 0 && (
            <>
              <div className="overflow-x-auto">
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
                                variant="destructive-ghost"
                                size="xs"
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
              </div>
              <TablePagination table={table} pageSizeOptions={[5, 10, 25, 50]} />
            </>
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
          {applyResult !== null && (
            <Alert className="mt-3">
              <CircleCheck className="size-4 text-success" />
              <AlertDescription>
                Applied changes to {applyResult} transaction(s).
              </AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>

      <SuggestRulesWizard
        open={wizardOpen}
        onOpenChange={setWizardOpen}
        accounts={accountsData?.accounts ?? []}
        onRulesAdded={() => queryClient.invalidateQueries({ queryKey: queryKeys.rules() })}
      />

      <Drawer open={drawerOpen} onOpenChange={setDrawerOpen} direction="bottom">
        <DrawerContent className="max-h-[80vh]">
          <DrawerHeader className="text-left">
            <DrawerTitle>Preview Changes</DrawerTitle>
            {applyPreviews && (
              <DrawerDescription>
                {applyPreviews.length === 0
                  ? "No transactions match any rules."
                  : `${applyPreviews.length} transaction(s) will be updated by the current rules.`}
              </DrawerDescription>
            )}
          </DrawerHeader>
          {applyPreviews && applyPreviews.length > 0 && (
            <div className="overflow-y-auto px-4">
              <div className="overflow-x-auto">
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
                    <TableHead>Reviewed</TableHead>
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
                      <TableCell className="text-xs">
                        {p.willMarkReviewed ? (
                          <span className="text-success">Will mark reviewed</span>
                        ) : (
                          <span className="text-muted-foreground/60">—</span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              </div>
            </div>
          )}
          <DrawerFooter>
            {applyError && <ErrorBanner error={applyError} />}
            {applyPreviews && applyPreviews.length > 0 && (
              <Button
                size="sm"
                onClick={handleApply}
                disabled={selectedFids.size === 0}
                isLoading={applying}
                loadingText={
                  applyProgress && applyProgress.total > 0
                    ? `Applying ${applyProgress.applied} of ${applyProgress.total}…`
                    : "Applying…"
                }
              >
                Apply to {selectedFids.size} Transaction(s)
              </Button>
            )}
            <DrawerClose asChild>
              <Button variant="outline" size="sm">Cancel</Button>
            </DrawerClose>
          </DrawerFooter>
        </DrawerContent>
      </Drawer>
    </div>
  );
}
