// Builds the production web UI and a floatd binary that embeds it, once per
// run, so the suite exercises exactly what ships. Set FLOAT_E2E_BIN to an
// existing floatd binary (built with -tags webui) to skip the build.
import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const webDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repoRoot = path.resolve(webDir, "..");
export const defaultBin = path.join(webDir, "e2e", ".bin", "floatd");

export default function globalSetup() {
  if (process.env.FLOAT_E2E_BIN) {
    if (!existsSync(process.env.FLOAT_E2E_BIN)) {
      throw new Error(`FLOAT_E2E_BIN=${process.env.FLOAT_E2E_BIN} does not exist`);
    }
    return;
  }
  const run = (cmd, args, cwd) => execFileSync(cmd, args, { cwd, stdio: "inherit" });
  console.log("e2e: building web UI…");
  run("bun", ["run", "build"], webDir);
  console.log("e2e: building floatd…");
  run("go", ["build", "-tags", "webui", "-o", defaultBin, "./cmd/floatd"], repoRoot);
}
