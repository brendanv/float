import { useState } from "react";
import { Link } from "@tanstack/react-router";
import {
  House,
  List,
  TrendingUp,
  Tag,
  PlusCircle,
  Menu,
  History,
  Upload,
  ListFilter,
  Sun,
  Moon,
  ClockArrowUp,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetTrigger,
  SheetTitle,
} from "@/components/ui/sheet";
import { cn } from "@/lib/utils";

const NAV_ITEMS = [
  { href: "/", label: "Home", icon: House },
  { href: "/transactions", label: "Transactions", icon: List },
  { href: "/trends", label: "Trends", icon: TrendingUp },
  { href: "/prices", label: "Prices", icon: Tag },
  { href: "/snapshots", label: "Snapshots", icon: History },
  { href: "/import", label: "Import", icon: Upload },
  { href: "/imports", label: "Import History", icon: ClockArrowUp },
  { href: "/rules", label: "Rules", icon: ListFilter },
  { href: "/add", label: "Add Transaction", icon: PlusCircle },
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

function NavLink({ href, label, icon: Icon, current, onClick }) {
  const active = current === href;
  return (
    <Link
      to={href}
      onClick={onClick}
      className={cn(
        "flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors",
        active
          ? "bg-sidebar-accent text-sidebar-accent-foreground"
          : "text-sidebar-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground"
      )}
    >
      <Icon size={18} />
      {label}
    </Link>
  );
}

function SidebarContent({ currentPath, onNavigate }) {
  return (
    <div className="flex h-full flex-col bg-sidebar">
      {/* Brand */}
      <div className="border-b border-sidebar-border p-4">
        <Link
          to="/"
          onClick={onNavigate}
          className="flex items-center gap-3 transition-opacity hover:opacity-80"
        >
          <img src="/icon.png" alt="" className="size-9 rounded" />
          <span className="text-xl font-semibold text-sidebar-foreground">
            float
          </span>
        </Link>
      </div>

      {/* Navigation */}
      <nav className="flex flex-1 flex-col gap-1 p-3">
        {NAV_ITEMS.map((item) => (
          <NavLink
            key={item.href}
            href={item.href}
            label={item.label}
            icon={item.icon}
            current={currentPath}
            onClick={onNavigate}
          />
        ))}
      </nav>

      {/* Theme switcher */}
      <div className="border-t border-sidebar-border p-3">
        <ThemeSwitcher />
      </div>
    </div>
  );
}

export function AppShell({ children, currentPath }) {
  const [mobileOpen, setMobileOpen] = useState(false);

  return (
    <div className="flex min-h-screen bg-background">
      {/* Desktop sidebar */}
      <aside className="hidden w-64 shrink-0 border-r border-sidebar-border lg:block">
        <div className="sticky top-0 h-screen">
          <SidebarContent currentPath={currentPath} />
        </div>
      </aside>

      {/* Main content area */}
      <div className="flex min-w-0 flex-1 flex-col">
        {/* Mobile top navbar */}
        <header className="flex items-center gap-2 border-b border-border bg-background p-3 lg:hidden">
          <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
            <SheetTrigger
              render={
                <Button variant="ghost" size="icon">
                  <Menu />
                </Button>
              }
            />
            <SheetContent side="left" className="w-64 p-0">
              <SheetTitle className="sr-only">Navigation</SheetTitle>
              <SidebarContent
                currentPath={currentPath}
                onNavigate={() => setMobileOpen(false)}
              />
            </SheetContent>
          </Sheet>
          <Link to="/" className="flex items-center gap-2 text-xl font-semibold">
            <img src="/icon.png" alt="" className="size-8 rounded" />
            float
          </Link>
        </header>

        {/* Page content */}
        <main className="container mx-auto max-w-7xl flex-1 px-4 py-6">
          {children}
        </main>
      </div>
    </div>
  );
}
