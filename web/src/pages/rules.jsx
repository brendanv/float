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
import { Check, ListFilter, Sparkles } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys, invalidateLedgerQueries } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { AccountInput } from "../components/posting-fields.jsx";
import { SuggestRulesWizard } from "../components/suggest-rules-wizard.jsx";
import { RuleIssuesDialog } from "../components/rule-issues-dialog.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { TableSortHeader } from "../components/table-sort-header.jsx";
import { TablePagination } from "../components/table-pagination.jsx";
import { Page, Section } from "../components/page.jsx";
import { Account } from "../components/account.jsx";
import { TagChip, tagLabel } from "../components/tag-editor.jsx";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Form, FormField } from "@/components/ui/form";
import { Checkbox } from "@/components/ui/checkbox";
import { Notice } from "@/components/ui/alert";
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
import { cn, ROW_ACTION } from "@/lib/utils";

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

function Dash({ children = "—" }) {
  return <span className="text-faint">{children}</span>;
}

// Change shows a preview's old value struck through and faint, then the new
// value in ink.
function Change({ from, children }) {
  return (
    <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
      {from && <span className="text-faint line-through [&_*]:text-faint [&_span]:line-through">{from}</span>}
      {from && <span aria-hidden="true" className="text-faint">→</span>}
      {children}
    </span>
  );
}

