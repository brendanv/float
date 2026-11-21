import { useState } from "react";
import { useQuery, useQueryClient, useMutation } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { CalendarClock, Link2, Link2Off, RefreshCw } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys, invalidateLedgerQueries } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { Page, Section } from "../components/page.jsx";
import { Account } from "../components/account.jsx";
import { Timestamp } from "../components/timestamp.jsx";
import { LinkMappingDialog } from "../components/connections/link-mapping-dialog.jsx";
import { ImportFromDialog } from "../components/connections/import-from-dialog.jsx";
import {
  FetchResults,
  refreshLine,
  useAccountFetch,
  useFetchAll,
} from "../components/connections/fetch.jsx";
import { Button } from "@/components/ui/button";
import { Notice } from "@/components/ui/alert";
import { cn, ROW_ACTION } from "@/lib/utils";

const DOT_CLASS = {
  accent: "bg-primary",
  warning: "bg-warning",
  below: "bg-below",
};

// StatusDot is the 8px status dot of a linked account. Healthy accounts get
// none (attention is earned).
function StatusDot({ tone, label }) {
  return (
    <span className="inline-flex size-5 shrink-0 items-center justify-center">
      {tone && (
        <span
          role="img"
          aria-label={label}
          title={label}
          className={cn("size-2 rounded-full", DOT_CLASS[tone])}
        />
      )}
    </span>
  );
}

const LINE_TONE = { ink: "text-muted-foreground", warning: "text-warning", below: "text-below" };

// AccountName is the row's leading cell: display name (500) with the
// institution in ink-3.
function AccountName({ account }) {
  return (
    <span className="flex min-w-0 items-baseline gap-2">
      <span className="truncate font-medium">{account.displayName || account.stripeAccountId}</span>
      {account.institutionName && (
        <span className="min-w-0 shrink-[4] truncate text-control text-faint">{account.institutionName}</span>
      )}
    </span>
  );
}

// Phone: a stack of name, then account and last import on one line, then
// actions, all indented under the name past the status dot.
// From md: one dense line on a shared grid so the columns line up.
const ROW =
  "flex flex-col gap-1.5 py-3 md:grid md:min-h-[38px] md:grid-cols-[minmax(0,1fr)_minmax(0,12rem)_minmax(0,9.5rem)_auto] md:items-center md:gap-x-4 md:gap-y-1 md:py-1";

// DETAIL is the phone's second line (account + last import); from md its
// two cells join the row's grid.
const DETAIL = "flex min-w-0 items-baseline gap-3 pl-[26px] md:contents";

