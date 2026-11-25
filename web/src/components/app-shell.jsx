import { useEffect, useState } from "react";
import { Link } from "@tanstack/react-router";
import { BarChart2, House, List, Menu, Moon, Plus, Search, Sun } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
  useSidebar,
} from "@/components/ui/sidebar";
import { useIsMobile } from "@/hooks/use-mobile";
import { setThemeColor } from "@/lib/pwa.js";
import { AddTransactionModal } from "./add-transaction-modal.jsx";
import { UndoToaster } from "./undo-toaster.jsx";
import { CommandPalette } from "./command-palette.jsx";
import { NAV_GROUPS, SETTINGS_ITEM, navItemFor } from "./nav-items.js";
import { GroupHeader, GroupHeaderProvider } from "./page-header.jsx";

const IS_MAC = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform ?? "");

function Kbd({ className, children }) {
  return (
    <kbd
      className={cn(
        "ml-auto rounded-sm border border-input px-1 font-mono text-[10.5px] leading-4 font-normal text-faint",
        className,
      )}
    >
      {children}
    </kbd>
  );
}

// isTypingTarget is true when a keystroke belongs to a field, not a shortcut.
function isTypingTarget(el) {
  if (!el) return false;
  if (el.isContentEditable) return true;
  return ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName);
}

// N opens a new transaction (outside fields and dialogs); Ctrl/⌘+K toggles
// the command palette (it won't open over another dialog). Both callbacks must be stable (state setters).
function useShellShortcuts(setAddOpen, setJumpOpen) {
  useEffect(() => {
    function onKeyDown(event) {
      if (event.defaultPrevented || event.isComposing) return;
      const key = event.key.toLowerCase();
      // A dialog, sheet or menu is open (the palette itself counts).
      const overlayOpen = !!document.querySelector('[role="dialog"], [role="alertdialog"], [role="menu"]');
      if (key === "k" && (event.metaKey || event.ctrlKey) && !event.altKey) {
        event.preventDefault();
        // ⌘K closes the palette, but doesn't open it over another dialog:
        // jumping to a page would leave that dialog open on the new page.
        setJumpOpen((open) => (open || !overlayOpen ? !open : open));
        return;
      }
      if (key !== "n" || event.metaKey || event.ctrlKey || event.altKey) return;
      if (isTypingTarget(document.activeElement)) return;
      if (overlayOpen) return;
      event.preventDefault();
      setAddOpen(true);
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [setAddOpen, setJumpOpen]);
}

function useTheme() {
  const [isDark, setIsDark] = useState(
    () => localStorage.getItem("float-theme") === "dark"
  );
  function toggle() {
    const next = !isDark;
    setIsDark(next);
    document.documentElement.classList.toggle("dark", next);
    setThemeColor(next);
    localStorage.setItem("float-theme", next ? "dark" : "light");
  }
  return [isDark, toggle];
}

function ThemeToggle() {
  const [isDark, toggle] = useTheme();
  const label = isDark ? "Switch to light mode" : "Switch to dark mode";
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      onClick={toggle}
      aria-label={label}
      title={label}
      className="shrink-0 text-muted-foreground"
    >
      {isDark ? <Moon /> : <Sun />}
    </Button>
  );
}

function Wordmark({ className }) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 text-[21px] leading-none font-semibold tracking-[-0.04em] text-primary",
        className,
      )}
    >
      float
      <i aria-hidden="true" className="inline-block size-[7px] translate-y-[3px] rounded-full bg-primary" />
    </span>
  );
}

function NavItem({ item, active, onNavigate }) {
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        isActive={active}
        render={<Link to={item.href} onClick={onNavigate} aria-current={active ? "page" : undefined} />}
      >
        <item.icon />
        <span>{item.label}</span>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}

