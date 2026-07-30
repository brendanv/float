import { useEffect, useState } from "react";
import { ledgerClient } from "../client.js";
import { ErrorBanner } from "./error-banner.jsx";
import { Loading } from "./loading.jsx";
import { EmptyState } from "./empty-state.jsx";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  ResponsiveDialog,
  ResponsiveDialogClose,
  ResponsiveDialogContent,
  ResponsiveDialogDescription,
  ResponsiveDialogFooter,
  ResponsiveDialogHeader,
  ResponsiveDialogTitle,
} from "@/components/ui/responsive-dialog";
import { CircleCheck, Sparkles } from "lucide-react";

const ISSUE_LABELS = {
  duplicate: "Duplicate",
  contradiction: "Contradiction",
  combinable: "Combinable",
};

const ISSUE_BADGE_VARIANTS = {
  duplicate: "secondary",
  contradiction: "destructive",
  combinable: "outline",
};

function ruleLabel(rule) {
  if (!rule) return null;
  return rule.payee || rule.account || rule.pattern;
}

export function RuleIssuesDialog({ open, onOpenChange, rules }) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [issues, setIssues] = useState(null);

  async function scan() {
    setError(null);
    setLoading(true);
    try {
      const res = await ledgerClient.findRuleIssues({});
      setIssues(res.issues ?? []);
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    if (open) scan();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  function handleOpenChange(val) {
    if (!val) {
      setIssues(null);
      setError(null);
    }
    onOpenChange(val);
  }

  const rulesById = new Map(rules.map((r) => [r.id, r]));

  return (
    <ResponsiveDialog open={open} onOpenChange={handleOpenChange}>
      <ResponsiveDialogContent className="sm:max-w-2xl">
        <ResponsiveDialogHeader>
          <ResponsiveDialogTitle>Rule Issues</ResponsiveDialogTitle>
          <ResponsiveDialogDescription>
            Duplicate, contradictory, or combinable rules found by scanning your categorization rules.
          </ResponsiveDialogDescription>
        </ResponsiveDialogHeader>

        {loading && <Loading />}
        {error && <ErrorBanner error={error} />}

        {!loading && !error && issues && issues.length === 0 && (
          <EmptyState
            icon={CircleCheck}
            title="No issues found"
            description="Your rules look clean — no duplicates, contradictions, or obvious merge opportunities."
          />
        )}

        {!loading && !error && issues && issues.length > 0 && (
          <div className="flex max-h-[50vh] flex-col gap-2 overflow-y-auto">
            {issues.map((issue, i) => (
              <div key={i} className="flex flex-col gap-2 rounded border p-3">
                <div className="flex items-center gap-2">
                  <Badge variant={ISSUE_BADGE_VARIANTS[issue.issueType] ?? "outline"}>
                    {ISSUE_LABELS[issue.issueType] ?? issue.issueType}
                  </Badge>
                  <span className="text-xs text-muted-foreground">
                    {issue.ruleIds.length} rules
                  </span>
                </div>
                <div className="flex flex-col gap-1">
                  {issue.ruleIds.map((id) => {
                    const rule = rulesById.get(id);
                    return (
                      <div key={id} className="flex flex-wrap items-center gap-1.5 text-xs">
                        <code className="rounded bg-muted px-1.5 py-0.5 font-mono">
                          {rule ? rule.pattern : id}
                        </code>
                        {rule && (
                          <span className="text-muted-foreground">→ {ruleLabel(rule)}</span>
                        )}
                        {!rule && (
                          <span className="text-muted-foreground/60">(rule no longer exists)</span>
                        )}
                      </div>
                    );
                  })}
                </div>
                {issue.explanation && (
                  <p className="text-xs text-muted-foreground">{issue.explanation}</p>
                )}
              </div>
            ))}
          </div>
        )}

        <ResponsiveDialogFooter>
          <ResponsiveDialogClose asChild>
            <Button variant="outline" size="sm" disabled={loading}>Close</Button>
          </ResponsiveDialogClose>
          <Button
            size="sm"
            onClick={scan}
            disabled={loading}
            isLoading={loading}
            loadingText="Scanning…"
          >
            <Sparkles data-icon="inline-start" className="size-3.5" />
            Rescan
          </Button>
        </ResponsiveDialogFooter>
      </ResponsiveDialogContent>
    </ResponsiveDialog>
  );
}
