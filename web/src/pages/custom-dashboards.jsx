import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { Page, PageCard, DashboardGrid } from "../components/page.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Form, FormField } from "@/components/ui/form";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { CheckCircle, Circle, ExternalLink } from "lucide-react";

export function CustomDashboardsPage() {
  const queryClient = useQueryClient();

  const { data, isLoading, error } = useQuery({
    queryKey: queryKeys.metabaseConfig(),
    queryFn: () => ledgerClient.getMetabaseConfig({}),
  });

  // --- Connection form state (initialized from server once loaded) ---
  const [form, setForm] = useState(null);
  const [apiKey, setApiKey] = useState("");
  const [saveError, setSaveError] = useState(null);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    if (data && form === null) {
      setForm({
        enabled: data.enabled,
        url: data.url,
        apiUrl: data.apiUrl,
        dbPath: data.dbPath,
        dbName: data.dbName,
      });
    }
  }, [data, form]);

  const saveMutation = useMutation({
    mutationFn: (payload) => ledgerClient.setMetabaseConfig(payload),
    onSuccess: () => {
      setApiKey("");
      setSaved(true);
      setSaveError(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.metabaseConfig() });
    },
    onError: (err) => {
      setSaved(false);
      setSaveError(err);
    },
  });

  function handleSave(e) {
    e?.preventDefault?.();
    saveMutation.mutate({
      enabled: form.enabled,
      url: form.url.trim(),
      apiUrl: form.apiUrl.trim(),
      dbPath: form.dbPath.trim(),
      dbName: form.dbName.trim(),
      apiKey: apiKey,
      clearApiKey: false,
    });
  }

  // --- Prepare / open ---
  const [prepareError, setPrepareError] = useState(null);
  const [lastResult, setLastResult] = useState(null);

  const prepareMutation = useMutation({
    mutationFn: () => ledgerClient.prepareDashboards({}),
    onSuccess: (res) => {
      setPrepareError(null);
      setLastResult(res);
      if (res.openUrl) {
        window.open(res.openUrl, "_blank", "noopener,noreferrer");
      }
    },
    onError: (err) => setPrepareError(err),
  });

  function update(field, value) {
    setForm((f) => ({ ...f, [field]: value }));
  }

  return (
    <Page>
      <PageHeader
        title="Custom Dashboards"
        description="Generate a fresh SQLite snapshot of your ledger and explore it in Metabase, where you can build your own charts, queries, and dashboards."
      />

      <DashboardGrid>
        {/* Open dashboards */}
        <PageCard
          title="Open Metabase"
          description="Regenerate the data snapshot, sync it into Metabase, and open it in a new tab."
          className="col-span-12 lg:col-span-6"
          contentClassName="flex flex-col gap-4"
        >
          {prepareError && <ErrorBanner error={prepareError} />}

          <p className="text-sm text-muted-foreground">
            Metabase runs as a separate service with its own login. Finish its
            one-time setup wizard, create an API key, and configure the
            connection below before using this button.
          </p>

          <div>
            <Button
              onClick={() => prepareMutation.mutate()}
              disabled={!data?.configured}
              isLoading={prepareMutation.isPending}
              loadingText="Preparing…"
            >
              <ExternalLink data-icon="inline-start" />
              Regenerate data &amp; open Metabase
            </Button>
            {data && !data.configured && (
              <p className="mt-2 text-xs text-muted-foreground">
                Complete and save the connection settings to enable this.
              </p>
            )}
          </div>

          {lastResult && (
            <p className="text-xs text-success">
              Exported {Number(lastResult.postingCount).toLocaleString()} postings
              {lastResult.generatedAt
                ? ` at ${new Date(lastResult.generatedAt).toLocaleString()}`
                : ""}
              .
            </p>
          )}
        </PageCard>

        {/* Connection settings */}
        <PageCard
          title="Connection"
          description="How floatd reaches your Metabase instance and where it writes the SQLite export."
          className="col-span-12 lg:col-span-6"
          contentClassName="flex flex-col gap-4"
        >
          {error && <ErrorBanner error={error} />}
          {saveError && <ErrorBanner error={saveError} />}
          {isLoading && <Loading />}

          {data && form && (
            <>
              <div className="flex items-center gap-2 text-sm">
                {data.configured ? (
                  <>
                    <CheckCircle className="size-4 text-success" />
                    <span>Configured</span>
                  </>
                ) : (
                  <>
                    <Circle className="size-4 text-muted-foreground" />
                    <span className="text-muted-foreground">Not configured</span>
                  </>
                )}
              </div>

              <Form onSubmit={handleSave}>
                <div className="flex items-center gap-2">
                  <Checkbox
                    id="mb-enabled"
                    checked={form.enabled}
                    onCheckedChange={(v) => update("enabled", v === true)}
                  />
                  <Label htmlFor="mb-enabled">Enable custom dashboards</Label>
                </div>

                <FormField label="Metabase URL (browser)" htmlFor="mb-url">
                  <Input
                    id="mb-url"
                    placeholder="http://localhost:3000"
                    value={form.url}
                    onChange={(e) => update("url", e.target.value)}
                    className="font-mono"
                  />
                </FormField>

                <FormField
                  label="Metabase API URL (server-to-server)"
                  htmlFor="mb-api-url"
                >
                  <Input
                    id="mb-api-url"
                    placeholder="http://metabase:3000 (defaults to the browser URL)"
                    value={form.apiUrl}
                    onChange={(e) => update("apiUrl", e.target.value)}
                    className="font-mono"
                  />
                </FormField>

                <FormField label="SQLite path (inside Metabase)" htmlFor="mb-db-path">
                  <Input
                    id="mb-db-path"
                    placeholder="/float-data/exports/float.db"
                    value={form.dbPath}
                    onChange={(e) => update("dbPath", e.target.value)}
                    className="font-mono"
                  />
                </FormField>

                <FormField label="Database name in Metabase" htmlFor="mb-db-name">
                  <Input
                    id="mb-db-name"
                    placeholder="float"
                    value={form.dbName}
                    onChange={(e) => update("dbName", e.target.value)}
                  />
                </FormField>

                <FormField
                  label={data.apiKeySet ? "Replace API key" : "API key"}
                  htmlFor="mb-api-key"
                >
                  <Input
                    id="mb-api-key"
                    type="password"
                    placeholder={
                      data.apiKeySet
                        ? "Leave blank to keep the current key"
                        : "Enter Metabase API key"
                    }
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                    className="font-mono"
                  />
                </FormField>

                <div>
                  <Button
                    type="submit"
                    isLoading={saveMutation.isPending}
                    loadingText="Saving…"
                  >
                    Save
                  </Button>
                </div>
                {saved && <p className="text-xs text-success">Settings saved.</p>}
              </Form>
            </>
          )}
        </PageCard>
      </DashboardGrid>
    </Page>
  );
}
