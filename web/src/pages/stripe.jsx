import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { loadStripe } from "@stripe/stripe-js";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { CheckCircle, Circle, RefreshCw, Trash2, Link2 } from "lucide-react";

// Publishable Stripe.js key. The Financial Connections collection flow
// requires this even when using a custom backend — it's safe to expose.
// Defaults to Stripe's universal test publishable key; users can override
// via the input field on first launch (stored in localStorage).
const DEFAULT_PUBLISHABLE_KEY_STORAGE = "float-stripe-publishable-key";

function getPublishableKey() {
  if (typeof window === "undefined") return "";
  return window.localStorage.getItem(DEFAULT_PUBLISHABLE_KEY_STORAGE) ?? "";
}

export function StripePage() {
  const queryClient = useQueryClient();

  const { data: status, isLoading: statusLoading, error: statusError } = useQuery({
    queryKey: queryKeys.stripeStatus(),
    queryFn: () => ledgerClient.getStripeStatus({}),
  });

  const { data: connsData, error: connsError } = useQuery({
    queryKey: queryKeys.stripeConnections(),
    queryFn: () => ledgerClient.listStripeConnections({}),
  });

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h2 className="text-2xl font-bold">Stripe Financial Connections</h2>
        <p className="text-muted-foreground text-sm mt-1">
          Pull transactions from your bank accounts via Stripe and import them
          straight into the journal.
        </p>
      </div>

      {statusError && <ErrorBanner error={statusError} />}
      {statusLoading && <Loading />}

      {status && (
        <ApiKeyCard
          status={status}
          onChange={() => queryClient.invalidateQueries({ queryKey: queryKeys.stripeStatus() })}
        />
      )}

      {connsError && <ErrorBanner error={connsError} />}

      {status?.apiKeyConfigured && (
        <ConnectionsCard
          connections={connsData?.connections ?? []}
          onChange={() =>
            queryClient.invalidateQueries({ queryKey: queryKeys.stripeConnections() })
          }
        />
      )}
    </div>
  );
}

function ApiKeyCard({ status, onChange }) {
  const [apiKey, setApiKey] = useState("");
  const [pubKey, setPubKey] = useState(getPublishableKey());
  const [error, setError] = useState(null);
  const [saved, setSaved] = useState(false);

  const setKeyMutation = useMutation({
    mutationFn: (key) => ledgerClient.setStripeApiKey({ apiKey: key }),
    onSuccess: () => {
      setApiKey("");
      setSaved(true);
      setError(null);
      onChange();
      setTimeout(() => setSaved(false), 3000);
    },
    onError: setError,
  });

  function handleSave(e) {
    e.preventDefault();
    setError(null);
    setKeyMutation.mutate(apiKey);
  }

  function handleClear() {
    if (!confirm("Clear the Stripe secret key? Linked connections are kept.")) return;
    setError(null);
    setKeyMutation.mutate("");
  }

  function handleSavePub(e) {
    e.preventDefault();
    window.localStorage.setItem(DEFAULT_PUBLISHABLE_KEY_STORAGE, pubKey.trim());
    setSaved(true);
    setTimeout(() => setSaved(false), 3000);
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Stripe API keys</CardTitle>
        <CardDescription>
          The secret key (sk_…) is stored in <code>config.toml</code> on the
          server. The publishable key (pk_…) is kept in your browser's local
          storage and only used by Stripe.js to launch the account-linking
          flow.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {error && <ErrorBanner error={error} />}

        <div className="flex items-center gap-2 text-sm">
          {status.apiKeyConfigured ? (
            <>
              <CheckCircle className="size-4 text-success" />
              <span>Secret key configured</span>
              <Badge variant="secondary" className="font-mono">
                {status.apiKeyPreview}
              </Badge>
            </>
          ) : (
            <>
              <Circle className="size-4 text-muted-foreground" />
              <span className="text-muted-foreground">No secret key set</span>
            </>
          )}
        </div>

        <form onSubmit={handleSave} className="flex flex-col gap-1.5">
          <Label htmlFor="stripe-secret">
            {status.apiKeyConfigured ? "Replace secret key" : "Set secret key"}
          </Label>
          <div className="flex gap-2">
            <Input
              id="stripe-secret"
              type="password"
              placeholder="sk_live_… or sk_test_…"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              className="max-w-sm font-mono"
            />
            <Button type="submit" disabled={!apiKey || setKeyMutation.isPending}>
              {setKeyMutation.isPending ? "Saving…" : "Save"}
            </Button>
          </div>
        </form>

        {status.apiKeyConfigured && (
          <div>
            <Button
              variant="ghost"
              size="sm"
              className="text-destructive hover:text-destructive"
              disabled={setKeyMutation.isPending}
              onClick={handleClear}
            >
              Clear secret key
            </Button>
          </div>
        )}

        <form onSubmit={handleSavePub} className="flex flex-col gap-1.5 mt-2">
          <Label htmlFor="stripe-pub">Publishable key (browser only)</Label>
          <div className="flex gap-2">
            <Input
              id="stripe-pub"
              type="text"
              placeholder="pk_live_… or pk_test_…"
              value={pubKey}
              onChange={(e) => setPubKey(e.target.value)}
              className="max-w-sm font-mono"
            />
            <Button type="submit" variant="secondary">Save</Button>
          </div>
        </form>

        {saved && <p className="text-sm text-success">Saved.</p>}
      </CardContent>
    </Card>
  );
}

