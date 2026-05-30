import { useState } from "react";
import { ChevronsUpDown } from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@/components/ui/drawer";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { useIsMobile } from "@/hooks/use-mobile";
import { cn } from "@/lib/utils";

// Generic typeahead combobox. Provides the trigger button + popover + cmdk
// filtering. Options can be plain strings or {value, label} objects.
//
// On desktop the list is shown in a popover anchored to the trigger. On mobile
// it opens in a bottom drawer instead: an anchored popover gets dismissed when
// the on-screen keyboard resizes the viewport and scrolls the trigger out of
// view, which made inline editing (e.g. the account selector) nearly unusable.
//
// Props:
//   value           — currently selected value (string)
//   onChange        — called with the new value
//   options         — array of strings or {value, label} objects
//   placeholder     — placeholder shown in the trigger when no value
//   searchPlaceholder — placeholder for the search input inside the popover
//   emptyMessage    — text shown when no options match
//   allowCustomValue — when true, lets users pick the search term verbatim
//                     (shows "Use \"foo\"" item). Defaults to false.
//   className       — extra classes on the trigger
//   id              — id attached to the trigger (for label htmlFor)
export function Combobox({
  value,
  onChange,
  options,
  placeholder = "Select…",
  searchPlaceholder,
  emptyMessage = "No matches.",
  allowCustomValue = false,
  disabled = false,
  className,
  id,
}) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const isMobile = useIsMobile();

  const normalized = (options || []).map((o) =>
    typeof o === "string" ? { value: o, label: o } : o
  );
  const exactMatch = normalized.some(
    (o) => o.value.toLowerCase() === search.toLowerCase()
  );

  function select(next) {
    onChange(next === value ? "" : next);
    setSearch("");
    setOpen(false);
  }

  function pickCustom() {
    onChange(search);
    setSearch("");
    setOpen(false);
  }

  const trigger = (
    <button
      id={id}
      type="button"
      role="combobox"
      aria-expanded={open}
      disabled={disabled}
      className={cn(
        "flex h-8 w-full min-w-0 items-center justify-between rounded-none border border-input bg-background px-2.5 text-xs outline-offset-0 outline-none transition-colors hover:bg-background focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50",
        !value && "text-muted-foreground",
        className
      )}
    >
      <span className="truncate">
        {value
          ? normalized.find((o) => o.value === value)?.label ?? value
          : placeholder}
      </span>
      <ChevronsUpDown className="ml-1 size-3.5 shrink-0 opacity-50" />
    </button>
  );

  const list = (
    <Command>
      <CommandInput
        placeholder={searchPlaceholder ?? `Search…`}
        value={search}
        onValueChange={setSearch}
      />
      <CommandList>
        <CommandEmpty>
          {allowCustomValue && search ? (
            <button
              type="button"
              className="w-full px-2 py-1.5 text-left text-xs hover:bg-accent hover:text-accent-foreground"
              onClick={pickCustom}
            >
              Use "<span className="font-medium">{search}</span>"
            </button>
          ) : (
            emptyMessage
          )}
        </CommandEmpty>
        <CommandGroup>
          {allowCustomValue && !exactMatch && search && (
            <CommandItem
              value={`__use__${search}`}
              onSelect={pickCustom}
              className="text-muted-foreground"
            >
              Use "<span className="font-medium text-foreground">{search}</span>"
            </CommandItem>
          )}
          {normalized.map((o) => (
            <CommandItem
              key={o.value}
              value={o.value}
              onSelect={() => select(o.value)}
              data-checked={value === o.value ? "true" : undefined}
            >
              {o.label}
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </Command>
  );

  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent className="max-h-[85vh]">
          <DrawerHeader className="pb-1 text-left">
            <DrawerTitle className="text-sm">{placeholder}</DrawerTitle>
          </DrawerHeader>
          <div className="min-h-0 flex-1 overflow-hidden px-2 pb-[max(env(safe-area-inset-bottom),0.75rem)]">
            {list}
          </div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger render={trigger} />
      <PopoverContent align="start" className="w-(--anchor-width) p-0">
        {list}
      </PopoverContent>
    </Popover>
  );
}
