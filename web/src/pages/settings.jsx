import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { Page, Section } from "../components/page.jsx";
import { Timestamp } from "../components/timestamp.jsx";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Form, FormField, FormActions } from "@/components/ui/form";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@/components/ui/collapsible";
import { NativeSelect, NativeSelectOption, NativeSelectOptGroup } from "@/components/ui/native-select";
import { ChevronDown, Download } from "lucide-react";
import { cn } from "@/lib/utils";

export function SettingsPage() {
  const queryClient = useQueryClient();

  // --- Stripe Customer ID ---
  const { data: stripeData, isLoading: stripeLoading, error: stripeFetchError } = useQuery({
    queryKey: queryKeys.stripeConfig(),
    queryFn: () => ledgerClient.getStripeConfig({}),
  });

  const [customerIdInput, setCustomerIdInput] = useState("");
  const [stripeMutationError, setStripeMutationError] = useState(null);
  const [stripeSaved, setStripeSaved] = useState(false);

  const setCustomerIdMutation = useMutation({
    mutationFn: (customerId) => ledgerClient.setStripeCustomerId({ customerId }),
    onSuccess: () => {
      setCustomerIdInput("");
      setStripeSaved(true);
      setStripeMutationError(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.stripeConfig() });
      setTimeout(() => setStripeSaved(false), 3000);
    },
    onError: (err) => {
      setStripeMutationError(err);
      setStripeSaved(false);
    },
  });

  function handleStripeCustomerIdSave(e) {
    e.preventDefault();
    setStripeMutationError(null);
    setStripeSaved(false);
    setCustomerIdMutation.mutate(customerIdInput.trim());
  }

  function handleStripeCustomerIdClear() {
    if (!confirm("Clear the Stripe customer ID? This will disconnect you from any existing Stripe customer.")) return;
    setStripeMutationError(null);
    setStripeSaved(false);
    setCustomerIdMutation.mutate("");
  }

  // --- Stripe Daily Auto-Import ---
  const [dailyImportError, setDailyImportError] = useState(null);

  const setDailyImportMutation = useMutation({
    mutationFn: (enabled) => ledgerClient.setStripeDailyImportEnabled({ enabled }),
    onSuccess: () => {
      setDailyImportError(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.stripeConfig() });
    },
    onError: (err) => setDailyImportError(err),
  });

  const dailyImportAvailable = !!stripeData?.enabled && (stripeData?.linkedAccountCount ?? 0) > 0;

  function handleDailyImportToggle(checked) {
    setDailyImportError(null);
    setDailyImportMutation.mutate(checked === true);
  }

  // --- General Config (Timezone) ---
  const { data: generalData, isLoading: generalLoading, error: generalFetchError } = useQuery({
    queryKey: queryKeys.generalConfig(),
    queryFn: () => ledgerClient.getGeneralConfig({}),
  });

  const [timezoneInput, setTimezoneInput] = useState(null); // null = not yet initialized
  const [timezoneMutationError, setTimezoneMutationError] = useState(null);
  const [timezoneSaved, setTimezoneSaved] = useState(false);

  const setTimezoneMutation = useMutation({
    mutationFn: (tz) => ledgerClient.setTimezone({ timezone: tz }),
    onSuccess: () => {
      setTimezoneSaved(true);
      setTimezoneMutationError(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.generalConfig() });
      setTimeout(() => setTimezoneSaved(false), 3000);
    },
    onError: (err) => {
      setTimezoneMutationError(err);
      setTimezoneSaved(false);
    },
  });

  if (generalData && timezoneInput === null) {
    setTimezoneInput(generalData.timezone || "");
  }

  function handleTimezoneSave(e) {
    e.preventDefault();
    setTimezoneMutationError(null);
    setTimezoneSaved(false);
    setTimezoneMutation.mutate(timezoneInput);
  }

  // --- Alpha Vantage ---
  const { data: avData, isLoading: avLoading, error: avFetchError } = useQuery({
    queryKey: queryKeys.alphaVantageConfig(),
    queryFn: () => ledgerClient.getAlphaVantageConfig({}),
  });

  const [apiKey, setApiKey] = useState("");
  const [avMutationError, setAvMutationError] = useState(null);
  const [avSaved, setAvSaved] = useState(false);

  const setKeyMutation = useMutation({
    mutationFn: (key) => ledgerClient.setAlphaVantageApiKey({ apiKey: key }),
    onSuccess: () => {
      setApiKey("");
      setAvSaved(true);
      setAvMutationError(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.alphaVantageConfig() });
      setTimeout(() => setAvSaved(false), 3000);
    },
    onError: (err) => {
      setAvMutationError(err);
      setAvSaved(false);
    },
  });

  function handleAvSave(e) {
    e.preventDefault();
    setAvMutationError(null);
    setAvSaved(false);
    setKeyMutation.mutate(apiKey);
  }

  function handleAvClear() {
    if (!confirm("Clear the AlphaVantage API key?")) return;
    setAvMutationError(null);
    setAvSaved(false);
    setKeyMutation.mutate("");
  }

  // --- AI Model ---
  const { data: aiData, isLoading: aiLoading, error: aiFetchError } = useQuery({
    queryKey: queryKeys.aiConfig(),
    queryFn: () => ledgerClient.getAIConfig({}),
  });

  const [modelInput, setModelInput] = useState("");
  const [aiMutationError, setAiMutationError] = useState(null);
  const [aiSaved, setAiSaved] = useState(false);

  const setModelMutation = useMutation({
    mutationFn: (model) => ledgerClient.setAIModel({ model }),
    onSuccess: () => {
      setModelInput("");
      setAiSaved(true);
      setAiMutationError(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.aiConfig() });
      setTimeout(() => setAiSaved(false), 3000);
    },
    onError: (err) => {
      setAiMutationError(err);
      setAiSaved(false);
    },
  });

  function handleAiSave(e) {
    e.preventDefault();
    setAiMutationError(null);
    setAiSaved(false);
    setModelMutation.mutate(modelInput.trim());
  }

  function handleAiReset() {
    if (!confirm("Reset the AI model to the default?")) return;
    setAiMutationError(null);
    setAiSaved(false);
    setModelMutation.mutate("");
  }

  // --- AI Prompt ---
  const [promptInput, setPromptInput] = useState(null); // null = not yet initialized from server
  const [promptMutationError, setPromptMutationError] = useState(null);
  const [promptSaved, setPromptSaved] = useState(false);

  const setPromptMutation = useMutation({
    mutationFn: (prompt) => ledgerClient.setAIPrompt({ prompt }),
    onSuccess: () => {
      setPromptSaved(true);
      setPromptMutationError(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.aiConfig() });
      setTimeout(() => setPromptSaved(false), 3000);
    },
    onError: (err) => {
      setPromptMutationError(err);
      setPromptSaved(false);
    },
  });

  // Initialise textarea from server data once loaded (only on first load).
  if (aiData && promptInput === null) {
    setPromptInput(aiData.prompt ?? "");
  }

  function handlePromptSave(e) {
    e.preventDefault();
    setPromptMutationError(null);
    setPromptSaved(false);
    setPromptMutation.mutate(promptInput);
  }

  // --- Export Journal ---
  const [exportError, setExportError] = useState(null);

  const exportJournalMutation = useMutation({
    mutationFn: () => ledgerClient.exportJournal({}),
    onSuccess: (data) => {
      setExportError(null);
      const blob = new Blob([data.content], { type: "text/plain" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = data.filename || "float-export.journal";
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    },
    onError: (err) => setExportError(err),
  });

  // --- Collapsible state (null = not yet initialized; defaults to open only when enabled) ---
  const [stripeOpen, setStripeOpen] = useState(null);
  const [aiOpen, setAiOpen] = useState(null);

  if (stripeData && stripeOpen === null) {
    setStripeOpen(stripeData.enabled);
  }
  if (aiData && aiOpen === null) {
    setAiOpen(aiData.enabled);
  }

  return (
    <Page>
      <Section title="Journal">
        <SettingGroups>
          <SettingGroup
            title="Timezone"
            description="Used when converting bank transaction timestamps to calendar dates. Set it to your local timezone so evening transactions don't land a day off."
          >
            {generalFetchError && <ErrorBanner error={generalFetchError} />}
            {timezoneMutationError && <ErrorBanner error={timezoneMutationError} />}
            {generalLoading && <Loading />}
            {generalData && timezoneInput !== null && (
              <Form onSubmit={handleTimezoneSave} className="gap-2">
                <div className="flex flex-wrap items-center gap-2">
                  <NativeSelect
                    id="timezone"
                    aria-label="Timezone"
                    value={timezoneInput}
                    onChange={(e) => setTimezoneInput(e.target.value)}
                  >
                    <NativeSelectOption value="">UTC (default)</NativeSelectOption>
                    <NativeSelectOptGroup label="United States">
                      <NativeSelectOption value="America/New_York">America/New_York (Eastern)</NativeSelectOption>
                      <NativeSelectOption value="America/Chicago">America/Chicago (Central)</NativeSelectOption>
                      <NativeSelectOption value="America/Denver">America/Denver (Mountain)</NativeSelectOption>
                      <NativeSelectOption value="America/Phoenix">America/Phoenix (Arizona)</NativeSelectOption>
                      <NativeSelectOption value="America/Los_Angeles">America/Los_Angeles (Pacific)</NativeSelectOption>
                      <NativeSelectOption value="America/Anchorage">America/Anchorage (Alaska)</NativeSelectOption>
                      <NativeSelectOption value="Pacific/Honolulu">Pacific/Honolulu (Hawaii)</NativeSelectOption>
                    </NativeSelectOptGroup>
                    <NativeSelectOptGroup label="Europe">
                      <NativeSelectOption value="Europe/London">Europe/London</NativeSelectOption>
                      <NativeSelectOption value="Europe/Paris">Europe/Paris</NativeSelectOption>
                      <NativeSelectOption value="Europe/Berlin">Europe/Berlin</NativeSelectOption>
                      <NativeSelectOption value="Europe/Rome">Europe/Rome</NativeSelectOption>
                      <NativeSelectOption value="Europe/Amsterdam">Europe/Amsterdam</NativeSelectOption>
                      <NativeSelectOption value="Europe/Madrid">Europe/Madrid</NativeSelectOption>
                      <NativeSelectOption value="Europe/Moscow">Europe/Moscow</NativeSelectOption>
                    </NativeSelectOptGroup>
                    <NativeSelectOptGroup label="Asia / Pacific">
                      <NativeSelectOption value="Asia/Dubai">Asia/Dubai</NativeSelectOption>
                      <NativeSelectOption value="Asia/Kolkata">Asia/Kolkata</NativeSelectOption>
                      <NativeSelectOption value="Asia/Singapore">Asia/Singapore</NativeSelectOption>
                      <NativeSelectOption value="Asia/Shanghai">Asia/Shanghai</NativeSelectOption>
                      <NativeSelectOption value="Asia/Tokyo">Asia/Tokyo</NativeSelectOption>
                      <NativeSelectOption value="Australia/Sydney">Australia/Sydney</NativeSelectOption>
                      <NativeSelectOption value="Pacific/Auckland">Pacific/Auckland</NativeSelectOption>
                    </NativeSelectOptGroup>
                  </NativeSelect>
                  <Button
                    type="submit"
                    variant="outline"
                    isLoading={setTimezoneMutation.isPending}
                    loadingText="Saving…"
                  >
                    Save
                  </Button>
                </div>
                {timezoneSaved && <Saved>Timezone saved.</Saved>}
              </Form>
            )}
          </SettingGroup>

          <SettingGroup
            title="Export journal"
            description="Download the whole hledger journal (account declarations, prices and every transaction) as one plain-text file."
          >
            {exportError && <ErrorBanner error={exportError} />}
            <div>
              <Button
                variant="outline"
                onClick={() => exportJournalMutation.mutate()}
                isLoading={exportJournalMutation.isPending}
                loadingText="Exporting…"
              >
                <Download data-icon="inline-start" />
                Download journal
              </Button>
            </div>
          </SettingGroup>
        </SettingGroups>
      </Section>

      <Section title="Alpha Vantage">
        <SettingGroups>
          <SettingGroup
            title="API key"
            description={
              <>
                Used to backfill commodity prices. Get a free key at{" "}
                <a
                  href="https://www.alphavantage.co/support/#api-key"
                  target="_blank"
                  rel="noreferrer"
                  className="text-primary underline-offset-2 hover:underline"
                >
                  alphavantage.co
                </a>
                .
              </>
            }
          >
            {avFetchError && <ErrorBanner error={avFetchError} />}
            {avMutationError && <ErrorBanner error={avMutationError} />}
            {avLoading && <Loading />}
            {avData && (
              <>
                {avData.apiKeyConfigured ? (
                  <ConfigState on>
                    API key configured <Code>{avData.apiKeyPreview}</Code>
                  </ConfigState>
                ) : (
                  <ConfigState>No API key set</ConfigState>
                )}
                <Form onSubmit={handleAvSave} className="gap-2">
                  <FormField
                    label={avData.apiKeyConfigured ? "Replace API key" : "Set API key"}
                    htmlFor="api-key"
                  >
                    <div className="flex gap-2">
                      <Input
                        id="api-key"
                        type="password"
                        placeholder="Enter Alpha Vantage API key"
                        value={apiKey}
                        onChange={(e) => setApiKey(e.target.value)}
                        className="max-w-sm font-mono text-code placeholder:font-sans placeholder:text-control"
                      />
                      <Button
                        type="submit"
                        variant="outline"
                        disabled={!apiKey}
                        isLoading={setKeyMutation.isPending}
                        loadingText="Saving…"
                      >
                        Save
                      </Button>
                    </div>
                  </FormField>
                  {avSaved && <Saved>API key saved.</Saved>}
                </Form>
                {avData.apiKeyConfigured && (
                  <ClearButton disabled={setKeyMutation.isPending} onClick={handleAvClear}>
                    Clear API key
                  </ClearButton>
                )}
              </>
            )}
          </SettingGroup>
        </SettingGroups>
      </Section>

      <Collapsible open={stripeOpen} onOpenChange={setStripeOpen}>
        <Section
          title={<SectionToggle open={stripeOpen}>Stripe</SectionToggle>}
          description={stripeData && <EnabledState on={stripeData.enabled} />}
        >
          {stripeData && !stripeData.enabled && (
            <p className="text-control text-muted-foreground">
              Set <Code>STRIPE_SECRET_KEY</Code> and <Code>STRIPE_PUBLISHABLE_KEY</Code> on the server to enable Stripe Financial Connections.
            </p>
          )}
          <CollapsibleContent>
            <SettingGroups>
              <SettingGroup
                title="Customer ID"
                description="The Stripe customer that Financial Connections accounts are linked under."
              >
                {stripeFetchError && <ErrorBanner error={stripeFetchError} />}
                {stripeMutationError && <ErrorBanner error={stripeMutationError} />}
                {stripeLoading && <Loading />}
                {stripeData && (
                  <>
                    {stripeData.customerId ? (
                      <ConfigState on>
                        Customer ID set <Code>{stripeData.customerId}</Code>
                      </ConfigState>
                    ) : (
                      <ConfigState>No customer ID set</ConfigState>
                    )}
                    <Form onSubmit={handleStripeCustomerIdSave} className="gap-2">
                      <FormField
                        label={stripeData.customerId ? "Replace customer ID" : "Set customer ID"}
                        htmlFor="stripe-customer-id"
                      >
                        <div className="flex gap-2">
                          <Input
                            id="stripe-customer-id"
                            type="text"
                            placeholder="cus_..."
                            value={customerIdInput}
                            onChange={(e) => setCustomerIdInput(e.target.value)}
                            className="max-w-sm font-mono text-code placeholder:font-sans placeholder:text-control"
                          />
                          <Button
                            type="submit"
                            variant="outline"
                            disabled={!customerIdInput.trim()}
                            isLoading={setCustomerIdMutation.isPending}
                            loadingText="Saving…"
                          >
                            Save
                          </Button>
                        </div>
                      </FormField>
                      {stripeSaved && <Saved>Customer ID saved.</Saved>}
                    </Form>
                    {stripeData.customerId && (
                      <ClearButton disabled={setCustomerIdMutation.isPending} onClick={handleStripeCustomerIdClear}>
                        Clear customer ID
                      </ClearButton>
                    )}
                  </>
                )}
              </SettingGroup>

              <SettingGroup
                title="Daily auto-import"
                description="Fetch and import new transactions from every linked account once a day."
              >
                {dailyImportError && <ErrorBanner error={dailyImportError} />}
                {stripeLoading && <Loading />}
                {stripeData && (
                  <>
                    <Label
                      htmlFor="stripe-daily-import-enabled"
                      className="flex cursor-pointer items-start gap-2.5"
                    >
                      <Checkbox
                        id="stripe-daily-import-enabled"
                        checked={!!stripeData.dailyImportEnabled}
                        disabled={!dailyImportAvailable || setDailyImportMutation.isPending}
                        onCheckedChange={handleDailyImportToggle}
                        className="mt-0.5"
                      />
                      <span className="flex flex-col gap-0.5">
                        <span className="text-control font-medium">Enable daily automatic import</span>
                        {!stripeData.enabled && (
                          <span className="text-control font-normal text-muted-foreground">
                            Requires <Code>STRIPE_SECRET_KEY</Code> to be set on the server.
                          </span>
                        )}
                        {stripeData.enabled && (stripeData.linkedAccountCount ?? 0) === 0 && (
                          <span className="text-control font-normal text-muted-foreground">
                            Link at least one bank account on the{" "}
                            <a href="#/connections" className="text-primary underline-offset-2 hover:underline">
                              Connections page
                            </a>{" "}
                            first.
                          </span>
                        )}
                      </span>
                    </Label>
                    <p className="text-control text-muted-foreground">
                      Last automatic import:{" "}
                      {stripeData.lastDailyImportAt ? <Timestamp value={stripeData.lastDailyImportAt} /> : "never"}
                    </p>
                  </>
                )}
              </SettingGroup>
            </SettingGroups>
          </CollapsibleContent>
        </Section>
      </Collapsible>

      <Collapsible open={aiOpen} onOpenChange={setAiOpen}>
        <Section
          title={<SectionToggle open={aiOpen}>AI</SectionToggle>}
          description={aiData && <EnabledState on={aiData.enabled} />}
        >
          {aiData && !aiData.enabled && (
            <p className="text-control text-muted-foreground">
              Set <Code>OPENROUTER_API_KEY</Code> on the server to enable AI features (rule suggestions, query translation).
            </p>
          )}
          <CollapsibleContent>
            <SettingGroups>
              <SettingGroup
                title="Model"
                description={
                  <>
                    OpenRouter model used for AI features (rule suggestions, query translation). Browse models at{" "}
                    <a
                      href="https://openrouter.ai/models"
                      target="_blank"
                      rel="noreferrer"
                      className="text-primary underline-offset-2 hover:underline"
                    >
                      openrouter.ai/models
                    </a>
                    .
                  </>
                }
              >
                {aiFetchError && <ErrorBanner error={aiFetchError} />}
                {aiMutationError && <ErrorBanner error={aiMutationError} />}
                {aiLoading && <Loading />}
                {aiData && (
                  <>
                    <ConfigState on={!!aiData.model}>
                      Active model <Code>{aiData.effectiveModel}</Code>
                      {!aiData.model && <span className="text-faint">(default)</span>}
                    </ConfigState>
                    <Form onSubmit={handleAiSave} className="gap-2">
                      <FormField
                        label={aiData.model ? "Replace model" : "Set model"}
                        htmlFor="ai-model"
                      >
                        <div className="flex gap-2">
                          <Input
                            id="ai-model"
                            type="text"
                            placeholder={aiData.effectiveModel}
                            value={modelInput}
                            onChange={(e) => setModelInput(e.target.value)}
                            className="max-w-sm font-mono text-code placeholder:font-sans placeholder:text-control"
                          />
                          <Button
                            type="submit"
                            variant="outline"
                            disabled={!modelInput.trim()}
                            isLoading={setModelMutation.isPending}
                            loadingText="Saving…"
                          >
                            Save
                          </Button>
                        </div>
                      </FormField>
                      {aiSaved && <Saved>Model saved.</Saved>}
                    </Form>
                    {aiData.model && (
                      <ClearButton disabled={setModelMutation.isPending} onClick={handleAiReset}>
                        Reset to default
                      </ClearButton>
                    )}
                  </>
                )}
              </SettingGroup>

              <SettingGroup
                title="Guidelines"
                description="Optional instructions added to the AI system prompt for every AI feature: your account naming conventions, preferred categorization style, or any other context for its suggestions."
              >
                {promptMutationError && <ErrorBanner error={promptMutationError} />}
                {aiLoading && <Loading />}
                {aiData && promptInput !== null && (
                  <Form onSubmit={handlePromptSave} className="gap-2">
                    <Textarea
                      id="ai-prompt"
                      aria-label="Guidelines"
                      placeholder="e.g. My accounts use kebab-case. Groceries go under expenses:food:groceries. Always prefer specific accounts over broad ones."
                      value={promptInput}
                      onChange={(e) => setPromptInput(e.target.value)}
                      className="min-h-32 font-mono text-code placeholder:font-sans placeholder:text-control"
                    />
                    <FormActions align="start">
                      <Button
                        type="submit"
                        variant="outline"
                        isLoading={setPromptMutation.isPending}
                        loadingText="Saving…"
                      >
                        Save guidelines
                      </Button>
                      {promptSaved && <Saved>Guidelines saved.</Saved>}
                    </FormActions>
                  </Form>
                )}
              </SettingGroup>
            </SettingGroups>
          </CollapsibleContent>
        </Section>
      </Collapsible>
    </Page>
  );
}

// SectionToggle is a collapsible section heading: the title plus a chevron.
function SectionToggle({ open, children }) {
  return (
    <CollapsibleTrigger className="inline-flex items-center gap-1.5 rounded-sm text-left">
      {children}
      <ChevronDown className={cn("size-3.5 shrink-0 text-faint transition-transform duration-200", !open && "-rotate-90")} />
    </CollapsibleTrigger>
  );
}

// SettingGroups stacks setting rows with hairlines between them.
function SettingGroups({ children }) {
  return <div className="flex flex-col divide-y divide-border">{children}</div>;
}

// SettingGroup is one setting row: name and explanation on the left (above
// on phones), controls on the right.
function SettingGroup({ title, description, children }) {
  return (
    <div
      data-slot="setting"
      className="grid gap-x-10 gap-y-3 py-4 first:pt-1 md:grid-cols-[minmax(0,16rem)_minmax(0,1fr)]"
    >
      <div className="flex flex-col gap-1">
        <h3 className="text-body font-medium">{title}</h3>
        {description && <p className="text-control text-muted-foreground">{description}</p>}
      </div>
      <div className="flex min-w-0 max-w-xl flex-col items-start gap-3 *:max-w-full [&>form]:w-full">{children}</div>
    </div>
  );
}

// ConfigState is the quiet state line of a setting: an inflow dot when it is
// set, a hollow ink-3 dot when it is not.
function ConfigState({ on = false, children }) {
  return (
    <p className={cn("flex flex-wrap items-center gap-x-2 gap-y-1 text-control", !on && "text-muted-foreground")}>
      <span
        aria-hidden="true"
        className={cn("size-2 shrink-0 rounded-full", on ? "bg-inflow" : "border border-faint")}
      />
      {children}
    </p>
  );
}

// EnabledState is the section-level state shown next to a collapsible
// integration's heading.
function EnabledState({ on }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span
        aria-hidden="true"
        className={cn("size-2 rounded-full", on ? "bg-inflow" : "border border-faint")}
      />
      {on ? "Enabled" : "Not configured"}
    </span>
  );
}

// Code is a literal key, ID or environment variable name.
function Code({ children }) {
  return <code className="font-mono text-code text-foreground">{children}</code>;
}

function Saved({ children }) {
  return <p role="status" className="text-control text-inflow">{children}</p>;
}

// ClearButton is a destructive "Clear …" action: a ghost button in below tone.
function ClearButton({ children, ...props }) {
  return (
    <Button variant="destructive-ghost" size="sm" className="-ml-2" {...props}>
      {children}
    </Button>
  );
}
