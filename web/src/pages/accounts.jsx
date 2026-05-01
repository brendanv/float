import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Loader2, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
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
        <span className="font-mono text-sm flex-1">{label}</span>
        <Button
          variant="ghost"
          size="xs"
          className="opacity-0 group-hover:opacity-100 transition-opacity"
          onClick={() => onRename(name)}
        >
          Rename
        </Button>
        {declaredNames.has(name) && !decl?.hasPostings && (
          <Button
            variant="ghost"
            size="xs"
            className="text-destructive opacity-0 group-hover:opacity-100 transition-opacity mr-1"
            disabled={deletingName === name}
            onClick={() => onDelete(name)}
          >
            {deletingName === name ? (
              <Loader2 className="size-3 animate-spin" />
            ) : (
              "Delete"
            )}
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

export function AccountsPage() {
  const queryClient = useQueryClient();

  const { data, isLoading, error: fetchError } = useQuery({
    queryKey: queryKeys.accountDeclarations(),
    queryFn: () => ledgerClient.listAccountDeclarations({}),
  });

  const [name, setName] = useState("");
  const [formError, setFormError] = useState(null);
  const [deletingName, setDeletingName] = useState(null);
  const [renamingName, setRenamingName] = useState(null);

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
    setFormError(null);
    setRenamingName(name);
  }

  const declarations = data?.declarations ?? [];
  const declaredNames = new Set(declarations.map((d) => d.name));
  const { byName, children, roots } = buildTree(declarations);

  return (
    <div className="flex flex-col gap-6">
      <h2 className="text-2xl font-bold">Account Declarations</h2>

      <Card>
        <CardHeader>
          <CardTitle>Declare Account</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="flex flex-wrap items-end gap-3">
            <div className="w-full flex flex-col gap-1.5 sm:w-72">
              <Label htmlFor="acct-name">Account Name</Label>
              <Input
                id="acct-name"
                type="text"
                placeholder="assets:checking"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>
            <Button type="submit" disabled={addMutation.isPending}>
              {addMutation.isPending && (
                <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />
              )}
              {addMutation.isPending ? "Declaring…" : "Declare"}
            </Button>
          </form>
          {formError && <div className="mt-3"><ErrorBanner error={formError} /></div>}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Declared Accounts</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading && <Loading />}
          {fetchError && <ErrorBanner error={fetchError} />}
          {data && (
            declarations.length > 0 ? (
              <div className="flex flex-col gap-3">
                {roots.map((root) => {
                  const rootDecl = byName.get(root);
                  const kids = children.get(root) ?? [];
                  return (
                    <div key={root} className="rounded-md border overflow-hidden">
                      <div className="px-3 py-2 bg-muted/40 border-b flex items-center justify-between gap-2">
                        <span className="font-semibold text-sm capitalize">{root}</span>
                        <div className="flex items-center gap-1">
                          <Button
                            variant="ghost"
                            size="xs"
                            className="text-xs h-6"
                            onClick={() => handleRename(root)}
                          >
                            Rename
                          </Button>
                          {declaredNames.has(root) && !rootDecl?.hasPostings && (
                            <Button
                              variant="ghost"
                              size="xs"
                              className="text-destructive text-xs h-6"
                              disabled={deletingName === root}
                              onClick={() => handleDelete(root)}
                            >
                              {deletingName === root ? (
                                <Loader2 className="size-3 animate-spin" />
                              ) : (
                                "Delete"
                              )}
                            </Button>
                          )}
                        </div>
                      </div>
                      {kids.length > 0 && (
                        <div>
                          {kids.map((child) => (
                            <AccountTreeNode
                              key={child}
                              name={child}
                              byName={byName}
                              children={children}
                              depth={0}
                              onDelete={handleDelete}
                              onRename={handleRename}
                              deletingName={deletingName}
                              declaredNames={declaredNames}
                            />
                          ))}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">No account declarations yet.</p>
            )
          )}
        </CardContent>
      </Card>

      <RenameAccountDialog
        oldName={renamingName}
        open={renamingName !== null}
        onOpenChange={(open) => { if (!open) setRenamingName(null); }}
      />
    </div>
  );
}

function RenameAccountDialog({ oldName, open, onOpenChange }) {
  const queryClient = useQueryClient();
  const [newName, setNewName] = useState("");
  const [step, setStep] = useState("input");
  const [error, setError] = useState(null);

  useEffect(() => {
    if (open) {
      setNewName(oldName ?? "");
      setStep("input");
      setError(null);
    }
  }, [open, oldName]);

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

  function handleContinue(e) {
    e.preventDefault();
    setError(null);
    const trimmed = newName.trim();
    if (!trimmed) {
      setError(new Error("New account name is required."));
      return;
    }
    if (trimmed === oldName) {
      setError(new Error("New account name must differ from the current name."));
      return;
    }
    setStep("confirm");
  }

  function handleConfirm() {
    setError(null);
    renameMutation.mutate({ oldName, newName: newName.trim() });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md" showCloseButton>
        <DialogHeader>
          <DialogTitle>Rename Account</DialogTitle>
          <DialogDescription>
            Renames this account in the declaration file and across every posting in your
            journal history.
          </DialogDescription>
        </DialogHeader>
        {error && <ErrorBanner error={error} />}
        {step === "input" ? (
          <form onSubmit={handleContinue} className="flex flex-col gap-4">
            <div className="flex flex-col gap-1.5">
              <Label>Current name</Label>
              <Input value={oldName ?? ""} readOnly className="font-mono" />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="rename-new">New name</Label>
              <Input
                id="rename-new"
                type="text"
                placeholder="assets:bank:checking"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                autoFocus
                required
                className="font-mono"
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit">Continue</Button>
            </DialogFooter>
          </form>
        ) : (
          <div className="flex flex-col gap-4">
            <div className="rounded-md border bg-muted/40 p-3 text-xs">
              <div className="text-muted-foreground">Rename</div>
              <div className="font-mono">{oldName}</div>
              <div className="text-muted-foreground mt-2">to</div>
              <div className="font-mono">{newName.trim()}</div>
            </div>
            <p className="text-xs text-muted-foreground">
              This updates the account declaration and rewrites every posting that references{" "}
              <span className="font-mono">{oldName}</span> or any sub-account beneath it. The
              change is committed to the snapshot history and cannot be undone except by
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
                disabled={renameMutation.isPending}
                onClick={handleConfirm}
              >
                {renameMutation.isPending && (
                  <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />
                )}
                {renameMutation.isPending ? "Renaming…" : "Confirm Rename"}
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
