import { mkdtemp, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createServer } from "vite";
import { devGatewayPlugin } from "../scripts/web-dev.mjs";

const directory = await mkdtemp(join(tmpdir(), "cxz-vite-browser-"));
const tokenFile = join(directory, "token");
await writeFile(tokenFile, "a".repeat(32));
const server = await createServer({
  configFile: false,
  plugins: [devGatewayPlugin({ origin: "http://127.0.0.1:18082", tokenFile })],
  server: { host: "127.0.0.1", port: 5174, strictPort: true },
});
await server.listen();
let stopping = false;
async function stop() {
  if (stopping) return;
  stopping = true;
  await server.close();
  await rm(directory, { recursive: true, force: true });
  process.exit(0);
}
process.once("SIGTERM", stop);
process.once("SIGINT", stop);
