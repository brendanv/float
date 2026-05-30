import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { BookOpen } from "lucide-react";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { AccountInput } from "../components/posting-fields.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { Page, PageCard } from "../components/page.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@/components/ui/collapsible";
import { Input } from "@/components/ui/input";
import { ChevronRight, ChevronDown, Pencil } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Form, FormField, FormRow, FormActions } from "@/components/ui/form";
import { cn } from "@/lib/utils";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

function buildTree(declarations) {
  const byName = new Map(declarations.map((d) => [d.name, d]));
  const children = new Map();

  function ensureNode(name) {
    if (!byName.has(name)) {
      byName.set(name, { name, hasPostings: null });
    }
    const parts = name.split(":");
    if (parts.length > 1) {
      const parent = parts.slice(0, -1).join(":");
      ensureNode(parent);
      if (!children.has(parent)) children.set(parent, []);
      const siblings = children.get(parent);
      if (!siblings.includes(name)) siblings.push(name);
    }
  }

  for (const d of [...declarations].sort((a, b) => a.name.localeCompare(b.name))) {
    ensureNode(d.name);
  }

  for (const kids of children.values()) kids.sort();

  const roots = [...byName.keys()].filter((n) => !n.includes(":")).sort();
  return { byName, children, roots };
}

function AccountTreeNode({ name, byName, children, depth, onDelete, onRename, deletingName, declaredNames }) {
  const [expanded, setExpanded] = useState(true);
  const kids = children.get(name) ?? [];
  const decl = byName.get(name);
  const hasKids = kids.length > 0;
  const label = name.split(":").at(-1);

  return (
    <div>
      <div
        className="flex items-center gap-1 py-0.5 rounded hover:bg-muted/50 group"
        style={{ paddingLeft: `${depth * 1.25 + 0.25}rem` }}
      >
        <button
          className="flex items-center justify-center w-4 h-4 shrink-0"
          onClick={() => hasKids && setExpanded((e) => !e)}
          tabIndex={hasKids ? 0 : -1}
          aria-label={expanded ? "Collapse" : "Expand"}
        >
          {hasKids && (
            <ChevronRight
              className={`size-3 text-muted-foreground transition-transform ${expanded ? "rotate-90" : ""}`}
            />
          )}
        </button>
        <div className="flex flex-1 items-center gap-1 min-w-0">
          <span className="font-mono text-sm truncate" title={name}>{label}</span>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={`Rename ${name}`}
            title={`Rename ${name}`}
            onClick={() => onRename(name)}
          >
            <Pencil className="size-3" />
          </Button>
        </div>
        {declaredNames.has(name) && !decl?.hasPostings && (
          <Button
            variant="destructive-ghost"
            size="xs"
            className="opacity-0 group-hover:opacity-100 transition-opacity mr-1"
            isLoading={deletingName === name}
            onClick={() => onDelete(name)}
          >
            Delete
          </Button>
        )}
      </div>
      {hasKids && expanded && (
        <div>
          {kids.map((child) => (
            <AccountTreeNode
              key={child}
              name={child}
              byName={byName}
              children={children}
              depth={depth + 1}
              onDelete={onDelete}
              onRename={onRename}
              deletingName={deletingName}
              declaredNames={declaredNames}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function RootAccountCard({ root, rootDecl, kids, byName, childrenMap, declaredNames, deletingName, onDelete, onRename }) {
  const [open, setOpen] = useState(true);
  const canDelete = declaredNames.has(root) && !rootDecl?.hasPostings;
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between gap-2">
            <div className="flex flex-1 items-center gap-1 min-w-0">
              <CardTitle className="capitalize truncate" title={root}>{root}</CardTitle>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={`Rename ${root}`}
                title={`Rename ${root}`}
                onClick={() => onRename(root)}
              >
                <Pencil className="size-3" />
              </Button>
              <CollapsibleTrigger
                className="flex items-center justify-center size-6 shrink-0 text-muted-foreground hover:text-foreground"
                aria-label={open ? `Collapse ${root}` : `Expand ${root}`}
              >
                <ChevronDown
                  className={cn(
                    "size-4 transition-transform duration-200",
                    open && "rotate-180",
                  )}
                />
              </CollapsibleTrigger>
            </div>
            {canDelete && (
              <Button
                variant="destructive-ghost"
                size="xs"
                isLoading={deletingName === root}
                onClick={() => onDelete(root)}
              >
                Delete
              </Button>
            )}
          </div>
        </CardHeader>
        <CollapsibleContent>
          <CardContent>
            {kids.length > 0 ? (
              <div>
                {kids.map((child) => (
                  <AccountTreeNode
                    key={child}
                    name={child}
                    byName={byName}
                    children={childrenMap}
                    depth={0}
                    onDelete={onDelete}
                    onRename={onRename}
                    deletingName={deletingName}
                    declaredNames={declaredNames}
                  />
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">No sub-accounts declared.</p>
            )}
          </CardContent>
        </CollapsibleContent>
      </Card>
    </Collapsible>
  );
}

export function AccountsPage() {
  const queryClient = useQueryClient();

  const { data, isLoading, error: fetchError } = useQuery({
    queryKey: queryKeys.accountDeclarations(),
    queryFn: () => ledgerClient.listAccountDeclarations({}),
  });

  const [name, setName] = useState("");
  const [formError, setFormError] = useState(null);
  const [deletingName, setDeletingName] = useState(null);
  const [renameOpen, setRenameOpen] = useState(false);
  const [renameAccountName, setRenameAccountName] = useState("");

  const addMutation = useMutation({
    mutationFn: (vars) => ledgerClient.declareAccount(vars),
    onSuccess: () => {
      setName("");
      setFormError(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.accountDeclarations() });
    },
    onError: (err) => setFormError(err),
  });

  const deleteMutation = useMutation({
    mutationFn: ({ name }) => ledgerClient.deleteAccountDeclaration({ name }),
    onSuccess: () => {
      setDeletingName(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.accountDeclarations() });
    },
    onError: (err) => {
      setDeletingName(null);
      setFormError(err);
    },
  });

  function handleSubmit(e) {
    e.preventDefault();
    setFormError(null);
    addMutation.mutate({ name: name.trim() });
  }

  function handleDelete(name) {
    setDeletingName(name);
    setFormError(null);
    deleteMutation.mutate({ name });
  }

  function handleRename(name) {
    setRenameAccountName(name);
    setFormError(null);
    setRenameOpen(true);
  }

  const declarations = data?.declarations ?? [];
  const declaredNames = new Set(declarations.map((d) => d.name));
  const { byName, children, roots } = buildTree(declarations);

  return (
    <Page>
      <PageHeader title="Account Declarations" />

      <PageCard title="Declare Account">
        <Form onSubmit={handleSubmit}>
          {formError && <ErrorBanner error={formError} />}
          <FormRow cols={2}>
            <FormField
              label="Account Name"
              htmlFor="acct-name"
              description="Colon-separated hierarchy, e.g. assets:bank:checking"
            >
              <Input
                id="acct-name"
                type="text"
                placeholder="assets:checking"
                className="font-mono"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </FormField>
          </FormRow>
          <FormActions>
            <Button
              type="submit"
              isLoading={addMutation.isPending}
              loadingText="Declaring..."
            >
              Declare Account
            </Button>
          </FormActions>
        </Form>
      </PageCard>

      {isLoading && <Loading />}
      {fetchError && <ErrorBanner error={fetchError} />}
      {data && declarations.length === 0 && (
        <Card>
          <CardContent>
            <EmptyState
              icon={BookOpen}
              title="No account declarations yet"
              description="Declare accounts to get type-aware classification (assets, liabilities, income, expenses, equity)."
            />
          </CardContent>
        </Card>
      )}
      {data && declarations.length > 0 && roots.map((root) => {
        const rootDecl = byName.get(root);
        const kids = children.get(root) ?? [];
        return (
          <RootAccountCard
            key={root}
            root={root}
            rootDecl={rootDecl}
            kids={kids}
            byName={byName}
            childrenMap={children}
            declaredNames={declaredNames}
            deletingName={deletingName}
            onDelete={handleDelete}
            onRename={handleRename}
          />
        );
      })}

      <RenameAccountDialog
        open={renameOpen}
        onOpenChange={setRenameOpen}
        selectedAccountName={renameAccountName}
      />
    </Page>
  );
}

function RenameAccountDialog({ open, onOpenChange, selectedAccountName = "" }) {
  const queryClient = useQueryClient();
  const [oldName, setOldName] = useState("");
  const [newName, setNewName] = useState("");
  const [step, setStep] = useState("input");
  const [error, setError] = useState(null);

  const { data: accountsData } = useQuery({
    queryKey: queryKeys.accounts(),
    queryFn: () => ledgerClient.listAccounts({}),
    enabled: open,
  });

  useEffect(() => {
    if (open) {
      const selected = selectedAccountName.trim();
      setOldName(selected);
      setNewName(selected);
      setStep("input");
      setError(null);
    }
  }, [open, selectedAccountName]);

  const renameMutation = useMutation({
    mutationFn: (vars) => ledgerClient.renameAccount(vars),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.accountDeclarations() });
      queryClient.invalidateQueries({ queryKey: queryKeys.accounts() });
      queryClient.invalidateQueries({ queryKey: ["balances"] });
      queryClient.invalidateQueries({ queryKey: ["transactions"] });
      queryClient.invalidateQueries({ queryKey: ["accountRegister"] });
      queryClient.invalidateQueries({ queryKey: ["netWorthTimeseries"] });
      onOpenChange(false);
    },
    onError: (err) => setError(err),
  });

  function handleOldNameChange(value) {
    const previousOldName = oldName.trim();
    setOldName(value);
    setNewName((current) => (
      current.trim() === previousOldName ? value : current
    ));
  }

  function handleContinue(e) {
    e.preventDefault();
    setError(null);
    const trimmedOld = oldName.trim();
    const trimmedNew = newName.trim();
    if (!trimmedOld) {
      setError(new Error("Pick the account to rename."));
      return;
    }
    if (!trimmedNew) {
      setError(new Error("New account name is required."));
      return;
    }
    if (trimmedNew === trimmedOld) {
      setError(new Error("New account name must differ from the current name."));
      return;
    }
    setStep("confirm");
  }

  function handleConfirm() {
    setError(null);
    renameMutation.mutate({ oldName: oldName.trim(), newName: newName.trim() });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="sm" showCloseButton>
        <DialogHeader>
          <DialogTitle>Rename Account</DialogTitle>
          <DialogDescription>
            Renames an account in the declaration file and across every posting in your
            journal history.
          </DialogDescription>
        </DialogHeader>
        {error && <ErrorBanner error={error} />}
        {step === "input" ? (
          <Form onSubmit={handleContinue}>
            <FormField label="Account to rename">
              <AccountInput
                value={oldName}
                onChange={handleOldNameChange}
                accounts={accountsData?.accounts || []}
                placeholder="Select account"
              />
            </FormField>
            <FormField label="New name" htmlFor="rename-new">
              <Input
                id="rename-new"
                type="text"
                placeholder="assets:bank:checking"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                required
                className="font-mono"
              />
            </FormField>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit">Continue</Button>
            </DialogFooter>
          </Form>
        ) : (
          <div className="flex flex-col gap-4">
            <div className="rounded-md border bg-muted/40 p-3 text-xs">
              <div className="text-muted-foreground">Rename</div>
              <div className="font-mono">{oldName.trim()}</div>
              <div className="text-muted-foreground mt-2">to</div>
              <div className="font-mono">{newName.trim()}</div>
            </div>
            <p className="text-xs text-muted-foreground">
              This updates the account declaration and rewrites every posting that references{" "}
              <span className="font-mono">{oldName.trim()}</span> or any sub-account beneath it.
              The change is committed to the snapshot history and cannot be undone except by
              restoring a snapshot.
            </p>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={renameMutation.isPending}
                onClick={() => setStep("input")}
              >
                Back
              </Button>
              <Button
                type="button"
                isLoading={renameMutation.isPending}
                loadingText="Renaming..."
                onClick={handleConfirm}
              >
                Confirm Rename
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
