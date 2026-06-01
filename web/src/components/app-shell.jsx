import { useState } from "react";
import { Link, useRouterState } from "@tanstack/react-router";
import {
  House,
  List,
  TrendingUp,
  BarChart2,
  Briefcase,
  Tag,
  PlusCircle,
  History,
  Upload,
  ListFilter,
  Sun,
  Moon,
  ClockArrowUp,
  BookOpen,
  Users,
  Settings,
  Terminal,
  Link2,
  Logs,
  Scale,
  LayoutGrid,
  TableProperties,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
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
import { AddTransactionModal } from "./add-transaction-modal.jsx";

const NAV_FINANCES = [
  { href: "/", label: "Home", icon: House },
  { href: "/trends", label: "Trends", icon: TrendingUp },
  { href: "/monthly", label: "Monthly", icon: BarChart2 },
  { href: "/transactions", label: "Transactions", icon: List },
  { href: "/portfolio", label: "Portfolio", icon: Briefcase },
  { href: "/bulk-entry", label: "Bulk Entry", icon: LayoutGrid },
];

const NAV_MANAGE = [
  { href: "/accounts", label: "Accounts", icon: BookOpen },
  { href: "/payees", label: "Payees", icon: Users },
  { href: "/rules", label: "Rules", icon: ListFilter },
  { href: "/templates", label: "Templates", icon: TableProperties },
];

const NAV_DATA_SOURCES = [
  { href: "/connections", label: "Connections", icon: Link2 },
  { href: "/import", label: "Import", icon: Upload },
  { href: "/prices", label: "Prices", icon: Tag },
  { href: "/assertions", label: "Assertions", icon: Scale },
];

const NAV_HISTORY = [
  { href: "/imports", label: "Import History", icon: ClockArrowUp },
  { href: "/snapshots", label: "Snapshots", icon: History },
];

const NAV_SETTINGS = [
  { href: "/settings", label: "Settings", icon: Settings },
  { href: "/hledger-query", label: "Query", icon: Terminal },
  { href: "/logs", label: "Logs", icon: Logs },
];

const ALL_NAV = [
  ...NAV_FINANCES,
  ...NAV_MANAGE,
  ...NAV_DATA_SOURCES,
  ...NAV_HISTORY,
  ...NAV_SETTINGS,
];

function ThemeSwitcher() {
  const [isDark, setIsDark] = useState(
    () => localStorage.getItem("float-theme") === "dark"
  );

  function toggle() {
    const next = !isDark;
    setIsDark(next);
    if (next) {
      document.documentElement.classList.add("dark");
      localStorage.setItem("float-theme", "dark");
    } else {
      document.documentElement.classList.remove("dark");
      localStorage.setItem("float-theme", "light");
    }
  }

  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={toggle}
      className="w-full justify-start gap-2"
    >
      {isDark ? <Moon data-icon="inline-start" /> : <Sun data-icon="inline-start" />}
      {isDark ? "Dark mode" : "Light mode"}
    </Button>
  );
}

function NavGroup({ label, items, currentPath }) {
  const { isMobile, setOpenMobile } = useSidebar();
  return (
    <SidebarGroup>
      <SidebarGroupLabel>{label}</SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu>
          {items.map((item) => (
            <SidebarMenuItem key={item.href}>
              <SidebarMenuButton
                isActive={currentPath === item.href}
                tooltip={item.label}
                render={<Link to={item.href} onClick={() => isMobile && setOpenMobile(false)} />}
              >
                <item.icon />
                <span>{item.label}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          ))}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}

function AppSidebar({ currentPath, onAddTransaction }) {
  const { isMobile, setOpenMobile } = useSidebar();
  const closeMobile = () => isMobile && setOpenMobile(false);
  return (
    <Sidebar variant="inset">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" render={<Link to="/" onClick={closeMobile} />}>
              <img src="/text_logo.png" alt="float" className="h-8 w-auto" />
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <NavGroup label="Finances" items={NAV_FINANCES} currentPath={currentPath} />
        <NavGroup label="Manage" items={NAV_MANAGE} currentPath={currentPath} />
        <NavGroup label="Data sources" items={NAV_DATA_SOURCES} currentPath={currentPath} />
        <NavGroup label="History" items={NAV_HISTORY} currentPath={currentPath} />
        <NavGroup label="Settings" items={NAV_SETTINGS} currentPath={currentPath} />
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton tooltip="Add Transaction" onClick={onAddTransaction}>
                  <PlusCircle />
                  <span>Add Transaction</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter>
        <ThemeSwitcher />
      </SidebarFooter>
    </Sidebar>
  );
}

export function AppShell({ children, currentPath }) {
  const [addTxnOpen, setAddTxnOpen] = useState(false);

  return (
    <SidebarProvider>
      <AppSidebar currentPath={currentPath} onAddTransaction={() => setAddTxnOpen(true)} />
      <SidebarInset className="min-w-0">
        <header className="flex h-12 shrink-0 items-center gap-2 px-4">
          <SidebarTrigger className="-ml-1" />
          <Separator
            orientation="vertical"
            className="mr-2 data-vertical:h-4 data-vertical:self-auto"
          />
          <span className="text-sm font-medium text-muted-foreground">
            {ALL_NAV.find((i) => i.href === currentPath)?.label ?? "float"}
          </span>
        </header>
        <div className="flex flex-1 flex-col p-4 pt-0">
          <div className="container mx-auto max-w-7xl flex-1">
            {children}
          </div>
        </div>
      </SidebarInset>
      <AddTransactionModal open={addTxnOpen} onOpenChange={setAddTxnOpen} />
    </SidebarProvider>
  );
}
