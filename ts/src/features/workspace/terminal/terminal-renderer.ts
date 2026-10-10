import type { IDisposable, Terminal } from "@xterm/xterm";
import type { WebglAddon } from "@xterm/addon-webgl";

// Rendering is optional: neither a missing GPU nor a failed chunk download
// should prevent the shell from connecting. Disposing WebglAddon restores
// xterm's DOM renderer while preserving the terminal buffer and selection.
export function enableTerminalWebGL(
  terminal: Pick<Terminal, "loadAddon">,
  load: () => Promise<{ WebglAddon: typeof WebglAddon }> = () =>
    import("@xterm/addon-webgl"),
): () => void {
  let disposed = false;
  let addon: WebglAddon | undefined;
  let contextLoss: IDisposable | undefined;
  const release = () => {
    contextLoss?.dispose();
    contextLoss = undefined;
    const current = addon;
    addon = undefined;
    current?.dispose();
  };
  void (async () => {
    try {
      const { WebglAddon } = await load();
      if (disposed) return;
      addon = new WebglAddon();
      contextLoss = addon.onContextLoss(release);
      terminal.loadAddon(addon);
    } catch {
      release();
    }
  })();
  return () => {
    disposed = true;
    release();
  };
}
