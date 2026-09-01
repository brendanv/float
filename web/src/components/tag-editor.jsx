import { useState } from "react";
import { Check, Loader2, Plus, X } from "lucide-react";
import { ledgerClient } from "../client.js";
import { inlineEditKeyHandler } from "./inline-edit.jsx";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

// TagEditor edits a transaction's tags. In immediate-save mode (`fid` set)
// each add/remove fires its own bulkEditTransactions call. In controlled
// mode (`value`/`onChange` set) edits only update local state so they can
// be saved together with other field changes (or submitted as part of a
// new transaction).
export function TagEditor({ fid, tags, onChanged, className, value, onChange }) {
  const controlled = onChange != null;
  const currentTags = controlled ? (value || {}) : (tags || {});
  const [adding, setAdding] = useState(false);
  const [tagKey, setTagKey] = useState("");
  const [tagValue, setTagValue] = useState("");
  const [working, setWorking] = useState(false);
  const [removingKey, setRemovingKey] = useState(null);
  const [error, setError] = useState(null);

  const isBusy = working || removingKey !== null;

  async function removeTag(key) {
    if (controlled) {
      const next = { ...currentTags };
      delete next[key];
      onChange(next);
      return;
    }
    setRemovingKey(key);
    setError(null);
    try {
      const resp = await ledgerClient.bulkEditTransactions({
        fids: [fid],
        operations: [{ operation: { case: "removeTag", value: { key } } }],
      });
      if (onChanged) onChanged(resp.transactions?.[0]);
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setRemovingKey(null);
    }
  }

  async function addTag() {
    if (!tagKey.trim()) return;
    if (controlled) {
      onChange({ ...currentTags, [tagKey.trim()]: tagValue.trim() });
      setTagKey("");
      setTagValue("");
      setAdding(false);
      return;
    }
    setWorking(true);
    setError(null);
    try {
      const resp = await ledgerClient.bulkEditTransactions({
        fids: [fid],
        operations: [{ operation: { case: "addTag", value: { key: tagKey.trim(), value: tagValue.trim() } } }],
      });
      setTagKey("");
      setTagValue("");
      setAdding(false);
      if (onChanged) onChanged(resp.transactions?.[0]);
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setWorking(false);
    }
  }

  function cancelAdd() {
    setAdding(false);
    setTagKey("");
    setTagValue("");
    setError(null);
  }

  const onKey = inlineEditKeyHandler({ onSave: addTag, onCancel: cancelAdd });

  return (
    <div className={className}>
      <div className="flex flex-wrap items-center gap-1">
        {Object.entries(currentTags).map(([k, v]) => (
          <Badge key={k} variant="secondary" className="text-xs gap-1 pr-1">
            {v ? `${k}:${v}` : k}
            {removingKey === k ? (
              <Loader2 className="size-2.5 animate-spin" />
            ) : (
              <button
                type="button"
                className="rounded-sm p-0.5 hover:bg-foreground/20 disabled:opacity-50"
                onClick={(e) => { e.stopPropagation(); removeTag(k); }}
                disabled={isBusy}
                title={`Remove tag "${k}"`}
              >
                <X className="size-2.5" />
              </button>
            )}
          </Badge>
        ))}
        {adding ? (
          <span className="flex flex-wrap items-center gap-1">
            <Input
              className="h-6 w-24"
              placeholder="key"
              value={tagKey}
              onChange={(e) => setTagKey(e.target.value)}
              onKeyDown={onKey}
              autoFocus
            />
            <Input
              className="h-6 w-28"
              placeholder="value (optional)"
              value={tagValue}
              onChange={(e) => setTagValue(e.target.value)}
              onKeyDown={onKey}
            />
            <Button type="button" variant="ghost" size="icon-xs" onClick={addTag} disabled={!tagKey.trim()} isLoading={working} title="Add tag">
              <Check className="size-3" />
            </Button>
            <Button type="button" variant="ghost" size="icon-xs" onClick={cancelAdd} disabled={working} title="Cancel">
              <X className="size-3" />
            </Button>
          </span>
        ) : (
          <Button
            type="button"
            variant="ghost"
            size="xs"
            className="text-muted-foreground"
            onClick={(e) => { e.stopPropagation(); setAdding(true); }}
            disabled={isBusy}
          >
            <Plus data-icon="inline-start" /> Tag
          </Button>
        )}
      </div>
      {error && <p className="mt-1 text-[11px] leading-snug text-destructive">{error}</p>}
    </div>
  );
}
