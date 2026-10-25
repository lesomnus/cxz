import { defaultClientConditions, defaultServerConditions } from "vite";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";

function clientRevision() {
  try {
    return execFileSync("git", ["rev-parse", "--short=12", "HEAD"], {
      cwd: fileURLToPath(new URL("../..", import.meta.url)),
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();
  } catch {
    return "unknown";
  }
}

// Development reads the submodule's TSX/CSS directly. Production uses the
// package's compiled public entries at the commit pinned by cxz.
export function noxConfig(command) {
  return {
    define: { __CXZ_CLIENT_REVISION__: JSON.stringify(clientRevision()) },
    resolve: {
      conditions: [
        ...(command === "serve" ? ["nox-source"] : []),
        ...defaultClientConditions,
      ],
      dedupe: ["react", "react-dom", "monaco-editor"],
    },
    optimizeDeps: {
      exclude: [
        "@lesomnus/nox",
        "@lesomnus/nox/editor",
        "@lesomnus/nox/terminal",
      ],
      // Crawl the editor's own source for dependencies at startup. Excluding
      // the nox entries stops the scan at them, so monaco -- which only the
      // editor's lazy `import("./code-editor")` reaches -- was discovered when
      // a person opened a file. Discovering a dependency then re-optimizes,
      // which changes every pre-bundled chunk's hash, and the imports already
      // in flight answered 504 Outdated Optimize Dep: opening a file in the
      // explorer failed with "Failed to fetch dynamically imported module" and
      // recovered only on the reload vite asks for afterwards.
      //
      // Entries replace discovery rather than adding to it, so the html roots
      // are named too. Reading the list from the editor's source means a new
      // monaco language in nox needs nothing here. See cxz#232.
      ...(command === "serve"
        ? { entries: ["*.html", "packages/nox/src/editor/code-editor.ts"] }
        : {}),
    },
    ...(command === "serve"
      ? {
          ssr: {
            noExternal: ["@lesomnus/nox"],
            resolve: { conditions: ["nox-source", ...defaultServerConditions] },
          },
        }
      : {}),
  };
}
