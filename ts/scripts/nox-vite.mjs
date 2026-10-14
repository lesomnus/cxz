import { defaultClientConditions, defaultServerConditions } from "vite";

// Development reads the submodule's TSX/CSS directly. Production uses the
// package's compiled public entries at the commit pinned by cxz.
export function noxConfig(command) {
  return {
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
