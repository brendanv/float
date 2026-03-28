import { navigate } from "../router.jsx";

const manageRoutes = ["/prices"];

function NavLink({ href, label, current }) {
  const active = current === href;
  return (
    <li>
      <a
        href={"#" + href}
        class={active ? "active" : ""}
        onClick={(e) => {
          e.preventDefault();
          navigate(href);
        }}
      >
        {label}
      </a>
    </li>
  );
}

function ManageDropdown({ current }) {
  const active = manageRoutes.includes(current);
  return (
    <li>
      <details>
        <summary class={active ? "active" : ""}>Manage</summary>
        <ul class="bg-base-100 rounded-box z-10 w-36 shadow">
          <NavLink href="/prices" label="Prices" current={current} />
        </ul>
      </details>
    </li>
  );
}

export function AppShell({ children, currentPath }) {
  return (
    <div class="min-h-screen bg-base-200">
      <div class="navbar bg-base-100 shadow-sm">
        <div class="navbar-start">
          <a
            href="#/"
            class="btn btn-ghost gap-2 text-xl"
            onClick={(e) => { e.preventDefault(); navigate("/"); }}
          >
            <img src="/icon.png" alt="" class="h-8 w-8 rounded" />
            float
          </a>
        </div>
        <div class="navbar-end">
          <ul class="menu menu-horizontal px-1">
            <NavLink href="/" label="Home" current={currentPath} />
            <NavLink href="/transactions" label="Transactions" current={currentPath} />
            <NavLink href="/trends" label="Trends" current={currentPath} />
            <ManageDropdown current={currentPath} />
            <NavLink href="/add" label="Add" current={currentPath} />
          </ul>
        </div>
      </div>
      <main class="container mx-auto px-4 py-6 max-w-7xl">
        {children}
      </main>
    </div>
  );
}
