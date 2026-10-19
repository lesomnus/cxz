import { mkdtemp, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createServer } from "vite";
import { devGatewayPlugin } from "../scripts/web-dev.mjs";
import { noxConfig } from "../scripts/nox-vite.mjs";

// The dev server as a person runs it, which means the nox submodule resolved as
// source: `npm run dev:app` applies noxConfig and this did not, so what little
// of dev was tested here ran against the package's built entries instead. The
// one thing only dev does was therefore the one thing nothing checked, and it
// broke -- see cxz#232.
const directory = await mkdtemp(join(tmpdir(), "cxz-vite-browser-"));
const tokenFile = join(directory, "token");
await writeFile(tokenFile, "a".repeat(32));
const server = await createServer({
  configFile: false,
  ...noxConfig("serve"),
  // Its own pre-bundled dependencies, because the default is shared with the
  // dev server a person is running: a different config hashes differently, so
  // starting this one re-optimized that one's dependencies out from under it,
  // and the open page then answered 504 for every chunk it already had. Running
  // the tests broke the browser window next to them.
  cacheDir: join(directory, "vite"),
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
