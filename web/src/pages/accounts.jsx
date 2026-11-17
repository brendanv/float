import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys, invalidateLedgerQueries } from "../query-keys.js";
import { BookOpen, ChevronRight, Pencil } from "lucide-react";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { AccountInput } from "../components/posting-fields.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { Page, Section } from "../components/page.jsx";
import { Amount } from "../components/amount.jsx";
import { AccountGlyph } from "../components/account.jsx";
import { TagChip } from "../components/tag-editor.jsx";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Form, FormField } from "@/components/ui/form";
import { accountType } from "@/lib/accounts.js";
import { cn, ROW_ACTION } from "@/lib/utils";
import {
  ResponsiveDialog,
  ResponsiveDialogContent,
  ResponsiveDialogDescription,
  ResponsiveDialogFooter,
  ResponsiveDialogHeader,
  ResponsiveDialogTitle,
} from "@/components/ui/responsive-dialog";

// buildTree merges account declarations with accounts that have postings but
// no declaration, so any account can be renamed or closed from this page.
function buildTree(declarations, usedAccountNames = []) {
  const byName = new Map(declarations.map((d) => [d.name, d]));
  const children = new Map();

  function ensureNode(name) {
    if (!byName.has(name)) {
      byName.set(name, { name, hasPostings: null, closed: false });
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

  const names = new Set([...declarations.map((d) => d.name), ...usedAccountNames]);
  for (const name of [...names].sort((a, b) => a.localeCompare(b))) {
    ensureNode(name);
  }

  for (const kids of children.values()) kids.sort();

  const roots = [...byName.keys()].filter((n) => !n.includes(":")).sort();
  return { byName, children, roots };
}

// closedState returns "closed" when the account itself carries the closed
// tag, "inherited" when an ancestor does (hledger inherits account tags), or
// null when the account is open.
function closedState(name, byName) {
  if (byName.get(name)?.closed) return "closed";
  const parts = name.split(":");
  for (let i = parts.length - 1; i > 0; i--) {
    if (byName.get(parts.slice(0, i).join(":"))?.closed) return "inherited";
  }
  return null;
}

// Raw (unvalued) inclusive balances for every account, as an hledger tree.
const BALANCE_TREE_PARAMS = { tree: true };

// Credit-normal accounts read as positive amounts owed / earned.
const NEGATE_TYPES = new Set(["liability", "income", "equity"]);

// balancesByName maps each account to its `hledger bal --tree` amounts.
// hledger elides a parent with a single child and no postings of its own
// into one "parent:child" row; the elided parents share that row's total.
function balancesByName(rows) {
  const map = new Map();
  for (const row of rows) {
    map.set(row.fullName, row.amounts);
    const shown = String(row.displayName || "").split(":").length;
    const parts = row.fullName.split(":");
    for (let i = 1; i < shown; i++) {
      const ancestor = parts.slice(0, parts.length - i).join(":");
      if (!map.has(ancestor)) map.set(ancestor, row.amounts);
    }
  }
  return map;
}

// visibleRows flattens the tree depth-first, skipping collapsed subtrees.
function visibleRows(roots, children, collapsed) {
  const out = [];
  function walk(name, depth) {
    out.push({ name, depth });
    if (collapsed.has(name)) return;
    for (const child of children.get(name) ?? []) walk(child, depth + 1);
  }
  for (const root of roots) walk(root, 0);
  return out;
}

function AccountRow({ name, depth, tree }) {
  const { byName, children, declaredNames, balances, collapsed, pendingName, onToggle, onDelete, onRename, onSetClosed } = tree;
  const hasKids = (children.get(name) ?? []).length > 0;
  const expanded = !collapsed.has(name);
  const label = name.split(":").at(-1);
  const closed = closedState(name, byName);
  const canDelete = declaredNames.has(name) && !byName.get(name)?.hasPostings;
  const type = accountType(name);
  const amounts = balances.get(name);
  const isRoot = depth === 0;

  return (
    <div
      data-slot="account-row"
      className={cn(
        "group/row grid min-h-[34px] grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-x-2 border-b border-border px-1 hover:bg-muted/50",
        isRoot && "font-semibold",
        isRoot && "[&:not(:first-child)]:mt-4",
      )}
    >
      <div className="flex min-w-0 items-center gap-1.5" style={{ paddingLeft: `${depth * 1.125}rem` }}>
        <button
          type="button"
          className="touch-target -ml-1 flex size-5 shrink-0 items-center justify-center rounded text-faint hover:text-foreground disabled:pointer-events-none"
          onClick={() => onToggle(name)}
          disabled={!hasKids}
          tabIndex={hasKids ? 0 : -1}
          aria-label={hasKids ? `${expanded ? "Collapse" : "Expand"} ${name}` : undefined}
          aria-hidden={hasKids ? undefined : true}
          aria-expanded={hasKids ? expanded : undefined}
        >
          {hasKids && (
            <ChevronRight className={cn("size-3.5 transition-transform", expanded && "rotate-90")} />
          )}
        </button>
        <AccountGlyph type={type} />
        <span
          className={cn("min-w-[5ch] truncate", closed && "text-faint")}
          title={closed === "inherited" ? `${name} (closed via parent account)` : name}
        >
          {label}
        </span>
        {closed === "closed" && <TagChip className="shrink-0 font-normal">closed</TagChip>}
        <Button
          variant="ghost"
          size="icon-xs"
          className={cn("text-muted-foreground", ROW_ACTION)}
          aria-label={`Rename ${name}`}
          title={`Rename ${name}`}
          onClick={() => onRename(name)}
        >
          <Pencil className="size-3" />
        </Button>
      </div>
      <div className="flex items-center gap-1 font-normal">
        {closed !== "inherited" && (
          <Button
            variant="ghost"
            size="xs"
            className={cn("text-muted-foreground", ROW_ACTION)}
            isLoading={pendingName === name}
            onClick={() => onSetClosed(name, !closed)}
          >
            {closed ? "Reopen" : "Close"}
          </Button>
        )}
        {canDelete && (
          <Button
            variant="destructive-ghost"
            size="xs"
            className={ROW_ACTION}
            isLoading={pendingName === name}
            onClick={() => onDelete(name)}
          >
            Delete
          </Button>
        )}
      </div>
      <div className={cn("min-w-[5.5rem] py-1 text-right", closed && "text-faint")}>
        {amounts && <Amount value={amounts} negate={NEGATE_TYPES.has(type)} />}
      </div>
    </div>
  );
}

export function AccountsPage() {
  const queryClient = useQueryClient();

  const { data, isLoading, error: fetchError } = useQuery({
    queryKey: queryKeys.accountDeclarations(),
    queryFn: () => ledgerClient.listAccountDeclarations({}),
  });

  const { data: accountsData } = useQuery({
    queryKey: queryKeys.accounts(),
    queryFn: () => ledgerClient.listAccounts({}),
  });

  const { data: balancesData } = useQuery({
    queryKey: queryKeys.balances(BALANCE_TREE_PARAMS),
    queryFn: () => ledgerClient.getBalances(BALANCE_TREE_PARAMS),
  });

  const [name, setName] = useState("");
  const [formError, setFormError] = useState(null);
  const [pendingName, setPendingName] = useState(null);
  const [renameOpen, setRenameOpen] = useState(false);
  const [renameAccountName, setRenameAccountName] = useState("");
  const [collapsed, setCollapsed] = useState(() => new Set());

  const addMutation = useMutation({
    mutationFn: (vars) => ledgerClient.declareAccount(vars),
    onSuccess: () => {
      setName("");
      setFormError(null);
      invalidateLedgerQueries(queryClient);
    },
    onError: (err) => setFormError(err),
  });

  const deleteMutation = useMutation({
    mutationFn: ({ name }) => ledgerClient.deleteAccountDeclaration({ name }),
    onSuccess: () => {
      setPendingName(null);
      invalidateLedgerQueries(queryClient);
    },
    onError: (err) => {
      setPendingName(null);
      setFormError(err);
    },
  });

  const closedMutation = useMutation({
    mutationFn: ({ name, closed }) => ledgerClient.setAccountClosed({ name, closed }),
    onSuccess: () => {
      setPendingName(null);
      invalidateLedgerQueries(queryClient);
    },
    onError: (err) => {
      setPendingName(null);
      setFormError(err);
    },
  });

  function handleSubmit(e) {
    e.preventDefault();
    setFormError(null);
    addMutation.mutate({ name: name.trim() });
  }

  function handleDelete(name) {
    setPendingName(name);
    setFormError(null);
    deleteMutation.mutate({ name });
  }

  function handleSetClosed(name, closed) {
    setPendingName(name);
    setFormError(null);
    closedMutation.mutate({ name, closed });
  }

  function handleRename(name) {
    setRenameAccountName(name);
    setFormError(null);
    setRenameOpen(true);
  }

  function handleToggle(name) {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }

  const declarations = data?.declarations ?? [];
  const declaredNames = new Set(declarations.map((d) => d.name));
  const usedAccountNames = (accountsData?.accounts ?? []).map((a) => a.fullName);
  const { byName, children, roots } = buildTree(declarations, usedAccountNames);
  const tree = {
    byName,
    children,
    declaredNames,
    balances: balancesByName(balancesData?.report?.rows ?? []),
    collapsed,
    pendingName,
    onToggle: handleToggle,
    onDelete: handleDelete,
    onRename: handleRename,
    onSetClosed: handleSetClosed,
  };
  const rows = visibleRows(roots, children, collapsed);

  return (
    <Page>
      <PageHeader title="Accounts" />

      <Section title="Declare account">
        <Form onSubmit={handleSubmit} className="gap-3">
          {formError && <ErrorBanner error={formError} />}
          <div className="flex flex-col gap-2 sm:flex-row sm:items-start">
            <FormField
              label="Account name"
              htmlFor="acct-name"
              description="Colon-separated hierarchy, e.g. assets:bank:checking"
              className="sm:w-96"
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
            <Button
              type="submit"
              className="self-start sm:mt-[1.5625rem]"
              isLoading={addMutation.isPending}
              loadingText="Declaring..."
            >
              Declare account
            </Button>
          </div>
        </Form>
      </Section>

      <Section
        title="Account tree"
        action={<span className="text-label uppercase text-faint">Balance</span>}
      >
        {isLoading && <Loading />}
        {fetchError && <ErrorBanner error={fetchError} />}
        {data && roots.length === 0 && (
          <EmptyState
            icon={BookOpen}
            title="No accounts yet"
            description="Declare accounts to get type-aware classification (assets, liabilities, income, expenses, equity)."
          />
        )}
        {data && roots.length > 0 && (
          <div data-slot="account-tree" className="-mt-3 flex flex-col text-body">
            {rows.map(({ name, depth }) => (
              <AccountRow key={name} name={name} depth={depth} tree={tree} />
            ))}
          </div>
        )}
        {data && roots.length > 0 && (
          <p className="text-[12.5px] text-faint">
            Closed accounts are hidden from account pickers and Overview totals; hledger reports are unchanged.
          </p>
        )}
      </Section>

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
      invalidateLedgerQueries(queryClient);
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
    <ResponsiveDialog open={open} onOpenChange={onOpenChange}>
      <ResponsiveDialogContent size="sm" showCloseButton>
        <ResponsiveDialogHeader>
          <ResponsiveDialogTitle>Rename account</ResponsiveDialogTitle>
          <ResponsiveDialogDescription>
            Renames an account in the declaration file and across every posting in your
            journal history.
          </ResponsiveDialogDescription>
        </ResponsiveDialogHeader>
        {error && <ErrorBanner error={error} />}
        {step === "input" ? (
          <Form onSubmit={handleContinue}>
            <FormField label="Account to rename">
              <AccountInput
                value={oldName}
                onChange={handleOldNameChange}
                accounts={accountsData?.accounts || []}
                includeClosed
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
            <ResponsiveDialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit">Continue</Button>
            </ResponsiveDialogFooter>
          </Form>
        ) : (
          <div className="flex flex-col gap-4">
            <div className="rounded-lg bg-muted px-3 py-2.5 text-control">
              <div className="text-muted-foreground">Rename</div>
              <div className="font-mono text-code">{oldName.trim()}</div>
              <div className="mt-2 text-muted-foreground">to</div>
              <div className="font-mono text-code">{newName.trim()}</div>
            </div>
            <p className="text-control text-muted-foreground">
              This updates the account declaration and rewrites every posting that references{" "}
              <span className="font-mono">{oldName.trim()}</span> or any sub-account beneath it.
              The change is committed to the snapshot history and cannot be undone except by
              restoring a snapshot.
            </p>
            <ResponsiveDialogFooter>
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
            </ResponsiveDialogFooter>
          </div>
        )}
      </ResponsiveDialogContent>
    </ResponsiveDialog>
  );
}
