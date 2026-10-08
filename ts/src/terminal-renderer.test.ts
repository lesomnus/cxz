import { describe, expect, it, vi } from "vitest";
import { enableTerminalWebGL } from "./terminal-renderer";

function fixture() {
  let lost: (() => void) | undefined;
  const unsubscribe = vi.fn(() => {
    lost = undefined;
  });
  const dispose = vi.fn();
  const constructed = vi.fn();
  class Addon {
    constructor() {
      constructed();
    }
    dispose = dispose;
    onContextLoss = (callback: () => void) => {
      lost = callback;
      return { dispose: unsubscribe };
    };
  }
  const module = {
    WebglAddon: Addon,
  } as unknown as typeof import("@xterm/addon-webgl");
  const terminal = { loadAddon: vi.fn() };
  return {
    module,
    terminal,
    dispose,
    unsubscribe,
    constructed,
    lose: () => lost?.(),
  };
}

describe("optional terminal WebGL renderer", () => {
  it("restores DOM once on context loss and removes its listener", async () => {
    const f = fixture();
    const cleanup = enableTerminalWebGL(f.terminal, async () => f.module);
    await vi.waitFor(() => expect(f.terminal.loadAddon).toHaveBeenCalledOnce());
    f.lose();
    expect(f.dispose).toHaveBeenCalledOnce();
    expect(f.unsubscribe).toHaveBeenCalledOnce();
    f.lose();
    cleanup();
    expect(f.dispose).toHaveBeenCalledOnce();
  });

  it("removes partially activated addons when WebGL is unavailable", async () => {
    const f = fixture();
    f.terminal.loadAddon.mockImplementation(() => {
      throw new Error("No WebGL2");
    });
    const cleanup = enableTerminalWebGL(f.terminal, async () => f.module);
    await vi.waitFor(() => expect(f.dispose).toHaveBeenCalledOnce());
    expect(f.unsubscribe).toHaveBeenCalledOnce();
    cleanup();
    expect(f.dispose).toHaveBeenCalledOnce();
  });

  it("keeps DOM when the optional addon cannot be downloaded", async () => {
    const f = fixture();
    const load = vi.fn().mockRejectedValue(new Error("offline"));
    const cleanup = enableTerminalWebGL(f.terminal, load);
    await load.mock.results[0].value.catch(() => {});
    cleanup();
    expect(f.terminal.loadAddon).not.toHaveBeenCalled();
    expect(f.constructed).not.toHaveBeenCalled();
  });

  it("does not attach a delayed addon after the terminal closes", async () => {
    const f = fixture();
    let resolve!: (module: typeof f.module) => void;
    const pending = new Promise<typeof f.module>((done) => {
      resolve = done;
    });
    const cleanup = enableTerminalWebGL(f.terminal, () => pending);
    cleanup();
    resolve(f.module);
    await pending;
    expect(f.constructed).not.toHaveBeenCalled();
    expect(f.terminal.loadAddon).not.toHaveBeenCalled();
  });

  it("disposes an active renderer on unmount without waiting for loss", async () => {
    const f = fixture();
    const cleanup = enableTerminalWebGL(f.terminal, async () => f.module);
    await vi.waitFor(() => expect(f.terminal.loadAddon).toHaveBeenCalledOnce());
    cleanup();
    expect(f.unsubscribe).toHaveBeenCalledOnce();
    expect(f.dispose).toHaveBeenCalledOnce();
  });
});
