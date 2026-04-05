import { useState } from "preact/hooks";
import { ledgerClient } from "../client.js";
import { useRpc } from "../hooks/use-rpc.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";

function emptyForm() {
  return { pattern: "", payee: "", account: "", priority: "0", tags: "" };
}

function tagsFromString(str) {
  const tags = {};
  str.split(",").forEach((part) => {
    const [k, ...rest] = part.trim().split("=");
    if (k) tags[k.trim()] = rest.join("=").trim();
  });
  return tags;
}

function tagsToString(tags) {
  if (!tags) return "";
  return Object.entries(tags)
    .map(([k, v]) => (v ? `${k}=${v}` : k))
    .join(", ");
}

export function RulesPage() {
  const rulesList = useRpc(() => ledgerClient.listRules({}), []);

  const [form, setForm] = useState(emptyForm());
  const [editingId, setEditingId] = useState(null);
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState(null);

  // Pattern test
  const [testDesc, setTestDesc] = useState("");

  // Apply rules
  const [applyPreviews, setApplyPreviews] = useState(null);
  const [applyLoading, setApplyLoading] = useState(false);
  const [applyError, setApplyError] = useState(null);
  const [selectedFids, setSelectedFids] = useState(new Set());
  const [applying, setApplying] = useState(false);
  const [applyResult, setApplyResult] = useState(null);

  function setField(name, value) {
    setForm((f) => ({ ...f, [name]: value }));
  }

  function startEdit(rule) {
    setEditingId(rule.id);
    setForm({
      pattern: rule.pattern,
      payee: rule.payee,
      account: rule.account,
      priority: String(rule.priority),
      tags: tagsToString(rule.tags),
    });
    setFormError(null);
  }

  function cancelEdit() {
    setEditingId(null);
    setForm(emptyForm());
    setFormError(null);
  }

  async function handleSubmit(e) {
    e.preventDefault();
    setFormError(null);
    setSubmitting(true);
    const payload = {
      pattern: form.pattern,
      payee: form.payee,
      account: form.account,
      priority: parseInt(form.priority, 10) || 0,
      tags: tagsFromString(form.tags),
    };
    try {
      if (editingId) {
        await ledgerClient.updateRule({ id: editingId, ...payload });
      } else {
        await ledgerClient.addRule(payload);
      }
      setEditingId(null);
      setForm(emptyForm());
      rulesList.refetch();
    } catch (err) {
      setFormError(err);
    } finally {
      setSubmitting(false);
    }
  }

  async function handleDelete(id) {
    try {
      await ledgerClient.deleteRule({ id });
      rulesList.refetch();
    } catch (err) {
      setFormError(err);
    }
  }

  // Find which rule matches the test description.
  function getMatchingRule() {
    if (!testDesc || !rulesList.data?.rules) return null;
    const lower = testDesc.toLowerCase();
    for (const r of rulesList.data.rules) {
      if (!r.pattern) continue;
      try {
        if (new RegExp(r.pattern, "i").test(testDesc)) return r;
      } catch {}
    }
    return null;
  }

  async function handlePreviewApply() {
    setApplyError(null);
    setApplyLoading(true);
    setApplyPreviews(null);
    setApplyResult(null);
    try {
      const res = await ledgerClient.previewApplyRules({ ruleIds: [], query: [] });
      setApplyPreviews(res.previews);
      setSelectedFids(new Set(res.previews.map((p) => p.fid)));
    } catch (err) {
      setApplyError(err);
    } finally {
      setApplyLoading(false);
    }
  }

  function toggleFid(fid) {
    setSelectedFids((prev) => {
      const next = new Set(prev);
      if (next.has(fid)) next.delete(fid);
      else next.add(fid);
      return next;
    });
  }

  async function handleApply() {
    if (selectedFids.size === 0) return;
    setApplyError(null);
    setApplying(true);
    try {
      const res = await ledgerClient.applyRules({
        fids: Array.from(selectedFids),
        ruleIds: [],
        query: [],
      });
      setApplyResult(res.appliedCount);
      setApplyPreviews(null);
    } catch (err) {
      setApplyError(err);
    } finally {
      setApplying(false);
    }
  }

  const matchingRule = getMatchingRule();

  return (
    <div class="space-y-6">
      <h2 class="text-2xl font-bold">Categorization Rules</h2>

      {/* Rule form */}
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h3 class="card-title text-base">{editingId ? "Edit Rule" : "Add Rule"}</h3>
          <form onSubmit={handleSubmit} class="space-y-3">
            <div class="flex flex-wrap gap-3">
              <label class="form-control flex-1 min-w-48">
                <div class="label"><span class="label-text">Pattern (regex)</span></div>
                <input
                  type="text"
                  class="input input-bordered input-sm font-mono"
                  placeholder="AMAZON|amazon\.com"
                  value={form.pattern}
                  onInput={(e) => setField("pattern", e.target.value)}
                  required
                />
              </label>
              <label class="form-control w-full sm:w-40">
                <div class="label"><span class="label-text">Priority</span></div>
                <input
                  type="number"
                  class="input input-bordered input-sm"
                  value={form.priority}
                  onInput={(e) => setField("priority", e.target.value)}
                />
              </label>
            </div>
            <div class="flex flex-wrap gap-3">
              <label class="form-control flex-1 min-w-40">
                <div class="label"><span class="label-text">Set Payee</span></div>
                <input
                  type="text"
                  class="input input-bordered input-sm"
                  placeholder="Amazon"
                  value={form.payee}
                  onInput={(e) => setField("payee", e.target.value)}
                />
              </label>
              <label class="form-control flex-1 min-w-40">
                <div class="label"><span class="label-text">Set Category Account</span></div>
                <input
                  type="text"
                  class="input input-bordered input-sm"
                  placeholder="expenses:shopping"
                  value={form.account}
                  onInput={(e) => setField("account", e.target.value)}
                />
              </label>
              <label class="form-control flex-1 min-w-40">
                <div class="label">
                  <span class="label-text">Add Tags</span>
                  <span class="label-text-alt text-base-content/50">key=val, key2=val2</span>
                </div>
                <input
                  type="text"
                  class="input input-bordered input-sm font-mono"
                  placeholder="source=import, reviewed=no"
                  value={form.tags}
                  onInput={(e) => setField("tags", e.target.value)}
                />
              </label>
            </div>
            <div class="flex gap-2">
              <button type="submit" class="btn btn-primary btn-sm" disabled={submitting}>
                {submitting ? "Saving…" : editingId ? "Update Rule" : "Add Rule"}
              </button>
              {editingId && (
                <button type="button" class="btn btn-ghost btn-sm" onClick={cancelEdit}>
                  Cancel
                </button>
              )}
            </div>
          </form>
          {formError && <ErrorBanner error={formError} />}
        </div>
      </div>

      {/* Pattern tester */}
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h3 class="card-title text-base">Test Pattern</h3>
          <div class="flex gap-3 items-end flex-wrap">
            <label class="form-control flex-1 min-w-48">
              <div class="label"><span class="label-text">Transaction description</span></div>
              <input
                type="text"
                class="input input-bordered input-sm"
                placeholder="AMAZON.COM PURCHASE"
                value={testDesc}
                onInput={(e) => setTestDesc(e.target.value)}
              />
            </label>
            {testDesc && (
              <div class="pb-1">
                {matchingRule ? (
                  <span class="badge badge-success gap-1">
                    Matches rule: <strong>{matchingRule.id}</strong>
                    {matchingRule.payee && <> → payee: {matchingRule.payee}</>}
                    {matchingRule.account && <> → account: {matchingRule.account}</>}
                  </span>
                ) : (
                  <span class="badge badge-neutral">No rule matches</span>
                )}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Rules list */}
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h3 class="card-title text-base">Rules</h3>
          {rulesList.loading && <Loading />}
          {rulesList.error && <ErrorBanner error={rulesList.error} />}
          {rulesList.data && rulesList.data.rules.length === 0 && (
            <p class="text-base-content/60">No rules yet. Add one above.</p>
          )}
          {rulesList.data && rulesList.data.rules.length > 0 && (
            <div class="overflow-x-auto">
              <table class="table table-sm">
                <thead>
                  <tr>
                    <th>Priority</th>
                    <th>Pattern</th>
                    <th>Payee</th>
                    <th>Account</th>
                    <th>Tags</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {rulesList.data.rules.map((r) => (
                    <tr key={r.id} class={editingId === r.id ? "bg-base-200" : ""}>
                      <td class="font-mono text-xs">{r.priority}</td>
                      <td class="font-mono text-xs">{r.pattern}</td>
                      <td>{r.payee || <span class="text-base-content/40">—</span>}</td>
                      <td class="font-mono text-xs">{r.account || <span class="text-base-content/40">—</span>}</td>
                      <td class="text-xs">
                        {r.tags && Object.keys(r.tags).length > 0
                          ? tagsToString(r.tags)
                          : <span class="text-base-content/40">—</span>}
                      </td>
                      <td class="flex gap-1">
                        <button
                          class="btn btn-ghost btn-xs"
                          onClick={() => startEdit(r)}
                        >
                          Edit
                        </button>
                        <button
                          class="btn btn-ghost btn-xs text-error"
                          onClick={() => handleDelete(r.id)}
                        >
                          Delete
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>

      {/* Apply rules */}
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <div class="flex items-center justify-between gap-4 flex-wrap">
            <h3 class="card-title text-base">Apply Rules Retroactively</h3>
            <button
              class="btn btn-outline btn-sm"
              onClick={handlePreviewApply}
              disabled={applyLoading}
            >
              {applyLoading ? "Previewing…" : "Preview Changes"}
            </button>
          </div>
          {applyError && <ErrorBanner error={applyError} />}
          {applyResult !== null && (
            <div class="alert alert-success">
              Applied changes to {applyResult} transaction(s).
            </div>
          )}
          {applyPreviews && applyPreviews.length === 0 && (
            <p class="text-base-content/60">No transactions match any rules.</p>
          )}
          {applyPreviews && applyPreviews.length > 0 && (
            <div class="space-y-3">
              <div class="overflow-x-auto">
                <table class="table table-sm">
                  <thead>
                    <tr>
                      <th>
                        <input
                          type="checkbox"
                          class="checkbox checkbox-sm"
                          checked={selectedFids.size === applyPreviews.length}
                          onChange={() => {
                            if (selectedFids.size === applyPreviews.length) {
                              setSelectedFids(new Set());
                            } else {
                              setSelectedFids(new Set(applyPreviews.map((p) => p.fid)));
                            }
                          }}
                        />
                      </th>
                      <th>FID</th>
                      <th>Description</th>
                      <th>Rule</th>
                      <th>Account</th>
                      <th>Payee</th>
                    </tr>
                  </thead>
                  <tbody>
                    {applyPreviews.map((p) => (
                      <tr key={p.fid}>
                        <td>
                          <input
                            type="checkbox"
                            class="checkbox checkbox-sm"
                            checked={selectedFids.has(p.fid)}
                            onChange={() => toggleFid(p.fid)}
                          />
                        </td>
                        <td class="font-mono text-xs">{p.fid}</td>
                        <td>{p.description}</td>
                        <td class="font-mono text-xs text-base-content/60">{p.matchedRuleId}</td>
                        <td class="text-xs">
                          {p.newAccount ? (
                            <span>
                              <span class="line-through text-base-content/40">{p.currentAccount}</span>
                              {" → "}
                              <span class="text-success">{p.newAccount}</span>
                            </span>
                          ) : (
                            <span class="text-base-content/40">—</span>
                          )}
                        </td>
                        <td class="text-xs">
                          {p.newPayee ? (
                            <span>
                              <span class="line-through text-base-content/40">{p.currentPayee}</span>
                              {" → "}
                              <span class="text-success">{p.newPayee}</span>
                            </span>
                          ) : (
                            <span class="text-base-content/40">—</span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <button
                class="btn btn-primary btn-sm"
                onClick={handleApply}
                disabled={applying || selectedFids.size === 0}
              >
                {applying ? "Applying…" : `Apply to ${selectedFids.size} Transaction(s)`}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