// LinkedAccountRow is one configured account: status dot, name +
// institution, hledger account, last import (relative), and its fetch
// actions. Fetched transactions open under the row. The import start date
// button is accent (and always shown) when a cutoff is set, hover-revealed
// otherwise.
function LinkedAccountRow({ account, refreshAll, onImported, onUnlink, unlinking }) {
  const queryClient = useQueryClient();
  const fetch = useAccountFetch(account, onImported);
  const [editingImportFrom, setEditingImportFrom] = useState(false);
  const line = refreshAll ? refreshLine(refreshAll) : null;
  const dot = line?.tone === "below"
    ? { tone: "below", label: "Refresh failed" }
    : line?.tone === "warning"
      ? { tone: "warning", label: "Refresh throttled" }
      : !account.lastFetchedAt
        ? { tone: "accent", label: "Never imported" }
        : {};

  return (
    <li data-slot="linked-account" className="group/row border-b border-border last:border-0">
      <div className={ROW}>
        <span className="flex min-w-0 items-center gap-1.5">
          <StatusDot {...dot} />
          <AccountName account={account} />
        </span>
        <span className={DETAIL}>
          <span className="flex min-w-0 flex-1">
            <Account name={account.hledgerAccount} variant="path" />
          </span>
          <span className="max-w-[60%] shrink-0 text-right text-control md:max-w-none md:text-left">
            {line ? (
              <span className={cn(LINE_TONE[line.tone])}>{line.text}</span>
            ) : account.lastFetchedAt ? (
              <span className="whitespace-nowrap text-muted-foreground">
                <span className="max-md:hidden">Imported </span>
                <Timestamp value={account.lastFetchedAt} />
              </span>
            ) : (
              <span className="text-faint">Never imported</span>
            )}
          </span>
        </span>
        <span className="flex flex-wrap items-center gap-1 pl-[26px] md:ml-auto md:shrink-0 md:pl-0">
          <Button
            variant="ghost"
            size="sm"
            onClick={fetch.handleRefresh}
            disabled={fetch.fetching}
            isLoading={fetch.refreshing}
            loadingText={
              fetch.refreshStatus
                ? `Refreshing… (${fetch.refreshStatus.elapsedSeconds}s, attempt ${fetch.refreshStatus.attempt})`
                : "Refreshing…"
            }
          >
            <RefreshCw data-icon="inline-start" />
            Refresh<span className="max-md:hidden"> &amp; Fetch</span>
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="max-md:order-first"
            onClick={fetch.handleFetch}
            disabled={fetch.refreshing}
            isLoading={fetch.fetching}
            loadingText="Fetching…"
          >
            Fetch<span className="max-md:hidden"> Transactions</span>
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            className={cn(account.importFrom ? "text-primary" : ROW_ACTION)}
            aria-label={account.importFrom ? `Imports from ${account.importFrom}` : "Set import start date"}
            title={account.importFrom ? `Imports from ${account.importFrom}` : "Set import start date"}
            onClick={() => setEditingImportFrom(true)}
          >
            <CalendarClock />
          </Button>
          <Button
            variant="destructive-ghost"
            size="sm"
            className={cn("max-md:ml-auto", !unlinking && ROW_ACTION)}
            isLoading={unlinking}
            loadingText="Unlink"
            onClick={() => onUnlink(account.stripeAccountId)}
          >
            <Link2Off data-icon="inline-start" />
            Unlink
          </Button>
        </span>
      </div>
      <FetchResults fetch={fetch} className="pb-4 md:pl-[26px]" />
      {editingImportFrom && (
        <ImportFromDialog
          open
          account={account}
          onClose={() => setEditingImportFrom(false)}
          onSaved={async () => {
            setEditingImportFrom(false);
            await queryClient.invalidateQueries({ queryKey: queryKeys.stripeLinkedAccounts() });
            // Re-run an open fetch so its list honors the new cutoff.
            if (fetch.candidates) await fetch.handleFetch();
          }}
        />
      )}
    </li>
  );
}

// UnmappedAccountRow is a linked Stripe account with no hledger account yet.
function UnmappedAccountRow({ account, onConfigure, onDisconnect, disabled }) {
  return (
    <li data-slot="linked-account" data-unmapped="" className={cn(ROW, "border-b border-border last:border-0")}>
      <span className="flex min-w-0 items-center gap-1.5">
        <StatusDot tone="warning" label="Needs an hledger account" />
        <AccountName account={account} />
      </span>
      <span className={DETAIL}>
        <span className="min-w-0 flex-1 text-control text-warning">Needs an hledger account</span>
        <span className="min-w-0 truncate font-mono text-[11.5px] text-faint">{account.stripeAccountId}</span>
      </span>
      <span className="flex items-center gap-1 pl-[26px] md:ml-auto md:shrink-0 md:pl-0">
        <Button variant="outline" size="sm" onClick={onConfigure}>
          Configure
        </Button>
        <Button variant="destructive-ghost" size="sm" onClick={onDisconnect} disabled={disabled}>
          Disconnect
        </Button>
      </span>
    </li>
  );
}

