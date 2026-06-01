import { lazy, Suspense } from "react";
import {
  createHashHistory,
  createRouter,
  createRoute,
  createRootRoute,
  Outlet,
  useRouterState,
} from "@tanstack/react-router";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Loading } from "@/components/loading";
import { AppShell } from "./components/app-shell.jsx";

function lazyPage(loader, exportName) {
  return lazy(() => loader().then((m) => ({ default: m[exportName] })));
}

const HomePage = lazyPage(() => import("./pages/home.jsx"), "HomePage");
const TransactionsPage = lazyPage(() => import("./pages/transactions.jsx"), "TransactionsPage");
const AddTransactionPage = lazyPage(() => import("./pages/add-transaction.jsx"), "AddTransactionPage");
const TrendsPage = lazyPage(() => import("./pages/trends.jsx"), "TrendsPage");
const MonthlyDashboardPage = lazyPage(
  () => import("./pages/monthly-dashboard.jsx"),
  "MonthlyDashboardPage"
);
const PricesPage = lazyPage(() => import("./pages/prices.jsx"), "PricesPage");
const AccountsPage = lazyPage(() => import("./pages/accounts.jsx"), "AccountsPage");
const BalanceAssertionsPage = lazyPage(
  () => import("./pages/balance-assertions.jsx"),
  "BalanceAssertionsPage"
);
const SnapshotsPage = lazyPage(() => import("./pages/snapshots.jsx"), "SnapshotsPage");
const ImportPage = lazyPage(() => import("./pages/import.jsx"), "ImportPage");
const RulesPage = lazyPage(() => import("./pages/rules.jsx"), "RulesPage");
const ImportsHistoryPage = lazyPage(() => import("./pages/imports-history.jsx"), "ImportsHistoryPage");
const PayeesPage = lazyPage(() => import("./pages/payees.jsx"), "PayeesPage");
const PortfolioPage = lazyPage(() => import("./pages/portfolio.jsx"), "PortfolioPage");
const SettingsPage = lazyPage(() => import("./pages/settings.jsx"), "SettingsPage");
const HledgerQueryPage = lazyPage(() => import("./pages/hledger-query.jsx"), "HledgerQueryPage");
const ConnectionsPage = lazyPage(() => import("./pages/connections.jsx"), "ConnectionsPage");
const LogsPage = lazyPage(() => import("./pages/logs.jsx"), "LogsPage");
const TemplatesPage = lazyPage(() => import("./pages/templates.jsx"), "TemplatesPage");
const BulkEntryPage = lazyPage(() => import("./pages/bulk-entry.jsx"), "BulkEntryPage");

const rootRoute = createRootRoute({
  component: function Root() {
    const { location } = useRouterState();
    return (
      <TooltipProvider>
        <AppShell currentPath={location.pathname}>
          <Suspense fallback={<Loading />}>
            <Outlet />
          </Suspense>
        </AppShell>
      </TooltipProvider>
    );
  },
  notFoundComponent: () => (
    <p>
      Page not found. <a href="#/">Go home</a>
    </p>
  ),
});

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: HomePage,
});

export const transactionsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/transactions",
  validateSearch: (search) => ({
    account: search.account ?? "",
    payee: search.payee ?? "",
    importBatchId: search.importBatchId ?? "",
    search: search.search ?? "",
  }),
  component: TransactionsPage,
});

const addRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/add",
  component: AddTransactionPage,
});

const trendsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/trends",
  component: TrendsPage,
});

const monthlyDashboardRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/monthly",
  component: MonthlyDashboardPage,
});

const pricesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/prices",
  component: PricesPage,
});

const accountsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/accounts",
  component: AccountsPage,
});

const balanceAssertionsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/assertions",
  component: BalanceAssertionsPage,
});

const snapshotsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/snapshots",
  component: SnapshotsPage,
});

const importRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/import",
  component: ImportPage,
});

const rulesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/rules",
  component: RulesPage,
});

const importsHistoryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/imports",
  component: ImportsHistoryPage,
});

const payeesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/payees",
  component: PayeesPage,
});

const portfolioRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/portfolio",
  component: PortfolioPage,
});

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsPage,
});

const hledgerQueryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/hledger-query",
  component: HledgerQueryPage,
});

const connectionsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/connections",
  component: ConnectionsPage,
});

const logsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/logs",
  component: LogsPage,
});

const templatesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/templates",
  component: TemplatesPage,
});

const bulkEntryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/bulk-entry",
  validateSearch: (search) => ({
    templateId: search.templateId ?? "",
  }),
  component: BulkEntryPage,
});

const routeTree = rootRoute.addChildren([
  indexRoute,
  transactionsRoute,
  addRoute,
  trendsRoute,
  monthlyDashboardRoute,
  pricesRoute,
  accountsRoute,
  balanceAssertionsRoute,
  snapshotsRoute,
  importRoute,
  rulesRoute,
  importsHistoryRoute,
  payeesRoute,
  portfolioRoute,
  settingsRoute,
  hledgerQueryRoute,
  connectionsRoute,
  logsRoute,
  templatesRoute,
  bulkEntryRoute,
]);

export const router = createRouter({
  routeTree,
  history: createHashHistory(),
});