function AppSidebar({ activeId, onAddTransaction, onJump }) {
  const { isMobile, setOpenMobile } = useSidebar();
  const closeMobile = () => isMobile && setOpenMobile(false);

  return (
    <Sidebar>
      <SidebarHeader className="gap-3 px-2.5 pt-4">
        <Link
          to="/"
          onClick={closeMobile}
          aria-label="float home"
          className="self-start rounded-md px-2 py-0.5 outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Wordmark />
        </Link>
        <Button
          className="w-full justify-between"
          onClick={() => { closeMobile(); onAddTransaction(); }}
        >
          <span className="flex items-center gap-1.5">
            <Plus data-icon="inline-start" />
            New transaction
          </span>
          {!isMobile && (
            <Kbd className="ml-0 border-primary-foreground/40 text-primary-foreground">N</Kbd>
          )}
        </Button>
        <button
          type="button"
          onClick={() => { closeMobile(); onJump(); }}
          className="flex h-8 w-full items-center gap-2 rounded-md border border-input bg-background px-2 text-[12.5px] text-faint outline-none hover:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring [&_svg]:size-3.5"
        >
          <Search />
          Jump to…
          {!isMobile && <Kbd>{IS_MAC ? "⌘K" : "Ctrl K"}</Kbd>}
        </button>
      </SidebarHeader>
      <SidebarContent className="gap-0 px-0.5">
        {NAV_GROUPS.map((group) => (
          <SidebarGroup key={group.label ?? "main"} className="py-1">
            {group.label && <SidebarGroupLabel className="h-7">{group.label}</SidebarGroupLabel>}
            <SidebarGroupContent>
              <SidebarMenu className="gap-px">
                {group.items.map((item) => (
                  <NavItem
                    key={item.id}
                    item={item}
                    active={item.id === activeId}
                    onNavigate={closeMobile}
                  />
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        ))}
      </SidebarContent>
      <SidebarFooter className="px-2.5 pb-3">
        <SidebarMenu>
          <SidebarMenuItem className="flex items-center gap-1">
            <SidebarMenuButton
              isActive={activeId === SETTINGS_ITEM.id}
              render={
                <Link
                  to={SETTINGS_ITEM.href}
                  onClick={closeMobile}
                  aria-current={activeId === SETTINGS_ITEM.id ? "page" : undefined}
                />
              }
            >
              <SETTINGS_ITEM.icon />
              <span>{SETTINGS_ITEM.label}</span>
            </SidebarMenuButton>
            <ThemeToggle />
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  );
}

function TabBarLink({ to, icon: Icon, label, active }) {
  return (
    <Link
      to={to}
      aria-current={active ? "page" : undefined}
      className={cn(
        "flex flex-col items-center gap-0.5 rounded-md py-1 text-[10.5px] text-faint outline-none focus-visible:ring-2 focus-visible:ring-ring [&_svg]:size-5",
        active && "font-medium text-primary",
      )}
    >
      <Icon />
      {label}
    </Link>
  );
}

// MobileTabBar is the phone's bottom navigation, sticky at the end of the
// main column. "More" opens the sidebar sheet.
function MobileTabBar({ activeId, onAddTransaction }) {
  const { openMobile, setOpenMobile } = useSidebar();
  return (
    <nav
      aria-label="Primary"
      data-slot="mobile-tab-bar"
      className="sticky bottom-0 z-30 mt-auto grid grid-cols-5 items-center border-t border-border bg-sidebar px-1 pt-1.5 pb-[max(0.5rem,env(safe-area-inset-bottom))]"
    >
      <TabBarLink to="/" icon={House} label="Overview" active={activeId === "overview"} />
      <TabBarLink to="/transactions" icon={List} label="Activity" active={activeId === "transactions"} />
      <div className="flex justify-center">
        <button
          type="button"
          onClick={onAddTransaction}
          aria-label="New transaction"
          className="-mt-1 grid size-10 place-items-center rounded-full bg-primary text-primary-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 [&_svg]:size-5"
        >
          <Plus strokeWidth={2.2} />
        </button>
      </div>
      <TabBarLink to="/trends" icon={BarChart2} label="Reports" active={activeId === "reports"} />
      <button
        type="button"
        onClick={() => setOpenMobile(!openMobile)}
        aria-expanded={openMobile}
        className={cn(
          "flex flex-col items-center gap-0.5 rounded-md py-1 text-[10.5px] text-faint outline-none focus-visible:ring-2 focus-visible:ring-ring [&_svg]:size-5",
          openMobile && "font-medium text-primary",
        )}
      >
        <Menu />
        More
      </button>
    </nav>
  );
}

// DesktopReopen shows a sidebar trigger only while the desktop sidebar is
// collapsed (Ctrl/⌘+B), so it can always be brought back.
function DesktopReopen() {
  const { state, isMobile } = useSidebar();
  if (isMobile || state !== "collapsed") return null;
  return <SidebarTrigger className="fixed top-3 left-3 z-20" />;
}

export function AppShell({ children, currentPath }) {
  const [addTxnOpen, setAddTxnOpen] = useState(false);
  const [jumpOpen, setJumpOpen] = useState(false);
  const isMobile = useIsMobile();
  const navItem = navItemFor(currentPath);
  const [groupActionsEl, setGroupActionsEl] = useState(null);

  useShellShortcuts(setAddTxnOpen, setJumpOpen);

  return (
    <SidebarProvider style={{ "--sidebar-width": "13.5rem" }}>
      <AppSidebar
        activeId={navItem?.id}
        onAddTransaction={() => setAddTxnOpen(true)}
        onJump={() => setJumpOpen(true)}
      />
      <SidebarInset className="min-w-0">
        <DesktopReopen />
        <div className="container mx-auto flex max-w-7xl flex-1 flex-col gap-6 px-4 pt-5 pb-8 md:px-8 md:pt-6">
          {navItem?.tabs ? (
            <>
              <GroupHeader item={navItem} currentPath={currentPath} actionsRef={setGroupActionsEl} />
              <GroupHeaderProvider actionsEl={groupActionsEl}>
                <div className="flex-1">{children}</div>
              </GroupHeaderProvider>
            </>
          ) : (
            <div className="flex-1">{children}</div>
          )}
        </div>
        {isMobile && (
          <MobileTabBar activeId={navItem?.id} onAddTransaction={() => setAddTxnOpen(true)} />
        )}
      </SidebarInset>
      <AddTransactionModal open={addTxnOpen} onOpenChange={setAddTxnOpen} />
      <CommandPalette
        open={jumpOpen}
        onOpenChange={setJumpOpen}
        onNewTransaction={() => setAddTxnOpen(true)}
      />
      {/* On phones, keep toasts above the tab bar. */}
      <UndoToaster bottomOffset={isMobile ? "calc(env(safe-area-inset-bottom) + 4.5rem)" : undefined} />
    </SidebarProvider>
  );
}