function ConnectionsCard({ connections, onChange }) {
  const [linkOpen, setLinkOpen] = useState(false);
  const [editConn, setEditConn] = useState(null);

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <div>
          <CardTitle>Linked accounts</CardTitle>
          <CardDescription>
            One row per Stripe Financial Connections account.
          </CardDescription>
        </div>
        <Button onClick={() => setLinkOpen(true)}>
          <Link2 className="size-4" /> Connect new account
        </Button>
      </CardHeader>
      <CardContent>
        {connections.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No accounts linked yet. Click "Connect new account" to start.
          </p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Account</TableHead>
                <TableHead>hledger account</TableHead>
                <TableHead>Last synced</TableHead>
                <TableHead className="text-right">Imported</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {connections.map((c) => (
                <ConnectionRow
                  key={c.id}
                  connection={c}
                  onEdit={() => setEditConn(c)}
                  onChange={onChange}
                />
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>

      {linkOpen && (
        <LinkAccountsDialog
          onClose={(linked) => {
            setLinkOpen(false);
            if (linked && linked.length > 0) {
              onChange();
              setEditConn(linked[0]);
            }
          }}
        />
      )}

      {editConn && (
        <EditConnectionDialog
          connection={editConn}
          onClose={() => {
            setEditConn(null);
            onChange();
          }}
        />
      )}
    </Card>
  );
}

function ConnectionRow({ connection, onEdit, onChange }) {
  const [error, setError] = useState(null);

  const syncMutation = useMutation({
    mutationFn: () => ledgerClient.syncStripeConnection({ id: connection.id }),
    onSuccess: () => {
      setError(null);
      onChange();
    },
    onError: setError,
  });

  const deleteMutation = useMutation({
    mutationFn: () => ledgerClient.deleteStripeConnection({ id: connection.id }),
    onSuccess: () => {
      setError(null);
      onChange();
    },
    onError: setError,
  });

  const mapped = Boolean(connection.hledgerAccount);
  const lastSynced = connection.lastSyncedAt || "—";

  return (
    <>
      <TableRow>
        <TableCell>
          <div className="flex flex-col">
            <span className="font-medium">{connection.displayName}</span>
            <span className="text-xs text-muted-foreground">
              {connection.institutionName} · ····{connection.last4} ·{" "}
              {connection.accountSubcategory || connection.accountCategory}
            </span>
          </div>
        </TableCell>
        <TableCell className="font-mono text-xs">
          {mapped ? (
            connection.hledgerAccount
          ) : (
            <Badge variant="outline">unmapped</Badge>
          )}
        </TableCell>
        <TableCell className="text-xs">{lastSynced}</TableCell>
        <TableCell className="text-right">{connection.importedCount}</TableCell>
        <TableCell className="text-right">
          <div className="flex justify-end gap-1">
            <Button
              size="sm"
              variant="secondary"
              disabled={!mapped || syncMutation.isPending}
              onClick={() => syncMutation.mutate()}
              title={mapped ? "Sync now" : "Map an hledger account first"}
            >
              <RefreshCw className={`size-3 ${syncMutation.isPending ? "animate-spin" : ""}`} /> Sync
            </Button>
            <Button size="sm" variant="ghost" onClick={onEdit}>
              Edit
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="text-destructive hover:text-destructive"
              disabled={deleteMutation.isPending}
              onClick={() => {
                if (
                  confirm(
                    `Delete ${connection.displayName}? Existing imported transactions remain in the journal.`
                  )
                ) {
                  deleteMutation.mutate();
                }
              }}
            >
              <Trash2 className="size-3" />
            </Button>
          </div>
        </TableCell>
      </TableRow>
      {error && (
        <TableRow>
          <TableCell colSpan={5}>
            <ErrorBanner error={error} />
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

function LinkAccountsDialog({ onClose }) {
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);

  async function handleLink() {
    setError(null);
    setBusy(true);
    try {
      const pubKey = getPublishableKey();
      if (!pubKey) {
        throw new Error(
          "Publishable key not set. Add it under 'Stripe API keys' above first."
        );
      }
      const stripe = await loadStripe(pubKey);
      if (!stripe) throw new Error("Failed to load Stripe.js");

      const { clientSecret } = await ledgerClient.createStripeSession({});
      const result = await stripe.collectFinancialConnectionsAccounts({
        clientSecret,
      });
      if (result.error) {
        throw new Error(result.error.message || "Stripe.js returned an error");
      }
      const accounts = result.financialConnectionsSession?.accounts ?? [];
      if (accounts.length === 0) {
        onClose([]);
        return;
      }
      const linked = await ledgerClient.linkStripeAccounts({
        stripeAccountIds: accounts.map((a) => a.id),
      });
      onClose(linked.connections ?? []);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose(null)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Connect a Stripe account</DialogTitle>
          <DialogDescription>
            Launches Stripe's Financial Connections flow in a popover. You'll
            log into your bank and choose which accounts to share.
          </DialogDescription>
        </DialogHeader>
        {error && <ErrorBanner error={error} />}
        <DialogFooter>
          <Button variant="ghost" onClick={() => onClose(null)} disabled={busy}>
            Cancel
          </Button>
          <Button onClick={handleLink} disabled={busy}>
            {busy ? "Connecting…" : "Open Stripe"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function EditConnectionDialog({ connection, onClose }) {
  const [displayName, setDisplayName] = useState(connection.displayName);
  const [hledgerAccount, setHledgerAccount] = useState(connection.hledgerAccount);
  const [inflow, setInflow] = useState(
    connection.defaultInflowAccount || "income:unknown"
  );
  const [outflow, setOutflow] = useState(
    connection.defaultOutflowAccount || "expenses:unknown"
  );
  const [error, setError] = useState(null);

  const updateMutation = useMutation({
    mutationFn: () =>
      ledgerClient.updateStripeConnection({
        id: connection.id,
        displayName,
        hledgerAccount,
        defaultInflowAccount: inflow,
        defaultOutflowAccount: outflow,
      }),
    onSuccess: () => onClose(),
    onError: setError,
  });

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit account mapping</DialogTitle>
          <DialogDescription>
            {connection.institutionName} · ····{connection.last4}
          </DialogDescription>
        </DialogHeader>
        {error && <ErrorBanner error={error} />}
        <form
          className="flex flex-col gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            updateMutation.mutate();
          }}
        >
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="conn-name">Display name</Label>
            <Input
              id="conn-name"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="conn-acct">hledger account</Label>
            <Input
              id="conn-acct"
              placeholder="assets:chase:checking or liabilities:amex"
              value={hledgerAccount}
              onChange={(e) => setHledgerAccount(e.target.value)}
              className="font-mono"
            />
            <p className="text-xs text-muted-foreground">
              The asset or liability account this Stripe account represents.
              Required before syncing.
            </p>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="conn-inflow">Default inflow account</Label>
            <Input
              id="conn-inflow"
              placeholder="income:unknown"
              value={inflow}
              onChange={(e) => setInflow(e.target.value)}
              className="font-mono"
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="conn-outflow">Default outflow account</Label>
            <Input
              id="conn-outflow"
              placeholder="expenses:unknown"
              value={outflow}
              onChange={(e) => setOutflow(e.target.value)}
              className="font-mono"
            />
            <p className="text-xs text-muted-foreground">
              Used when no float rule matches the transaction description.
            </p>
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              onClick={() => onClose()}
              disabled={updateMutation.isPending}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={updateMutation.isPending}>
              {updateMutation.isPending ? "Saving…" : "Save"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
