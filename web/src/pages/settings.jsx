import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import { CheckCircle, Circle } from "lucide-react";

export function SettingsPage() {
  const queryClient = useQueryClient();

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

  return (
    <div className="flex flex-col gap-6">
      <h2 className="text-2xl font-bold">Settings</h2>

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

              <form onSubmit={handleAvSave} className="flex flex-col gap-3">
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="api-key">
                    {avData.apiKeyConfigured ? "Replace API key" : "Set API key"}
                  </Label>
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
                      disabled={!apiKey || setKeyMutation.isPending}
                    >
                      {setKeyMutation.isPending ? "Saving…" : "Save"}
                    </Button>
                  </div>
                </div>

                {avSaved && (
                  <p className="text-sm text-success">API key saved.</p>
                )}
              </form>

              {avData.apiKeyConfigured && (
                <div>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="text-destructive hover:text-destructive"
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

      <Card>
        <CardHeader>
          <CardTitle>AI Model</CardTitle>
          <CardDescription>
            OpenRouter model used for AI features (rule suggestions, query translation).
            Requires <code className="font-mono text-xs">OPENROUTER_API_KEY</code> to be set on the server.
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
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
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

              <form onSubmit={handleAiSave} className="flex flex-col gap-3">
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="ai-model">
                    {aiData.model ? "Replace model" : "Set model"}
                  </Label>
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
                      disabled={!modelInput.trim() || setModelMutation.isPending}
                    >
                      {setModelMutation.isPending ? "Saving…" : "Save"}
                    </Button>
                  </div>
                </div>

                {aiSaved && (
                  <p className="text-sm text-success">Model saved.</p>
                )}
              </form>

              {aiData.model && (
                <div>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="text-destructive hover:text-destructive"
                    disabled={setModelMutation.isPending}
                    onClick={handleAiReset}
                  >
                    Reset to default
                  </Button>
                </div>
              )}
            </>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>AI Guidelines</CardTitle>
          <CardDescription>
            Optional instructions prepended to the AI system prompt for all AI features.
            Use this to tell the AI about your account naming conventions, preferred categorization
            style, or any other context that should influence its suggestions.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {promptMutationError && <ErrorBanner error={promptMutationError} />}

          {aiLoading && <Loading />}

          {aiData && promptInput !== null && (
            <form onSubmit={handlePromptSave} className="flex flex-col gap-3">
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="ai-prompt">Guidelines</Label>
                <Textarea
                  id="ai-prompt"
                  placeholder="e.g. My accounts use kebab-case. Groceries go under expenses:food:groceries. Always prefer specific accounts over broad ones."
                  value={promptInput}
                  onChange={(e) => setPromptInput(e.target.value)}
                  className="min-h-32 font-mono text-sm"
                />
              </div>

              <div className="flex items-center gap-3">
                <Button type="submit" disabled={setPromptMutation.isPending}>
                  {setPromptMutation.isPending ? "Saving…" : "Save guidelines"}
                </Button>
                {promptSaved && (
                  <p className="text-sm text-success">Guidelines saved.</p>
                )}
              </div>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
