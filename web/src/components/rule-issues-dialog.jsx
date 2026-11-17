import { useEffect, useState } from "react";
import { ledgerClient } from "../client.js";
import { ErrorBanner } from "./error-banner.jsx";
import { Loading } from "./loading.jsx";
import { EmptyState } from "./empty-state.jsx";
import { Button } from "@/components/ui/button";
import { Notice } from "@/components/ui/alert";
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

// Contradictions change outcomes (critical); duplicates are worth a look
// (warning); combinable rules are a tidy-up suggestion (info).
const ISSUE_NOTICE_VARIANTS = {
  duplicate: "warning",
  contradiction: "critical",
  combinable: "info",
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
          <ResponsiveDialogTitle>Rule issues</ResponsiveDialogTitle>
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
          <div data-slot="rule-issues" className="flex max-h-[50vh] flex-col gap-2 overflow-y-auto">
            {issues.map((issue, i) => (
              <Notice
                key={i}
                variant={ISSUE_NOTICE_VARIANTS[issue.issueType] ?? "info"}
                className="items-start before:mt-1.5"
              >
                <span className="flex flex-col gap-1.5">
                  <span className="flex items-baseline gap-2">
                    <span className="font-medium">{ISSUE_LABELS[issue.issueType] ?? issue.issueType}</span>
                    <span className="text-muted-foreground">{issue.ruleIds.length} rules</span>
                  </span>
                  <span className="flex flex-col gap-1">
                    {issue.ruleIds.map((id) => {
                      const rule = rulesById.get(id);
                      return (
                        <span key={id} className="flex flex-wrap items-center gap-x-1.5 gap-y-0.5">
                          <code className="font-mono text-code">{rule ? rule.pattern : id}</code>
                          {rule && (
                            <span className="text-muted-foreground">→ {ruleLabel(rule)}</span>
                          )}
                          {!rule && (
                            <span className="text-faint">(rule no longer exists)</span>
                          )}
                        </span>
                      );
                    })}
                  </span>
                  {issue.explanation && (
                    <span className="text-muted-foreground">{issue.explanation}</span>
                  )}
                </span>
              </Notice>
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
            <Sparkles data-icon="inline-start" />
            Rescan
          </Button>
        </ResponsiveDialogFooter>
      </ResponsiveDialogContent>
    </ResponsiveDialog>
  );
}
