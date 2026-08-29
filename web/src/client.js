import { createClient, ConnectError, Code } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { LedgerService } from "./gen/float/v1/ledger_pb.js";
import { observeHeaders } from "./lib/generation.js";

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

// Track the server's txlock generation from every response. This is how the
// client knows which cube URL to fetch, and it costs nothing extra: any RPC the
// app already makes — including the mutation that caused the bump — carries it.
const trackGeneration = (next) => async (req) => {
  try {
    const res = await next(req);
    observeHeaders(res.header);
    return res;
  } catch (err) {
    // A failed write still bumps the generation, so read it off the error too.
    observeHeaders(err?.metadata);
    throw err;
  }
};

const transport = createConnectTransport({
  baseUrl: window.location.origin,
  interceptors: [redirectToLogin, trackGeneration],
});

export const ledgerClient = createClient(LedgerService, transport);
