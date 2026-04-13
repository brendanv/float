import { useState } from "react";
import { ledgerClient } from "../client.js";
import { useRpc } from "../hooks/use-rpc.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { AccountInput } from "../components/posting-fields.jsx";

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
  const accounts = useRpc(() => ledgerClient.listAccounts({}), []);

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

      {/* Card 1: Rule Editor */}
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h3 class="card-title text-base">{editingId ? "Edit Rule" : "Add Rule"}</h3>
          <form onSubmit={handleSubmit} class="space-y-3">
            <div class="flex flex-col gap-1">
              <span class="text-sm">Pattern (regex)</span>
              <input
                type="text"
                class="input input-bordered input-sm font-mono"
                placeholder="AMAZON|amazon\.com"
                value={form.pattern}
                onInput={(e) => setField("pattern", e.target.value)}
                required
              />
            </div>
            <div class="flex flex-wrap gap-3">
              <div class="flex flex-col gap-1 flex-1 min-w-40">
                <span class="text-sm">Set Payee</span>
                <input
                  type="text"
                  class="input input-bordered input-sm"
                  placeholder="Amazon"
                  value={form.payee}
                  onInput={(e) => setField("payee", e.target.value)}
                />
              </div>
              <div class="flex flex-col gap-1 flex-1 min-w-40">
                <span class="text-sm">Set Category Account</span>
                <AccountInput
                  value={form.account}
                  onChange={(v) => setField("account", v)}
                  accounts={accounts.data?.accounts ?? []}
                  placeholder="expenses:shopping"
                />
              </div>
              <div class="flex flex-col gap-1 flex-1 min-w-40">
                <span class="text-sm">Add Tags <span class="text-base-content/50 text-xs">key=val, key2</span></span>
                <input
                  type="text"
                  class="input input-bordered input-sm font-mono"
                  placeholder="source=import"
                  value={form.tags}
                  onInput={(e) => setField("tags", e.target.value)}
                />
              </div>
              <div class="flex flex-col gap-1 w-24">
                <span class="text-sm">Priority</span>
                <input
                  type="number"
                  class="input input-bordered input-sm"
                  value={form.priority}
                  onInput={(e) => setField("priority", e.target.value)}
                />
              </div>
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

      {/* Card 2: Rules list + Apply section */}
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <div class="flex items-center justify-between gap-4 flex-wrap">
            <h3 class="card-title text-base">Rules</h3>
            <button
              class="btn btn-outline btn-sm"
              onClick={handlePreviewApply}
              disabled={applyLoading}
            >
              {applyLoading ? "Previewing…" : "Preview Changes"}
            </button>
          </div>
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
                    <tr key={r.id} class={editingId === r.id ? "bg-primary/10" : ""}>
                      <td><span class="badge badge-neutral badge-sm font-mono">{r.priority}</span></td>
                      <td class="max-w-xs truncate font-mono text-xs" title={r.pattern}>{r.pattern}</td>
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
          <div class="divider my-1"></div>
          <div class="flex items-center gap-3">
            <span class="text-sm text-base-content/60 whitespace-nowrap">Test description:</span>
            <input
              type="text"
              class="input input-bordered input-sm font-mono flex-1"
              placeholder="AMAZON.COM PURCHASE"
              value={testDesc}
              onInput={(e) => setTestDesc(e.target.value)}
            />
            {testDesc && (
              matchingRule ? (
                <span class="badge badge-success gap-1 shrink-0">
                  {matchingRule.payee || matchingRule.account}
                </span>
              ) : (
                <span class="badge badge-ghost shrink-0">No match</span>
              )
            )}
          </div>
          {applyError && <ErrorBanner error={applyError} />}
          {applyResult !== null && (
            <div class="alert alert-success mt-3">
              Applied changes to {applyResult} transaction(s).
            </div>
          )}
          {applyPreviews && applyPreviews.length === 0 && (
            <p class="text-base-content/60 mt-3">No transactions match any rules.</p>
          )}
          {applyPreviews && applyPreviews.length > 0 && (
            <div class="space-y-3 mt-3">
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
                      <th>Description</th>
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
                        <td>{p.description}</td>
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
