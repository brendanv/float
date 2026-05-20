import { useState } from "react";
import { Check, Loader2, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

// InlineEdit is the shared "click-to-edit" cell pattern used inside table
// rows. The display slot is rendered until the user activates it; the editor
// slot is rendered with Save/Cancel icon buttons and consistent error/loading
// styling on either side.
//
// Callers manage the editing state and provide:
//   - display: ReactNode rendered when not editing
//   - canEdit: whether activation is allowed
//   - editing: boolean
//   - onActivate: called when the user clicks the display target
//   - onCancel:   called on Cancel button or Escape
//   - onSave:     called on Save button or Enter
//   - editor:     ReactNode (the input/combobox) shown while editing
//   - loading:    true while async setup (e.g. fetching the full row to edit)
//   - saving:     true while the save mutation is in flight
//   - error:      string error message
//   - title:      tooltip on the display target
//   - displayClassName / editorClassName: extra classes
export function InlineEdit({
  display,
  canEdit = true,
  editing,
  onActivate,
  onCancel,
  onSave,
  editor,
  loading = false,
  saving = false,
  error,
  title,
  displayClassName,
  editorClassName,
}) {
  if (loading) {
    return (
      <span className="inline-flex items-center">
        <Loader2 className="size-3 animate-spin" />
      </span>
    );
  }

  if (editing) {
    return (
      <span
        className={cn("flex min-w-0 flex-wrap items-center gap-1", editorClassName)}
        onClick={(e) => e.stopPropagation()}
      >
        <span className="flex min-w-0 flex-1 items-center">{editor}</span>
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={(e) => { e.stopPropagation(); onSave(); }}
          disabled={saving}
          title="Save"
        >
          {saving ? <Loader2 className="size-3 animate-spin" /> : <Check className="size-3" />}
        </Button>
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={(e) => { e.stopPropagation(); onCancel(); }}
          disabled={saving}
          title="Cancel"
        >
          <X className="size-3" />
        </Button>
        {error && (
          <span className="basis-full text-[11px] leading-snug text-destructive">
            {error}
          </span>
        )}
      </span>
    );
  }

  if (!canEdit) {
    return <span className={displayClassName}>{display}</span>;
  }

  return (
    <span
      onClick={(e) => { e.stopPropagation(); onActivate?.(); }}
      className={cn(
        "cursor-text decoration-dotted hover:underline",
        displayClassName,
      )}
      title={title}
    >
      {display}
    </span>
  );
}

// Helper for editors that should save on Enter and cancel on Escape.
export function inlineEditKeyHandler({ onSave, onCancel }) {
  return (e) => {
    if (e.key === "Enter") { e.preventDefault(); onSave?.(); }
    if (e.key === "Escape") { e.preventDefault(); onCancel?.(); }
  };
}

// useInlineEditState is a small convenience hook for the standard pattern:
//   editing flag, draft value, saving flag, error message, with a save()
//   that wraps an async mutation. Use directly or compose your own.
export function useInlineEditState(initial) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(initial);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  function start(value = initial) {
    setDraft(value);
    setEditing(true);
    setError(null);
  }
  function cancel() {
    setEditing(false);
    setError(null);
  }
  async function run(fn) {
    setSaving(true);
    setError(null);
    try {
      await fn();
      setEditing(false);
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setSaving(false);
    }
  }

  return { editing, draft, setDraft, saving, error, setError, start, cancel, run };
}
