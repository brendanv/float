import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Form, FormField, FormActions } from "@/components/ui/form";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@/components/ui/collapsible";
import { NativeSelect, NativeSelectOption, NativeSelectOptGroup } from "@/components/ui/native-select";
import { CheckCircle, XCircle, Circle, CreditCard, Repeat, ChevronDown, Bot, Globe } from "lucide-react";
import { cn } from "@/lib/utils";
import { useInitializedState } from "../hooks/use-initialized-state.js";

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

  const [timezoneInput, setTimezoneInput] = useInitializedState(generalData?.timezone || "", !!generalData);
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
  const [promptInput, setPromptInput] = useInitializedState(aiData?.prompt ?? "", !!aiData);
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

  function handlePromptSave(e) {
    e.preventDefault();
    setPromptMutationError(null);
    setPromptSaved(false);
    setPromptMutation.mutate(promptInput);
  }

  // --- Collapsible state (null = not yet initialized; defaults to open only when enabled) ---
  const [stripeOpen, setStripeOpen] = useInitializedState(stripeData?.enabled ?? false, !!stripeData);
  const [aiOpen, setAiOpen] = useInitializedState(aiData?.enabled ?? false, !!aiData);

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title="Settings" />

      {/* General Settings card */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Globe className="size-5" />
            General
          </CardTitle>
          <CardDescription>
            General settings that affect how float displays and processes your data.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {generalFetchError && <ErrorBanner error={generalFetchError} />}
          {timezoneMutationError && <ErrorBanner error={timezoneMutationError} />}
          {generalLoading && <Loading />}
          {generalData && timezoneInput !== null && (
            <Form onSubmit={handleTimezoneSave}>
              <FormField
                label="Timezone"
                htmlFor="timezone"
                description="Used when converting bank transaction timestamps to calendar dates. Set this to your local timezone to avoid off-by-one-day errors for transactions that occur in the evening."
              >
                <div className="flex gap-2 items-center flex-wrap">
                  <NativeSelect
                    id="timezone"
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
                    isLoading={setTimezoneMutation.isPending}
                    loadingText="Saving…"
                  >
                    Save
                  </Button>
                </div>
              </FormField>
              {timezoneSaved && (
                <p className="text-xs text-success">Timezone saved.</p>
              )}
            </Form>
          )}
        </CardContent>
      </Card>

      {/* AlphaVantage card */}
      <Card>
        <CardHeader>
          <CardTitle>AlphaVantage API Key</CardTitle>
          <CardDescription>
            Used to backfill commodity prices. Get a free key at{" "}
            <a
              href="https://www.alphavantage.co/support/#api-key"
              target="_blank"
              rel="noreferrer"
              className="underline"
            >
              alphavantage.co
            </a>
            .
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {avFetchError && <ErrorBanner error={avFetchError} />}
          {avMutationError && <ErrorBanner error={avMutationError} />}

          {avLoading && <Loading />}

          {avData && (
            <>
              <div className="flex items-center gap-2 text-sm">
                {avData.apiKeyConfigured ? (
                  <>
                    <CheckCircle className="size-4 text-success" />
                    <span>API key configured</span>
                    <Badge variant="secondary" className="font-mono">
                      {avData.apiKeyPreview}
                    </Badge>
                  </>
                ) : (
                  <>
                    <Circle className="size-4 text-muted-foreground" />
                    <span className="text-muted-foreground">No API key set</span>
                  </>
                )}
              </div>

              <Form onSubmit={handleAvSave}>
                <FormField
                  label={avData.apiKeyConfigured ? "Replace API key" : "Set API key"}
                  htmlFor="api-key"
                >
                  <div className="flex gap-2">
                    <Input
                      id="api-key"
                      type="password"
                      placeholder="Enter AlphaVantage API key"
                      value={apiKey}
                      onChange={(e) => setApiKey(e.target.value)}
                      className="max-w-sm font-mono"
                    />
                    <Button
                      type="submit"
                      disabled={!apiKey}
                      isLoading={setKeyMutation.isPending}
                      loadingText="Saving…"
                    >
                      Save
                    </Button>
                  </div>
                </FormField>
                {avSaved && (
                  <p className="text-xs text-success">API key saved.</p>
                )}
              </Form>

              {avData.apiKeyConfigured && (
                <div>
                  <Button
                    variant="destructive-ghost"
                    size="sm"
                    disabled={setKeyMutation.isPending}
                    onClick={handleAvClear}
                  >
                    Clear API key
                  </Button>
                </div>
              )}
            </>
          )}
        </CardContent>
      </Card>

      {/* Stripe collapsible card */}
      <Collapsible open={stripeOpen} onOpenChange={setStripeOpen}>
        <Card>
          <CollapsibleTrigger className="w-full text-left">
            <CardHeader className="cursor-pointer select-none hover:bg-muted/30 transition-colors">
              <CardTitle className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-2">
                  <CreditCard className="size-5" />
                  Stripe
                </div>
                <div className="flex items-center gap-2">
                  {stripeData && (
                    stripeData.enabled ? (
                      <CheckCircle className="size-4 text-success" />
                    ) : (
                      <XCircle className="size-4 text-muted-foreground" />
                    )
                  )}
                  <ChevronDown
                    className={cn(
                      "size-4 text-muted-foreground transition-transform duration-200",
                      stripeOpen && "rotate-180"
                    )}
                  />
                </div>
              </CardTitle>
              <CardDescription>
                {stripeData && !stripeData.enabled
                  ? <>Set <code className="font-mono">STRIPE_SECRET_KEY</code> and <code className="font-mono">STRIPE_PUBLISHABLE_KEY</code> on the server to enable Stripe Financial Connections.</>
                  : "Configure Stripe Financial Connections and daily auto-import."
                }
              </CardDescription>
            </CardHeader>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <CardContent className="flex flex-col gap-6 pt-0">
              {/* Customer ID section */}
              <div className="flex flex-col gap-4">
                <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  <Repeat className="size-3" />
                  Customer ID
                </div>
                {stripeFetchError && <ErrorBanner error={stripeFetchError} />}
                {stripeMutationError && <ErrorBanner error={stripeMutationError} />}
                {stripeLoading && <Loading />}
                {stripeData && (
                  <>
                    <div className="flex items-center gap-2 text-sm">
                      {stripeData.customerId ? (
                        <>
                          <CheckCircle className="size-4 text-success" />
                          <span>Customer ID set</span>
                          <Badge variant="secondary" className="font-mono">
                            {stripeData.customerId}
                          </Badge>
                        </>
                      ) : (
                        <>
                          <Circle className="size-4 text-muted-foreground" />
                          <span className="text-muted-foreground">No customer ID set</span>
                        </>
                      )}
                    </div>

                    <Form onSubmit={handleStripeCustomerIdSave}>
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
                            className="max-w-sm font-mono"
                          />
                          <Button
                            type="submit"
                            disabled={!customerIdInput.trim()}
                            isLoading={setCustomerIdMutation.isPending}
                            loadingText="Saving…"
                          >
                            Save
                          </Button>
                        </div>
                      </FormField>
                      {stripeSaved && (
                        <p className="text-xs text-success">Customer ID saved.</p>
                      )}
                    </Form>

                    {stripeData.customerId && (
                      <div>
                        <Button
                          variant="destructive-ghost"
                          size="sm"
                          disabled={setCustomerIdMutation.isPending}
                          onClick={handleStripeCustomerIdClear}
                        >
                          Clear customer ID
                        </Button>
                      </div>
                    )}
                  </>
                )}
              </div>

              {/* Divider */}
              <div className="border-t" />

              {/* Daily auto-import section */}
              <div className="flex flex-col gap-4">
                <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  <Repeat className="size-3" />
                  Daily Auto-Import
                </div>
                {dailyImportError && <ErrorBanner error={dailyImportError} />}
                {stripeLoading && <Loading />}
                {stripeData && (
                  <>
                    <Label
                      htmlFor="stripe-daily-import-enabled"
                      className="flex items-start gap-3 cursor-pointer"
                    >
                      <Checkbox
                        id="stripe-daily-import-enabled"
                        checked={!!stripeData.dailyImportEnabled}
                        disabled={!dailyImportAvailable || setDailyImportMutation.isPending}
                        onCheckedChange={handleDailyImportToggle}
                      />
                      <div className="flex flex-col gap-1">
                        <span className="text-sm font-medium">
                          Enable daily automatic import
                        </span>
                        {!stripeData.enabled && (
                          <span className="text-xs text-muted-foreground">
                            Requires <code className="font-mono">STRIPE_SECRET_KEY</code> to
                            be set on the server.
                          </span>
                        )}
                        {stripeData.enabled && (stripeData.linkedAccountCount ?? 0) === 0 && (
                          <span className="text-xs text-muted-foreground">
                            Link at least one bank account on the{" "}
                            <a href="#/connections" className="underline">
                              Connections page
                            </a>{" "}
                            first.
                          </span>
                        )}
                      </div>
                    </Label>

                    <div className="text-xs text-muted-foreground">
                      {stripeData.lastDailyImportAt ? (
                        <>Last automatic import: {stripeData.lastDailyImportAt}</>
                      ) : (
                        <>Last automatic import: never</>
                      )}
                    </div>
                  </>
                )}
              </div>
            </CardContent>
          </CollapsibleContent>
        </Card>
      </Collapsible>

      {/* AI collapsible card */}
      <Collapsible open={aiOpen} onOpenChange={setAiOpen}>
        <Card>
          <CollapsibleTrigger className="w-full text-left">
            <CardHeader className="cursor-pointer select-none hover:bg-muted/30 transition-colors">
              <CardTitle className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-2">
                  <Bot className="size-5" />
                  AI
                </div>
                <div className="flex items-center gap-2">
                  {aiData && (
                    aiData.enabled ? (
                      <CheckCircle className="size-4 text-success" />
                    ) : (
                      <XCircle className="size-4 text-muted-foreground" />
                    )
                  )}
                  <ChevronDown
                    className={cn(
                      "size-4 text-muted-foreground transition-transform duration-200",
                      aiOpen && "rotate-180"
                    )}
                  />
                </div>
              </CardTitle>
              <CardDescription>
                {aiData && !aiData.enabled
                  ? <>Set <code className="font-mono">OPENROUTER_API_KEY</code> on the server to enable AI features (rule suggestions, query translation).</>
                  : "Configure the OpenRouter AI model and guidelines for AI features."
                }
              </CardDescription>
            </CardHeader>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <CardContent className="flex flex-col gap-6 pt-0">
              {/* AI Model section */}
              <div className="flex flex-col gap-4">
                <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Model
                </div>
                <p className="text-xs text-muted-foreground">
                  OpenRouter model used for AI features (rule suggestions, query translation).
                  Requires <code className="font-mono">OPENROUTER_API_KEY</code> to be set on the server.
                  Browse available models at{" "}
                  <a
                    href="https://openrouter.ai/models"
                    target="_blank"
                    rel="noreferrer"
                    className="underline"
                  >
                    openrouter.ai/models
                  </a>
                  .
                </p>
                {aiFetchError && <ErrorBanner error={aiFetchError} />}
                {aiMutationError && <ErrorBanner error={aiMutationError} />}
                {aiLoading && <Loading />}
                {aiData && (
                  <>
                    <div className="flex items-center gap-2 text-sm">
                      <span className="text-muted-foreground">Active model:</span>
                      <Badge variant="secondary" className="font-mono">
                        {aiData.effectiveModel}
                      </Badge>
                      {!aiData.model && (
                        <span className="text-xs text-muted-foreground">(default)</span>
                      )}
                    </div>

                    <Form onSubmit={handleAiSave}>
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
                            className="max-w-sm font-mono"
                          />
                          <Button
                            type="submit"
                            disabled={!modelInput.trim()}
                            isLoading={setModelMutation.isPending}
                            loadingText="Saving…"
                          >
                            Save
                          </Button>
                        </div>
                      </FormField>
                      {aiSaved && (
                        <p className="text-xs text-success">Model saved.</p>
                      )}
                    </Form>

                    {aiData.model && (
                      <div>
                        <Button
                          variant="destructive-ghost"
                          size="sm"
                          disabled={setModelMutation.isPending}
                          onClick={handleAiReset}
                        >
                          Reset to default
                        </Button>
                      </div>
                    )}
                  </>
                )}
              </div>

              {/* Divider */}
              <div className="border-t" />

              {/* AI Guidelines section */}
              <div className="flex flex-col gap-4">
                <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Guidelines
                </div>
                <p className="text-xs text-muted-foreground">
                  Optional instructions prepended to the AI system prompt for all AI features.
                  Use this to tell the AI about your account naming conventions, preferred categorization
                  style, or any other context that should influence its suggestions.
                </p>
                {promptMutationError && <ErrorBanner error={promptMutationError} />}
                {aiLoading && <Loading />}
                {aiData && promptInput !== null && (
                  <Form onSubmit={handlePromptSave}>
                    <FormField label="Guidelines" htmlFor="ai-prompt">
                      <Textarea
                        id="ai-prompt"
                        placeholder="e.g. My accounts use kebab-case. Groceries go under expenses:food:groceries. Always prefer specific accounts over broad ones."
                        value={promptInput}
                        onChange={(e) => setPromptInput(e.target.value)}
                        className="min-h-32 font-mono text-xs"
                      />
                    </FormField>
                    <FormActions align="start">
                      <Button
                        type="submit"
                        isLoading={setPromptMutation.isPending}
                        loadingText="Saving…"
                      >
                        Save guidelines
                      </Button>
                      {promptSaved && (
                        <p className="text-xs text-success">Guidelines saved.</p>
                      )}
                    </FormActions>
                  </Form>
                )}
              </div>
            </CardContent>
          </CollapsibleContent>
        </Card>
      </Collapsible>
    </div>
  );
}
