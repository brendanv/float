import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  getPaginationRowModel,
  createColumnHelper,
  flexRender,
} from "@tanstack/react-table";
import { Pencil, Trash2, Plus, X, LayoutGrid } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { AccountInput } from "../components/posting-fields.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { TableSortHeader } from "../components/table-sort-header.jsx";
import { TablePagination } from "../components/table-pagination.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Form, FormField, FormRow, FormActions } from "@/components/ui/form";
import { Badge } from "@/components/ui/badge";
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
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

const columnHelper = createColumnHelper();

function emptyPosting() {
  return { account: "", commodity: "USD", defaultQuantity: "", comment: "" };
}

function TemplatePostingRow({ posting, index, onChange, onRemove, canRemove, accounts }) {
  return (
    <div className="flex items-start gap-2">
      <div className="flex flex-1 flex-wrap items-start gap-2">
        <AccountInput
          value={posting.account}
          onChange={(v) => onChange({ ...posting, account: v })}
          accounts={accounts}
          placeholder="Account"
          className="min-w-48 flex-1"
        />
        <Input
          placeholder="Commodity (e.g. USD)"
          value={posting.commodity}
          onChange={(e) => onChange({ ...posting, commodity: e.target.value })}
          className="w-28"
        />
        <Input
          type="number"
          step="any"
          placeholder="Default qty (blank = auto)"
          value={posting.defaultQuantity}
          onChange={(e) => onChange({ ...posting, defaultQuantity: e.target.value })}
          className="w-44"
        />
      </div>
      {canRemove && (
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          onClick={onRemove}
          className="mt-1 shrink-0 text-muted-foreground hover:text-destructive"
          title="Remove posting"
        >
          <X className="size-3.5" />
        </Button>
      )}
    </div>
  );
}

function TemplateForm({ initial, onSave, onCancel, accounts }) {
  const [name, setName] = useState(initial?.name ?? "");
  const [payee, setPayee] = useState(initial?.payee ?? "");
  const [note, setNote] = useState(initial?.note ?? "");
  const [postings, setPostings] = useState(
    initial?.postings?.length >= 2
      ? initial.postings.map((p) => ({ ...p }))
      : [emptyPosting(), emptyPosting()]
  );
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState(null);

  function updatePosting(i, updated) {
    setPostings((ps) => ps.map((p, idx) => (idx === i ? updated : p)));
  }

  function removePosting(i) {
    setPostings((ps) => ps.filter((_, idx) => idx !== i));
  }

  function addPosting() {
    setPostings((ps) => [...ps, emptyPosting()]);
  }

  async function handleSubmit(e) {
    e.preventDefault();
    const filled = postings.filter((p) => p.account.trim());
    if (filled.length < 2) {
      setError(new Error("At least 2 postings with accounts are required."));
      return;
    }
    setError(null);
    setSubmitting(true);
    try {
      await onSave({
        name: name.trim(),
        payee: payee.trim(),
        note: note.trim(),
        postings: filled.map((p) => ({
          account: p.account.trim(),
          commodity: p.commodity.trim(),
          defaultQuantity: p.defaultQuantity.trim(),
          comment: p.comment?.trim() ?? "",
        })),
        tags: {},
      });
    } catch (err) {
      setError(err);
      setSubmitting(false);
    }
  }

  return (
    <Form onSubmit={handleSubmit}>
      {error && <ErrorBanner error={error} />}
      <FormRow cols={2}>
        <FormField label="Template name" htmlFor="tmpl-name" className="sm:col-span-1">
          <Input
            id="tmpl-name"
            placeholder="e.g. Mortgage Payment"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
          />
        </FormField>
        <FormField label="Default payee" htmlFor="tmpl-payee" className="sm:col-span-1">
          <Input
            id="tmpl-payee"
            placeholder="e.g. Bank of America"
            value={payee}
            onChange={(e) => setPayee(e.target.value)}
          />
        </FormField>
      </FormRow>
      <FormField label="Default note" htmlFor="tmpl-note">
        <Input
          id="tmpl-note"
          placeholder="e.g. Mortgage (appears after | in description)"
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
      </FormField>
      <FormField label="Postings">
        <p className="mb-2 text-xs text-muted-foreground">
          Leave &ldquo;Default qty&rdquo; blank on one posting to let hledger auto-balance it.
        </p>
        <div className="flex flex-col gap-2">
          {postings.map((p, i) => (
            <TemplatePostingRow
              key={i}
              posting={p}
              index={i}
              onChange={(updated) => updatePosting(i, updated)}
              onRemove={() => removePosting(i)}
              canRemove={postings.length > 2}
              accounts={accounts}
            />
          ))}
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="self-start"
            onClick={addPosting}
          >
            <Plus data-icon="inline-start" /> Add posting
          </Button>
        </div>
      </FormField>
      <FormActions>
        {onCancel && (
          <Button type="button" variant="outline" onClick={onCancel} disabled={submitting}>
            Cancel
          </Button>
        )}
        <Button type="submit" isLoading={submitting} loadingText="Saving…">
          {initial ? "Update Template" : "Save Template"}
        </Button>
      </FormActions>
    </Form>
  );
}

function postingSummary(postings) {
  if (!postings || postings.length === 0) return "—";
  return postings.map((p) => p.account).join(" · ");
}

