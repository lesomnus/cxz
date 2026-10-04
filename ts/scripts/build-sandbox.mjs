import { execFileSync } from "node:child_process";
import { mkdirSync, copyFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
const root = fileURLToPath(new URL("../../", import.meta.url));
const out = join(root, "ts", ".sandbox");
mkdirSync(out, { recursive: true });
execFileSync(
  "go",
  [
    "build",
    "-tags",
    "grpcnotrace",
    "-trimpath",
    "-ldflags=-s -w",
    "-o",
    join(out, "app.wasm"),
    "./cmd/cxz-sandbox",
  ],
  {
    cwd: root,
    env: { ...process.env, GOOS: "js", GOARCH: "wasm", CGO_ENABLED: "0" },
    stdio: "inherit",
  },
);
const goroot = execFileSync("go", ["env", "GOROOT"], {
  cwd: root,
  encoding: "utf8",
}).trim();
copyFileSync(
  join(goroot, "lib", "wasm", "wasm_exec.js"),
  join(out, "wasm_exec.js"),
);
console.log("Sandbox WASM ready. Fake services only; no accounts or Docker.");
