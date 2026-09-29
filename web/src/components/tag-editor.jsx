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
  const [editingKey, setEditingKey] = useState(null);
  const [editKey, setEditKey] = useState("");
  const [editValue, setEditValue] = useState("");
  const [editWorking, setEditWorking] = useState(false);
  const [editError, setEditError] = useState(null);

  const isBusy = working || removingKey !== null || editWorking;

  function startEdit(key, value) {
    if (isBusy || adding || editingKey !== null) return;
    setEditingKey(key);
    setEditKey(key);
    setEditValue(value || "");
    setEditError(null);
  }

  function cancelEdit() {
    setEditingKey(null);
    setEditKey("");
    setEditValue("");
    setEditError(null);
  }

  async function saveEdit() {
    const newKey = editKey.trim();
    if (!newKey) return;
    const newValue = editValue.trim();
    const oldKey = editingKey;
    if (controlled) {
      const next = { ...currentTags };
      if (newKey !== oldKey) delete next[oldKey];
      next[newKey] = newValue;
      onChange(next);
      cancelEdit();
      return;
    }
    setEditWorking(true);
    setEditError(null);
    try {
      const operations = [];
      if (newKey !== oldKey) {
        operations.push({ operation: { case: "removeTag", value: { key: oldKey } } });
      }
      operations.push({ operation: { case: "addTag", value: { key: newKey, value: newValue } } });
      await ledgerClient.bulkEditTransactions({ fids: [fid], operations });
      if (onChanged) onChanged();
      cancelEdit();
    } catch (err) {
      setEditError(err.message || String(err));
    } finally {
      setEditWorking(false);
    }
  }

  const onEditKeyDown = inlineEditKeyHandler({ onSave: saveEdit, onCancel: cancelEdit });

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
      await ledgerClient.bulkEditTransactions({
        fids: [fid],
        operations: [{ operation: { case: "removeTag", value: { key } } }],
      });
      if (onChanged) onChanged();
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
      await ledgerClient.bulkEditTransactions({
        fids: [fid],
        operations: [{ operation: { case: "addTag", value: { key: tagKey.trim(), value: tagValue.trim() } } }],
      });
      setTagKey("");
      setTagValue("");
      setAdding(false);
      if (onChanged) onChanged();
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
        {Object.entries(currentTags).map(([k, v]) =>
          editingKey === k ? (
            <span key={k} className="flex flex-wrap items-center gap-1">
              <Input
                className="h-6 w-24"
                placeholder="key"
                value={editKey}
                onChange={(e) => setEditKey(e.target.value)}
                onKeyDown={onEditKeyDown}
                autoFocus
              />
              <Input
                className="h-6 w-28"
                placeholder="value (optional)"
                value={editValue}
                onChange={(e) => setEditValue(e.target.value)}
                onKeyDown={onEditKeyDown}
              />
              <Button type="button" variant="ghost" size="icon-xs" onClick={saveEdit} disabled={!editKey.trim()} isLoading={editWorking} title="Save tag">
                <Check className="size-3" />
              </Button>
              <Button type="button" variant="ghost" size="icon-xs" onClick={cancelEdit} disabled={editWorking} title="Cancel">
                <X className="size-3" />
              </Button>
              {editError && <p className="basis-full text-[11px] leading-snug text-destructive">{editError}</p>}
            </span>
          ) : (
            <Badge key={k} variant="secondary" className="text-xs gap-1 pr-1">
              <button
                type="button"
                className="rounded-sm hover:underline disabled:opacity-50"
                onClick={(e) => { e.stopPropagation(); startEdit(k, v); }}
                disabled={isBusy}
                title={`Edit tag "${k}"`}
              >
                {v ? `${k}:${v}` : k}
              </button>
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
          ),
        )}
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
            disabled={isBusy || editingKey !== null}
          >
            <Plus data-icon="inline-start" /> Tag
          </Button>
        )}
      </div>
      {error && <p className="mt-1 text-[11px] leading-snug text-destructive">{error}</p>}
    </div>
  );
}
