import { lazy } from "react";
import {
  createHashHistory,
  createRouter,
  createRoute,
  createRootRoute,
  Outlet,
  useRouterState,
} from "@tanstack/react-router";
import { TooltipProvider } from "@/components/ui/tooltip";
import { AppShell } from "./components/app-shell.jsx";
import { HomePage } from "./pages/home.jsx";
import { TransactionsPage } from "./pages/transactions.jsx";
import { AddTransactionPage } from "./pages/add-transaction.jsx";
import { PricesPage } from "./pages/prices.jsx";
import { SnapshotsPage } from "./pages/snapshots.jsx";
import { ImportPage } from "./pages/import.jsx";
import { RulesPage } from "./pages/rules.jsx";
import { ImportsHistoryPage } from "./pages/imports-history.jsx";

const LazyTrendsPage = lazy(() =>
  import("./pages/trends.jsx").then((m) => ({ default: m.TrendsPage }))
);

const rootRoute = createRootRoute({
  component: function Root() {
    const { location } = useRouterState();
    return (
      <TooltipProvider>
        <AppShell currentPath={location.pathname}>
          <Outlet />
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
  component: LazyTrendsPage,
});

const pricesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/prices",
  component: PricesPage,
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

const routeTree = rootRoute.addChildren([
  indexRoute,
  transactionsRoute,
  addRoute,
  trendsRoute,
  pricesRoute,
  snapshotsRoute,
  importRoute,
  rulesRoute,
  importsHistoryRoute,
]);

export const router = createRouter({
  routeTree,
  history: createHashHistory(),
});
