import { House, List, TrendingUp, Tag, PlusCircle, Menu } from "lucide-preact";
import { navigate } from "../router.jsx";

function NavLink({ href, label, icon: Icon, current }) {
  const active = current === href;
  return (
    <li>
      <a
        href={"#" + href}
        class={active ? "active" : ""}
        onClick={(e) => {
          e.preventDefault();
          navigate(href);
          // Close drawer on mobile after navigation
          const drawer = document.getElementById("nav-drawer");
          if (drawer) drawer.checked = false;
        }}
      >
        <Icon size={18} />
        {label}
      </a>
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
          <a
            href="#/"
            class="btn btn-ghost gap-2 text-xl"
            onClick={(e) => { e.preventDefault(); navigate("/"); }}
          >
            <img src="/icon.png" alt="" class="h-8 w-8 rounded" />
            float
          </a>
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
            <a
              href="#/"
              class="flex items-center gap-3 hover:opacity-80 transition-opacity"
              onClick={(e) => { e.preventDefault(); navigate("/"); }}
            >
              <img src="/icon.png" alt="" class="h-9 w-9 rounded" />
              <span class="text-xl font-semibold">float</span>
            </a>
          </div>

          {/* Navigation */}
          <ul class="menu p-3 flex-1 gap-1">
            <NavLink href="/" label="Home" icon={House} current={currentPath} />
            <NavLink href="/transactions" label="Transactions" icon={List} current={currentPath} />
            <NavLink href="/trends" label="Trends" icon={TrendingUp} current={currentPath} />
            <NavLink href="/prices" label="Prices" icon={Tag} current={currentPath} />
            <NavLink href="/add" label="Add Transaction" icon={PlusCircle} current={currentPath} />
          </ul>
        </aside>
      </div>
    </div>
  );
}