const rulesColumns = [
  columnHelper.accessor("priority", {
    header: ({ column }) => <TableSortHeader column={column}>P</TableSortHeader>,
    cell: ({ getValue }) => (
      <span className="block text-center tabular-nums text-muted-foreground">{getValue()}</span>
    ),
    sortingFn: "basic",
    meta: { headerClass: "w-10" },
  }),
  columnHelper.accessor("pattern", {
    header: ({ column }) => <TableSortHeader column={column}>Pattern</TableSortHeader>,
    cell: ({ getValue }) => (
      <span className="block max-w-xs truncate font-mono text-code" title={getValue()}>{getValue()}</span>
    ),
    filterFn: "includesString",
  }),
  columnHelper.accessor("payee", {
    header: ({ column }) => <TableSortHeader column={column}>Payee</TableSortHeader>,
    cell: ({ getValue }) =>
      getValue() ? <span className="font-medium">{getValue()}</span> : <Dash />,
    filterFn: "includesString",
  }),
  columnHelper.accessor("account", {
    header: ({ column }) => <TableSortHeader column={column}>Account</TableSortHeader>,
    cell: ({ getValue }) => (getValue() ? <Account name={getValue()} variant="path" /> : <Dash />),
    filterFn: "includesString",
  }),
  columnHelper.accessor("matchAccount", {
    header: ({ column }) => <TableSortHeader column={column}>Source account</TableSortHeader>,
    cell: ({ getValue }) => (getValue() ? <Account name={getValue()} variant="path" /> : <Dash>all</Dash>),
    filterFn: "includesString",
  }),
  columnHelper.accessor((row) => tagsToString(row.tags), {
    id: "tags",
    header: "Tags",
    cell: ({ row }) => {
      const entries = Object.entries(row.original.tags ?? {});
      if (entries.length === 0) return <Dash />;
      return (
        <span className="flex flex-wrap gap-1">
          {entries.map(([k, v]) => <TagChip key={k}>{tagLabel(k, v)}</TagChip>)}
        </span>
      );
    },
    enableSorting: false,
  }),
  columnHelper.accessor("autoReviewed", {
    header: "Auto-review",
    cell: ({ getValue }) =>
      getValue() ? (
        <Check aria-label="Auto-marks reviewed" className="size-3.5 text-muted-foreground" />
      ) : (
        <Dash />
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
  const [issuesDialogOpen, setIssuesDialogOpen] = useState(false);
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
  const [previewError, setPreviewError] = useState(null);
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
    setPreviewError(null);
    setApplyLoading(true);
    setApplyPreviews(null);
    setApplyResult(null);
    try {
      const res = await ledgerClient.previewApplyRules({ ruleIds: [], query: [] });
      setApplyPreviews(res.previews);
      setSelectedFids(new Set(res.previews.map((p) => p.fid)));
      setDrawerOpen(true);
    } catch (err) {
      setPreviewError(err);
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
          invalidateLedgerQueries(queryClient);
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
    <Page>
      <PageHeader title="Rules" />

      <Section title={editingId ? "Edit rule" : "Add rule"}>
        <Form
          className="gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            e.stopPropagation();
            form.handleSubmit();
          }}
        >
          {formError && <ErrorBanner error={formError} />}
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
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
            <form.Field
              name="payee"
              children={(field) => (
                <FormField label="Set payee" htmlFor="rule-payee">
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
                <FormField label="Set category account">
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
                <FormField label="Match source account" hint="optional">
                  <AccountInput
                    value={field.state.value}
                    onChange={(v) => field.handleChange(v)}
                    accounts={accountsData?.accounts ?? []}
                    includeAccountPrefixes={["assets", "liabilities"]}
                    placeholder="all accounts"
                    clearable
                    clearLabel="All accounts"
                  />
                </FormField>
              )}
            />
          </div>
          <div className="flex flex-wrap items-end gap-x-3 gap-y-3">
            <form.Field
              name="tags"
              children={(field) => (
                <FormField label="Add tags" htmlFor="rule-tags" hint="key=val, key2" className="w-full sm:w-64">
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
                <FormField label="Priority" htmlFor="rule-priority" className="w-24">
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
            <form.Field
              name="autoReviewed"
              children={(field) => (
                <div className="flex h-8 items-center gap-2">
                  <Checkbox
                    id="rule-auto-reviewed"
                    checked={field.state.value}
                    onCheckedChange={(v) => field.handleChange(v)}
                  />
                  <Label htmlFor="rule-auto-reviewed" className="font-normal">Auto-mark as reviewed on import</Label>
                </div>
              )}
            />
            <form.Subscribe
              selector={(state) => state.canSubmit}
              children={(canSubmit) => (
                <div className="ml-auto flex items-center gap-2">
                  {editingId && (
                    <Button type="button" variant="ghost" onClick={cancelEdit}>
                      Cancel
                    </Button>
                  )}
                  <Button
                    type="submit"
                    disabled={!canSubmit}
                    isLoading={saveRuleMutation.isPending}
                    loadingText="Saving…"
                  >
                    {editingId ? "Update rule" : "Add rule"}
                  </Button>
                </div>
              )}
            />
          </div>
        </Form>
      </Section>

      <Section
        title="Rules"
        description={rules.length > 0 ? rules.length : null}
        action={
          <div className="flex w-full flex-wrap items-center gap-2 sm:w-auto sm:justify-end">
            <Input
              type="text"
              placeholder="Filter rules…"
              aria-label="Filter rules"
              className="h-7 w-full sm:w-48"
              value={globalFilter ?? ""}
              onChange={(e) => setGlobalFilter(e.target.value)}
            />
            <Button variant="outline" size="sm" onClick={() => setWizardOpen(true)}>
              <Sparkles data-icon="inline-start" />
              Suggest Rules
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setIssuesDialogOpen(true)}
              disabled={rules.length < 2}
            >
              <Sparkles data-icon="inline-start" />
              Find Issues
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
        }
      >
        {applyResult !== null && (
          <Notice variant="info">Applied changes to {applyResult} transaction(s).</Notice>
        )}
        {/* A failed preview never opens the drawer, so its error shows here. */}
        {previewError && (
          <Notice variant="critical" data-slot="preview-error">
            Couldn&rsquo;t preview changes: {(previewError.message || String(previewError)).replace(/^\[[^\]]+\]\s*/, "")}
          </Notice>
        )}
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
            <Table>
              <TableHeader>
                {table.getHeaderGroups().map((headerGroup) => (
                  <TableRow key={headerGroup.id} className="hover:bg-transparent">
                    {headerGroup.headers.map((header) => (
                      <TableHead key={header.id} className={header.column.columnDef.meta?.headerClass}>
                        {header.isPlaceholder
                          ? null
                          : flexRender(header.column.columnDef.header, header.getContext())}
                      </TableHead>
                    ))}
                  </TableRow>
                ))}
              </TableHeader>
              <TableBody>
                {table.getRowModel().rows.map((row) => {
                  const editing = editingId === row.original.id;
                  return (
                    <TableRow
                      key={row.id}
                      data-state={editing ? "selected" : undefined}
                      className={cn("group/row", editing && "bg-accent hover:bg-accent data-[state=selected]:bg-accent")}
                    >
                      {row.getVisibleCells().map((cell) =>
                        cell.column.id === "actions" ? (
                          <TableCell key={cell.id} className="py-1">
                            <div className={cn("flex justify-end gap-1", !editing && ROW_ACTION)}>
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
                          <TableCell key={cell.id} className="py-1">
                            {flexRender(cell.column.columnDef.cell, cell.getContext())}
                          </TableCell>
                        )
                      )}
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
            <TablePagination table={table} pageSizeOptions={[5, 10, 25, 50]} />
          </>
        )}
        <div data-slot="rule-test" className="flex flex-wrap items-center gap-x-3 gap-y-2 border-t border-border pt-3">
          <Label htmlFor="rule-test-desc" className="font-normal whitespace-nowrap text-muted-foreground">
            Test a description
          </Label>
          <Input
            id="rule-test-desc"
            type="text"
            className="min-w-0 flex-1 basis-56 font-mono"
            placeholder="AMAZON.COM PURCHASE"
            value={testDesc}
            onChange={(e) => setTestDesc(e.target.value)}
          />
          {testDesc && (
            matchingRule ? (
              <span className="inline-flex shrink-0 items-center gap-1.5 rounded-sm bg-accent px-2 py-0.5 text-control">
                <span className="text-muted-foreground">Matches</span>
                <span className="font-medium">{matchingRule.payee || matchingRule.account}</span>
              </span>
            ) : (
              <span className="shrink-0 text-control text-faint">No match</span>
            )
          )}
        </div>
      </Section>

      <SuggestRulesWizard
        open={wizardOpen}
        onOpenChange={setWizardOpen}
        accounts={accountsData?.accounts ?? []}
        onRulesAdded={() => queryClient.invalidateQueries({ queryKey: queryKeys.rules() })}
      />

      <RuleIssuesDialog
        open={issuesDialogOpen}
        onOpenChange={setIssuesDialogOpen}
        rules={rules}
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
                  <TableRow className="hover:bg-transparent">
                    <TableHead>
                      <Checkbox
                        aria-label="Select all"
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
                      <TableCell className="py-1">
                        <Checkbox
                          aria-label={`Apply to ${p.description}`}
                          checked={selectedFids.has(p.fid)}
                          onCheckedChange={() => toggleFid(p.fid)}
                        />
                      </TableCell>
                      <TableCell className="py-1">{p.description}</TableCell>
                      <TableCell className="py-1">
                        {p.newAccount ? (
                          <Change from={p.currentAccount ? <Account name={p.currentAccount} variant="path" /> : null}>
                            <Account name={p.newAccount} variant="path" />
                          </Change>
                        ) : (
                          <Dash />
                        )}
                      </TableCell>
                      <TableCell className="py-1">
                        {p.newPayee ? (
                          <Change from={p.currentPayee}>
                            <span className="font-medium">{p.newPayee}</span>
                          </Change>
                        ) : (
                          <Dash />
                        )}
                      </TableCell>
                      <TableCell className="py-1 text-muted-foreground">
                        {p.willMarkReviewed ? "Will mark reviewed" : <Dash />}
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
    </Page>
  );
}