export function ConnectionsPage() {
  const queryClient = useQueryClient();
  const [linking, setLinking] = useState(false);
  const [linkError, setLinkError] = useState(null);
  const [pendingSession, setPendingSession] = useState(null);
  const [pendingConfigureAccount, setPendingConfigureAccount] = useState(null);

  const {
    data: configData,
    isLoading: configLoading,
    error: configError,
  } = useQuery({
    queryKey: queryKeys.stripeConfig(),
    queryFn: () => ledgerClient.getStripeConfig({}),
  });

  const {
    data: linkedData,
    isLoading: linkedLoading,
    error: linkedError,
  } = useQuery({
    queryKey: queryKeys.stripeLinkedAccounts(),
    queryFn: () => ledgerClient.listStripeLinkedAccounts({}),
    enabled: !!configData?.enabled,
  });

  const { data: declarationsData } = useQuery({
    queryKey: queryKeys.accountDeclarations(),
    queryFn: () => ledgerClient.listAccountDeclarations({}),
    enabled: !!configData?.enabled,
  });

  const accounts = linkedData?.accounts ?? [];
  const configuredAccounts = accounts.filter((a) => a.hledgerAccount);
  const unconfiguredAccounts = accounts.filter((a) => !a.hledgerAccount);
  const onImported = () => invalidateLedgerQueries(queryClient);
  const fetchAll = useFetchAll(configuredAccounts, onImported);

  async function handleLinkAccount() {
    setLinkError(null);
    setLinking(true);
    try {
      const sessionRes = await ledgerClient.createStripeLinkSession({});
      const { loadStripe } = await import("@stripe/stripe-js");
      const stripe = await loadStripe(configData.publishableKey);
      if (!stripe) throw new Error("Failed to load Stripe.js");
      const result = await stripe.collectFinancialConnectionsAccounts({
        clientSecret: sessionRes.clientSecret,
      });
      if (result.error) {
        setLinkError(new Error(result.error.message));
        return;
      }
      const session = result.financialConnectionsSession;
      if (!session || !session.accounts || session.accounts.length === 0) {
        setLinkError(new Error("No accounts were selected."));
        return;
      }
      setPendingSession({ id: session.id, accounts: session.accounts });
    } catch (err) {
      setLinkError(err);
    } finally {
      setLinking(false);
    }
  }

  async function handleCompleteLinking(mappings) {
    const linked = pendingSession.accounts.map((a) => ({
      stripeAccountId: a.id,
      hledgerAccount: mappings[a.id].hledgerAccount,
      displayName:
        mappings[a.id].displayName ||
        a.display_name ||
        a.displayName ||
        a.id,
      importFrom: mappings[a.id].importFrom || "",
    }));
    await ledgerClient.completeStripeLinking({ accounts: linked });
    setPendingSession(null);
    queryClient.invalidateQueries({ queryKey: queryKeys.stripeLinkedAccounts() });
    queryClient.invalidateQueries({ queryKey: queryKeys.stripeConfig() });
  }

  const unlinkMutation = useMutation({
    mutationFn: (stripeAccountId) =>
      ledgerClient.unlinkStripeAccount({ stripeAccountId }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.stripeLinkedAccounts() });
      queryClient.invalidateQueries({ queryKey: queryKeys.stripeConfig() });
    },
  });

  function handleUnlink(stripeAccountId) {
    if (!confirm("Unlink this account? Existing imported transactions will not be removed.")) return;
    unlinkMutation.mutate(stripeAccountId);
  }

  async function handleConfigureComplete(mappings) {
    const a = pendingConfigureAccount;
    await ledgerClient.completeStripeLinking({
      accounts: [{
        stripeAccountId: a.id,
        hledgerAccount: mappings[a.id].hledgerAccount,
        displayName: mappings[a.id].displayName || a.display_name || a.id,
        importFrom: mappings[a.id].importFrom || "",
      }],
    });
    setPendingConfigureAccount(null);
    queryClient.invalidateQueries({ queryKey: queryKeys.stripeLinkedAccounts() });
    queryClient.invalidateQueries({ queryKey: queryKeys.stripeConfig() });
  }

  if (configLoading) return <Loading />;
  if (configError) return <ErrorBanner error={configError} />;

  if (!configData?.enabled) {
    return (
      <Page>
        <PageHeader title="Connections" />
        <Notice variant="info" action={<Link to="/settings">Settings</Link>}>
          Bank linking is off. Set{" "}
          <code className="font-mono text-code">STRIPE_SECRET_KEY</code> and{" "}
          <code className="font-mono text-code">STRIPE_PUBLISHABLE_KEY</code> for floatd (test keys work for
          development), then restart it.
        </Notice>
      </Page>
    );
  }

  const fetchAllAccounts = configuredAccounts.length;

  return (
    <Page>
      <PageHeader title="Connections">
        <Button
          onClick={handleLinkAccount}
          isLoading={linking}
          loadingText="Connecting…"
        >
          <Link2 data-icon="inline-start" />
          Link Account
        </Button>
      </PageHeader>

      {linkError && <ErrorBanner error={linkError} />}
      {unlinkMutation.error && <ErrorBanner error={unlinkMutation.error} />}
      {linkedError && <ErrorBanner error={linkedError} />}

      <Section
        title="Linked accounts"
        description={accounts.length > 0 ? accounts.length : null}
        action={fetchAllAccounts > 0 && (
          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={fetchAll.handleRefreshAll}
              disabled={fetchAll.fetching}
              isLoading={fetchAll.refreshing}
              loadingText="Refreshing…"
            >
              <RefreshCw data-icon="inline-start" />
              Refresh<span className="max-sm:hidden"> &amp; Fetch</span> All
            </Button>
            <Button
              size="sm"
              onClick={fetchAll.handleFetchAll}
              disabled={fetchAll.refreshing}
              isLoading={fetchAll.fetching}
              loadingText="Fetching…"
            >
              Fetch All
            </Button>
          </div>
        )}
        className="gap-0"
      >
        {linkedLoading && <Loading />}
        {!linkedLoading && accounts.length === 0 && (
          <EmptyState
            icon={Link2}
            title="No linked accounts"
            description="Link a bank account through Stripe Financial Connections to fetch its transactions."
          />
        )}
        {accounts.length > 0 && (
          <ul data-slot="linked-accounts">
            {configuredAccounts.map((account) => (
              <LinkedAccountRow
                key={account.stripeAccountId}
                account={account}
                refreshAll={fetchAll.refreshAccountStatus.get(account.stripeAccountId)}
                onImported={onImported}
                onUnlink={handleUnlink}
                unlinking={unlinkMutation.isPending && unlinkMutation.variables === account.stripeAccountId}
              />
            ))}
            {unconfiguredAccounts.map((account) => (
              <UnmappedAccountRow
                key={account.stripeAccountId}
                account={account}
                onConfigure={() => setPendingConfigureAccount({ id: account.stripeAccountId, display_name: account.displayName })}
                onDisconnect={() => handleUnlink(account.stripeAccountId)}
                disabled={unlinkMutation.isPending}
              />
            ))}
          </ul>
        )}
      </Section>

      {(fetchAll.refreshError || fetchAll.fetchError || fetchAll.importResult || fetchAll.importError || fetchAll.candidates) && (
        <Section
          title="All accounts"
          description={`Fetched from ${fetchAllAccounts} linked account${fetchAllAccounts !== 1 ? "s" : ""}`}
        >
          <FetchResults
            fetch={fetchAll}
            showAccount
            resultText={`across ${fetchAllAccounts} account${fetchAllAccounts !== 1 ? "s" : ""}`}
          />
        </Section>
      )}

      {pendingSession && (
        <LinkMappingDialog
          key={pendingSession.id}
          open={true}
          fcAccounts={pendingSession.accounts}
          accountDeclarations={declarationsData?.declarations ?? []}
          onComplete={handleCompleteLinking}
          onClose={() => setPendingSession(null)}
        />
      )}

      {pendingConfigureAccount && (
        <LinkMappingDialog
          open={true}
          fcAccounts={[pendingConfigureAccount]}
          accountDeclarations={declarationsData?.declarations ?? []}
          onComplete={handleConfigureComplete}
          onClose={() => setPendingConfigureAccount(null)}
        />
      )}
    </Page>
  );
}
