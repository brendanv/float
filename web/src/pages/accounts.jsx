import { useState } from "react";
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

function buildTree(declarations) {
  const byName = new Map(declarations.map((d) => [d.name, d]));
  const children = new Map();

  function ensureNode(name) {
    if (!byName.has(name)) {
      byName.set(name, { name, aid: null, hasPostings: null });
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

function AccountTreeNode({ name, byName, children, depth, onDelete, deletingAid }) {
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
        {decl?.aid && !decl.hasPostings && (
          <Button
            variant="ghost"
            size="xs"
            className="text-destructive opacity-0 group-hover:opacity-100 transition-opacity mr-1"
            disabled={deletingAid === decl.aid}
            onClick={() => onDelete(decl.aid)}
          >
            {deletingAid === decl.aid ? (
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
              deletingAid={deletingAid}
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
  const [deletingAid, setDeletingAid] = useState(null);

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
    mutationFn: ({ aid }) => ledgerClient.deleteAccountDeclaration({ aid }),
    onSuccess: () => {
      setDeletingAid(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.accountDeclarations() });
    },
    onError: (err) => {
      setDeletingAid(null);
      setFormError(err);
    },
  });

  function handleSubmit(e) {
    e.preventDefault();
    setFormError(null);
    addMutation.mutate({ name: name.trim() });
  }

  function handleDelete(aid) {
    setDeletingAid(aid);
    setFormError(null);
    deleteMutation.mutate({ aid });
  }

  const declarations = data?.declarations ?? [];
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
                      <div className="px-3 py-2 bg-muted/40 border-b flex items-center justify-between">
                        <span className="font-semibold text-sm capitalize">{root}</span>
                        {rootDecl?.aid && !rootDecl.hasPostings && (
                          <Button
                            variant="ghost"
                            size="xs"
                            className="text-destructive text-xs h-6"
                            disabled={deletingAid === rootDecl.aid}
                            onClick={() => handleDelete(rootDecl.aid)}
                          >
                            {deletingAid === rootDecl.aid ? (
                              <Loader2 className="size-3 animate-spin" />
                            ) : (
                              "Delete"
                            )}
                          </Button>
                        )}
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
                              deletingAid={deletingAid}
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
    </div>
  );
}
