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
import { CheckCircle, Circle } from "lucide-react";

export function SettingsPage() {
  const queryClient = useQueryClient();

  const { data, isLoading, error: fetchError } = useQuery({
    queryKey: queryKeys.alphaVantageConfig(),
    queryFn: () => ledgerClient.getAlphaVantageConfig({}),
  });

  const [apiKey, setApiKey] = useState("");
  const [mutationError, setMutationError] = useState(null);
  const [saved, setSaved] = useState(false);

  const setKeyMutation = useMutation({
    mutationFn: (key) => ledgerClient.setAlphaVantageApiKey({ apiKey: key }),
    onSuccess: () => {
      setApiKey("");
      setSaved(true);
      setMutationError(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.alphaVantageConfig() });
      setTimeout(() => setSaved(false), 3000);
    },
    onError: (err) => {
      setMutationError(err);
      setSaved(false);
    },
  });

  function handleSave(e) {
    e.preventDefault();
    setMutationError(null);
    setSaved(false);
    setKeyMutation.mutate(apiKey);
  }

  function handleClear() {
    if (!confirm("Clear the AlphaVantage API key?")) return;
    setMutationError(null);
    setSaved(false);
    setKeyMutation.mutate("");
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
          {fetchError && <ErrorBanner error={fetchError} />}
          {mutationError && <ErrorBanner error={mutationError} />}

          {isLoading && <Loading />}

          {data && (
            <>
              <div className="flex items-center gap-2 text-sm">
                {data.apiKeyConfigured ? (
                  <>
                    <CheckCircle className="size-4 text-success" />
                    <span>API key configured</span>
                    <Badge variant="secondary" className="font-mono">
                      {data.apiKeyPreview}
                    </Badge>
                  </>
                ) : (
                  <>
                    <Circle className="size-4 text-muted-foreground" />
                    <span className="text-muted-foreground">No API key set</span>
                  </>
                )}
              </div>

              <form onSubmit={handleSave} className="flex flex-col gap-3">
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="api-key">
                    {data.apiKeyConfigured ? "Replace API key" : "Set API key"}
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

                {saved && (
                  <p className="text-sm text-success">API key saved.</p>
                )}
              </form>

              {data.apiKeyConfigured && (
                <div>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="text-destructive hover:text-destructive"
                    disabled={setKeyMutation.isPending}
                    onClick={handleClear}
                  >
                    Clear API key
                  </Button>
                </div>
              )}
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
