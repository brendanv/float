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
          {/* Mobile hamburger menu */}
          <div class="dropdown dropdown-end sm:hidden">
            <label tabIndex={0} class="btn btn-circle swap swap-rotate">
              <input type="checkbox" />
              <svg class="swap-off fill-current" xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 512 512">
                <path d="M64,384H448V341.33H64Zm0-106.67H448V234.67H64ZM64,128v42.67H448V128Z" />
              </svg>
              <svg class="swap-on fill-current" xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 512 512">
                <polygon points="400 145.49 366.51 112 256 222.51 145.49 112 112 145.49 222.51 256 112 366.51 145.49 400 256 289.49 366.51 400 400 366.51 289.49 256 400 145.49" />
              </svg>
            </label>
            <ul
              tabIndex={0}
              class="menu menu-sm dropdown-content bg-base-100 rounded-box z-10 mt-3 w-52 shadow"
            >
              <NavLink href="/" label="Home" current={currentPath} />
              <NavLink href="/transactions" label="Transactions" current={currentPath} />
              <NavLink href="/trends" label="Trends" current={currentPath} />
              <ManageDropdown current={currentPath} />
              <NavLink href="/add" label="Add" current={currentPath} />
            </ul>
          </div>
          {/* Desktop menu */}
          <ul class="menu menu-horizontal px-1 hidden sm:flex">
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
