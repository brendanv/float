import { createContext, useContext, useMemo } from "react";
import { createPortal } from "react-dom";
import { cn } from "@/lib/utils";
import { SectionTabs } from "./section-tabs.jsx";

// On grouped routes (Reports, Investments, Import, Settings) the shell
// renders a GroupHeader: the group name as the h1, an actions slot on the
// right, and the SectionTabs under it. Pages keep rendering <PageHeader>;
// inside a group it portals its actions into that slot instead of drawing a
// second title.
const GroupHeaderContext = createContext(null);

// GroupHeaderProvider marks its subtree as inside a grouped route.
// `actionsEl` is the GroupHeader's actions slot (null until it mounts).
export function GroupHeaderProvider({ actionsEl, children }) {
  const value = useMemo(() => ({ actionsEl }), [actionsEl]);
  return <GroupHeaderContext.Provider value={value}>{children}</GroupHeaderContext.Provider>;
}

// GroupHeader is the shell's header for a grouped route. `item` is the nav
// item (label + tabs); `actionsRef` receives the actions slot element.
export function GroupHeader({ item, currentPath, actionsRef }) {
  return (
    <div data-slot="group-header" className="flex flex-col gap-4">
      <div data-slot="page-header" className="flex min-h-8 flex-wrap items-center gap-x-3 gap-y-2">
        <h1 className="text-title font-semibold">{item.label}</h1>
        <div
          ref={actionsRef}
          data-slot="page-header-actions"
          className="ml-auto flex flex-wrap items-center gap-2 empty:hidden"
        />
      </div>
      <SectionTabs items={item.tabs} currentPath={currentPath} label={item.label} />
    </div>
  );
}

// PageHeader is the page title (22px) with a right-aligned actions slot.
// Pass actions as `actions` or as children. `description` is a muted line
// under the title; leave it out when it would only repeat the title.
// Inside a group (see GroupHeader) the title is the group's, so `title` is
// not drawn: actions go to the group header and the description stays here.
export function PageHeader({ title, description, actions, children, className }) {
  const slot = actions ?? children;
  const group = useContext(GroupHeaderContext);

  if (group) {
    return (
      <>
        {slot && group.actionsEl && createPortal(slot, group.actionsEl)}
        {description && (
          <p data-slot="page-description" className={cn("text-control text-muted-foreground", className)}>
            {description}
          </p>
        )}
      </>
    );
  }

  return (
    <div
      data-slot="page-header"
      className={cn("flex min-h-8 flex-wrap items-center gap-x-3 gap-y-2", className)}
    >
      <div className="flex min-w-0 flex-col gap-0.5">
        <h1 className="text-title font-semibold">{title}</h1>
        {description && (
          <p className="text-control text-muted-foreground">{description}</p>
        )}
      </div>
      {slot && (
        <div data-slot="page-header-actions" className="ml-auto flex flex-wrap items-center gap-2">
          {slot}
        </div>
      )}
    </div>
  );
}
