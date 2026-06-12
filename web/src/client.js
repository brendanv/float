import { createClient, ConnectError, Code } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { LedgerService } from "./gen/float/v1/ledger_pb.js";

// Redirect to the login page whenever the server rejects a call as
// unauthenticated. The session cookie set by /api/login rides along on
// same-origin requests, so no header handling is needed here.
const redirectToLogin = (next) => async (req) => {
  try {
    return await next(req);
  } catch (err) {
    if (
      err instanceof ConnectError &&
      err.code === Code.Unauthenticated &&
      !window.location.hash.startsWith("#/login")
    ) {
      window.location.hash = "#/login";
    }
    throw err;
  }
};

const transport = createConnectTransport({
  baseUrl: window.location.origin,
  interceptors: [redirectToLogin],
});

export const ledgerClient = createClient(LedgerService, transport);