export function TemplatesPage() {
  const queryClient = useQueryClient();
  const [editingTemplate, setEditingTemplate] = useState(null);
  const [deleteTarget, setDeleteTarget] = useState(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState(null);
  const [showForm, setShowForm] = useState(false);

  const { data: templatesData, isLoading, error } = useQuery({
    queryKey: queryKeys.templates(),
    queryFn: () => ledgerClient.listTemplates({}),
  });

  const { data: accountsData } = useQuery({
    queryKey: queryKeys.accounts(),
    queryFn: () => ledgerClient.listAccounts({}),
  });

  const accounts = accountsData?.accounts ?? [];
  const templatesList = templatesData?.templates ?? [];

  const addMutation = useMutation({
    mutationFn: (req) => ledgerClient.addTemplate(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.templates() });
      setShowForm(false);
    },
  });

  const updateMutation = useMutation({
    mutationFn: (req) => ledgerClient.updateTemplate(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.templates() });
      setEditingTemplate(null);
    },
  });

  async function handleDelete() {
    if (!deleteTarget) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      await ledgerClient.deleteTemplate({ id: deleteTarget.id });
      queryClient.invalidateQueries({ queryKey: queryKeys.templates() });
      setDeleteTarget(null);
    } catch (err) {
      setDeleteError(err);
    } finally {
      setDeleting(false);
    }
  }

  const columns = [
    columnHelper.accessor("name", {
      header: ({ column }) => <TableSortHeader column={column}>Name</TableSortHeader>,
      cell: ({ getValue }) => <span className="font-medium">{getValue()}</span>,
    }),
    columnHelper.accessor("payee", {
      header: "Payee",
      cell: ({ getValue }) => getValue() || <span className="text-muted-foreground">—</span>,
    }),
    columnHelper.accessor("postings", {
      header: "Accounts",
      cell: ({ getValue }) => (
        <span className="font-mono text-xs text-muted-foreground">{postingSummary(getValue())}</span>
      ),
      enableSorting: false,
    }),
    columnHelper.display({
      id: "actions",
      header: "",
      cell: ({ row }) => (
        <div className="flex justify-end gap-1">
          <Button
            variant="ghost"
            size="icon-xs"
            title="Edit template"
            onClick={() => setEditingTemplate(row.original)}
          >
            <Pencil className="size-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon-xs"
            title="Delete template"
            className="text-muted-foreground hover:text-destructive"
            onClick={() => setDeleteTarget(row.original)}
          >
            <Trash2 className="size-3.5" />
          </Button>
        </div>
      ),
      meta: { headerClass: "w-24" },
    }),
  ];

  const table = useReactTable({
    data: templatesList,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    initialState: { pagination: { pageSize: 25 } },
  });

  if (isLoading) return <Loading />;
  if (error) return <ErrorBanner error={error} />;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Transaction Templates"
        description="Save recurring transaction shapes for bulk entry."
      />

      {/* Create / edit form */}
      {(showForm || editingTemplate) && (
        <Card>
          <CardHeader>
            <CardTitle>{editingTemplate ? "Edit Template" : "New Template"}</CardTitle>
          </CardHeader>
          <CardContent>
            <TemplateForm
              initial={editingTemplate ?? undefined}
              accounts={accounts}
              onSave={async (data) => {
                if (editingTemplate) {
                  await updateMutation.mutateAsync({ id: editingTemplate.id, ...data });
                } else {
                  await addMutation.mutateAsync(data);
                }
              }}
              onCancel={() => {
                setShowForm(false);
                setEditingTemplate(null);
              }}
            />
          </CardContent>
        </Card>
      )}

      {/* Templates list */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>Templates</CardTitle>
          {!showForm && !editingTemplate && (
            <Button size="sm" onClick={() => setShowForm(true)}>
              <Plus data-icon="inline-start" /> New Template
            </Button>
          )}
        </CardHeader>
        <CardContent>
          {templatesList.length === 0 ? (
            <EmptyState
              icon={LayoutGrid}
              title="No templates yet"
              description="Create a template to save a transaction shape for quick entry."
            />
          ) : (
            <>
              <Table>
                <TableHeader>
                  {table.getHeaderGroups().map((hg) => (
                    <TableRow key={hg.id}>
                      {hg.headers.map((h) => (
                        <TableHead
                          key={h.id}
                          className={cn(h.column.columnDef.meta?.headerClass)}
                        >
                          {flexRender(h.column.columnDef.header, h.getContext())}
                        </TableHead>
                      ))}
                    </TableRow>
                  ))}
                </TableHeader>
                <TableBody>
                  {table.getRowModel().rows.map((row) => (
                    <TableRow key={row.id}>
                      {row.getVisibleCells().map((cell) => (
                        <TableCell key={cell.id}>
                          {flexRender(cell.column.columnDef.cell, cell.getContext())}
                        </TableCell>
                      ))}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              <TablePagination table={table} />
            </>
          )}
        </CardContent>
      </Card>

      {/* Delete confirmation dialog */}
      <Dialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>Delete template?</DialogTitle>
            <DialogDescription>
              This will permanently delete &ldquo;{deleteTarget?.name}&rdquo;. Existing transactions are not affected.
            </DialogDescription>
          </DialogHeader>
          {deleteError && <ErrorBanner error={deleteError} />}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)} disabled={deleting}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDelete} isLoading={deleting}>
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
