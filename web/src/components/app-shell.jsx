import { useState } from "react";
import { Link, useRouterState } from "@tanstack/react-router";
import {
  House,
  List,
  TrendingUp,
  Tag,
  PlusCircle,
  History,
  Upload,
  ListFilter,
  Sun,
  Moon,
  ClockArrowUp,
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
} from "@/components/ui/sidebar";
import { AddTransactionModal } from "./add-transaction-modal.jsx";

const NAV_MAIN = [
  { href: "/", label: "Home", icon: House },
  { href: "/transactions", label: "Transactions", icon: List },
  { href: "/trends", label: "Trends", icon: TrendingUp },
  { href: "/prices", label: "Prices", icon: Tag },
  { href: "/snapshots", label: "Snapshots", icon: History },
];

const NAV_MANAGE = [
  { href: "/import", label: "Import", icon: Upload },
  { href: "/imports", label: "Import History", icon: ClockArrowUp },
  { href: "/rules", label: "Rules", icon: ListFilter },
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
                render={<Link to={item.href} />}
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
  return (
    <Sidebar variant="inset">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" render={<Link to="/" />}>
              <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                <img src="/icon.png" alt="" className="size-6 rounded" />
              </div>
              <div className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-medium">float</span>
                <span className="truncate text-xs text-muted-foreground">
                  Personal Finance
                </span>
              </div>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <NavGroup label="Overview" items={NAV_MAIN} currentPath={currentPath} />
        <NavGroup label="Manage" items={NAV_MANAGE} currentPath={currentPath} />
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
      <SidebarInset>
        <header className="flex h-12 shrink-0 items-center gap-2 px-4">
          <SidebarTrigger className="-ml-1" />
          <Separator
            orientation="vertical"
            className="mr-2 data-vertical:h-4 data-vertical:self-auto"
          />
          <span className="text-sm font-medium text-muted-foreground">
            {[...NAV_MAIN, ...NAV_MANAGE].find((i) => i.href === currentPath)?.label ?? "float"}
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
