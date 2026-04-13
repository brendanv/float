import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { House, List, TrendingUp, Tag, PlusCircle, Menu, History, Upload, ListFilter, Palette } from "lucide-react";

const THEMES = [
  { id: "business", label: "Business" },
  { id: "light", label: "Light" },
  { id: "dark", label: "Dark" },
  { id: "emerald", label: "Emerald" },
  { id: "cyberpunk", label: "Cyberpunk" },
  { id: "nord", label: "Nord" },
  { id: "lemonade", label: "Lemonade" },
  { id: "dim", label: "Dim" },
  { id: "sunset", label: "Sunset" },
];

function ThemeSwitcher() {
  const [theme, setTheme] = useState(
    () => localStorage.getItem("float-theme") || "business"
  );

  const handleChange = (e) => {
    const next = e.target.value;
    setTheme(next);
    document.documentElement.setAttribute("data-theme", next);
    localStorage.setItem("float-theme", next);
  };

  return (
    <div class="p-3 border-t border-base-300">
      <label class="flex items-center gap-2 text-xs text-base-content/60 uppercase tracking-wide mb-2">
        <Palette size={13} />
        Theme
      </label>
      <select class="select select-sm w-full" value={theme} onChange={handleChange}>
        {THEMES.map((t) => (
          <option key={t.id} value={t.id}>{t.label}</option>
        ))}
      </select>
    </div>
  );
}

function NavLink({ href, label, icon: Icon, current }) {
  const active = current === href;
  return (
    <li>
      <Link
        to={href}
        class={active ? "active" : ""}
        onClick={() => {
          const drawer = document.getElementById("nav-drawer");
          if (drawer) drawer.checked = false;
        }}
      >
        <Icon size={18} />
        {label}
      </Link>
    </li>
  );
}

export function AppShell({ children, currentPath }) {
  return (
    <div class="drawer lg:drawer-open">
      <input id="nav-drawer" type="checkbox" class="drawer-toggle" />

      {/* Main content */}
      <div class="drawer-content flex flex-col min-h-screen bg-base-200">
        {/* Mobile top navbar */}
        <div class="navbar bg-base-100 shadow-sm lg:hidden">
          <label for="nav-drawer" class="btn btn-ghost">
            <Menu size={24} />
          </label>
          <Link to="/" class="btn btn-ghost gap-2 text-xl">
            <img src="/icon.png" alt="" class="h-8 w-8 rounded" />
            float
          </Link>
        </div>

        {/* Page content */}
        <main class="container mx-auto px-4 py-6 max-w-7xl flex-1">
          {children}
        </main>
      </div>

      {/* Sidebar */}
      <div class="drawer-side z-40">
        <label for="nav-drawer" class="drawer-overlay" />
        <aside class="bg-base-100 min-h-screen w-64 flex flex-col border-r border-base-300">
          {/* Brand */}
          <div class="p-4 border-b border-base-300">
            <Link
              to="/"
              class="flex items-center gap-3 hover:opacity-80 transition-opacity"
            >
              <img src="/icon.png" alt="" class="h-9 w-9 rounded" />
              <span class="text-xl font-semibold">float</span>
            </Link>
          </div>

          {/* Navigation */}
          <ul class="menu p-3 flex-1 gap-1">
            <NavLink href="/" label="Home" icon={House} current={currentPath} />
            <NavLink href="/transactions" label="Transactions" icon={List} current={currentPath} />
            <NavLink href="/trends" label="Trends" icon={TrendingUp} current={currentPath} />
            <NavLink href="/prices" label="Prices" icon={Tag} current={currentPath} />
            <NavLink href="/snapshots" label="Snapshots" icon={History} current={currentPath} />
            <NavLink href="/import" label="Import" icon={Upload} current={currentPath} />
            <NavLink href="/rules" label="Rules" icon={ListFilter} current={currentPath} />
            <NavLink href="/add" label="Add Transaction" icon={PlusCircle} current={currentPath} />
          </ul>

          <ThemeSwitcher />
        </aside>
      </div>
    </div>
  );
}
