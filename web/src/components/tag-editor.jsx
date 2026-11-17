import { useState } from "react";
import { Check, Loader2, Plus, X } from "lucide-react";
import { ledgerClient } from "../client.js";
import { inlineEditKeyHandler } from "./inline-edit.jsx";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

// TagForm is the key/value input row shared by "add a new tag" and "edit an
// existing tag" — an add is just an edit with no original key.
function TagForm({ draft, setDraft, onSave, onCancel, working, saveTitle }) {
  const onKeyDown = inlineEditKeyHandler({ onSave, onCancel });
  return (
    <span className="flex flex-wrap items-center gap-1">
      <Input
        className="h-6 w-24"
        placeholder="key"
        value={draft.key}
        onChange={(e) => setDraft({ ...draft, key: e.target.value })}
        onKeyDown={onKeyDown}
        autoFocus
      />
      <Input
        className="h-6 w-28"
        placeholder="value (optional)"
        value={draft.value}
        onChange={(e) => setDraft({ ...draft, value: e.target.value })}
        onKeyDown={onKeyDown}
      />
      <Button type="button" variant="ghost" size="icon-xs" onClick={onSave} disabled={!draft.key.trim()} isLoading={working} title={saveTitle}>
        <Check className="size-3" />
      </Button>
      <Button type="button" variant="ghost" size="icon-xs" onClick={onCancel} disabled={working} title="Cancel">
        <X className="size-3" />
      </Button>
    </span>
  );
}

// tagLabel is how a tag reads in lists: "#key" or "#key:value".
export function tagLabel(key, value) {
  return value ? `#${key}:${value}` : `#${key}`;
}

// TagChip is the quiet tag chip (sunken fill, 4px radius, ink-2).
export function TagChip({ className, ...props }) {
  return (
    <span
      data-slot="tag"
      className={cn(
        "inline-flex h-[18px] max-w-full shrink-0 items-center whitespace-nowrap rounded-sm bg-muted px-1.5 text-[11.5px] leading-none text-muted-foreground",
        className,
      )}
      {...props}
    />
  );
}

// In quiet mode (dense rows) the remove button shows on chip hover/focus on
// devices that can hover; touch devices always show it.
const QUIET_REMOVE =
  "[@media(hover:hover)]:hidden [@media(hover:hover)]:group-hover/tag:inline-flex [@media(hover:hover)]:group-focus-within/tag:inline-flex";

// TagEditor edits a transaction's tags. In immediate-save mode (`fid` set)
// each add/remove/edit fires its own bulkEditTransactions call. In controlled
// mode (`value`/`onChange` set) edits only update local state so they can
// be saved together with other field changes (or submitted as part of a
// new transaction).
// addButtonClassName styles the "+ Tag" button (e.g. to reveal it on hover).
// quiet: dense-row mode (one line, remove buttons revealed on hover).
export function TagEditor({ fid, tags, onChanged, className, value, onChange, addButtonClassName, quiet = false }) {
  const controlled = onChange != null;
  const currentTags = controlled ? (value || {}) : (tags || {});
  // null: form closed. "": adding a new tag. "<key>": editing that tag.
  const [editing, setEditing] = useState(null);
  const [draft, setDraft] = useState({ key: "", value: "" });
  const [working, setWorking] = useState(false);
  const [removingKey, setRemovingKey] = useState(null);
  const [error, setError] = useState(null);

  const isBusy = working || removingKey !== null;

  function open(orig, value = "") {
    if (isBusy || editing !== null) return;
    setEditing(orig);
    setDraft({ key: orig, value });
    setError(null);
  }

  function close() {
    setEditing(null);
    setDraft({ key: "", value: "" });
    setError(null);
  }

  async function save() {
    const key = draft.key.trim();
    if (!key) return;
    const val = draft.value.trim();
    const renamed = editing && key !== editing;
    if (controlled) {
      const next = { ...currentTags };
      if (renamed) delete next[editing];
      next[key] = val;
      onChange(next);
      close();
      return;
    }
    setWorking(true);
    setError(null);
    try {
      const operations = [];
      if (renamed) {
        operations.push({ operation: { case: "removeTag", value: { key: editing } } });
      }
      operations.push({ operation: { case: "addTag", value: { key, value: val } } });
      await ledgerClient.bulkEditTransactions({ fids: [fid], operations });
      if (onChanged) onChanged();
      close();
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setWorking(false);
    }
  }

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

  return (
    <div className={className}>
      <div className={cn("flex items-center gap-1", quiet ? "min-w-0" : "flex-wrap")}>
        {Object.entries(currentTags).map(([k, v]) =>
          editing === k ? (
            <TagForm key={k} draft={draft} setDraft={setDraft} onSave={save} onCancel={close} working={working} saveTitle="Save tag" />
          ) : (
            <TagChip key={k} className="group/tag gap-0.5 pr-0.5">
              <button
                type="button"
                className="rounded-[3px] outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
                onClick={(e) => { e.stopPropagation(); open(k, v); }}
                disabled={isBusy}
                title={`Edit tag "${k}"`}
              >
                {tagLabel(k, v)}
              </button>
              {removingKey === k ? (
                <Loader2 className="size-2.5 animate-spin" />
              ) : (
                <button
                  type="button"
                  className={cn(
                    "rounded-[3px] p-0.5 outline-none hover:bg-foreground/10 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50",
                    quiet && QUIET_REMOVE,
                  )}
                  onClick={(e) => { e.stopPropagation(); removeTag(k); }}
                  disabled={isBusy}
                  title={`Remove tag "${k}"`}
                  aria-label={`Remove tag "${k}"`}
                >
                  <X className="size-2.5" />
                </button>
              )}
            </TagChip>
          ),
        )}
        {editing === "" ? (
          <TagForm draft={draft} setDraft={setDraft} onSave={save} onCancel={close} working={working} saveTitle="Add tag" />
        ) : (
          <Button
            type="button"
            variant="ghost"
            size="xs"
            className={cn("text-muted-foreground", quiet && "h-[18px] px-1.5 text-[11.5px]", addButtonClassName)}
            onClick={(e) => { e.stopPropagation(); open(""); }}
            disabled={isBusy || editing !== null}
          >
            <Plus data-icon="inline-start" /> Tag
          </Button>
        )}
      </div>
      {error && <p className="mt-1 text-[11px] leading-snug text-destructive">{error}</p>}
    </div>
  );
}
