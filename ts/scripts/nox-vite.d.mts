import type { ServerOptions, UserConfig } from "vite";
export const noxDevServer: Pick<ServerOptions, "warmup">;
export function noxConfig(command: "serve" | "build"): UserConfig;
