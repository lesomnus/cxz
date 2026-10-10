import type { ITheme } from "@xterm/xterm";

// Keep ANSI hue families; dark-mode colors are luminous pastels. Light-mode
// variants deepen the same hues so text remains readable on a light surface.
export function terminalTheme(theme: "light" | "dark"): ITheme {
  const light = theme === "light";
  return {
    background: light ? "#fbfbfb" : "#111111",
    foreground: light ? "#202020" : "#ededed",
    cursor: light ? "#202020" : "#ededed",
    selectionBackground: light ? "#cccccc" : "#444444",
    black: "#111111",
    red: light ? "#b5284f" : "#ff879f",
    green: light ? "#237a43" : "#9df5a7",
    yellow: light ? "#88600b" : "#ffe38a",
    blue: light ? "#365dc5" : "#8fb8ff",
    magenta: light ? "#8b3cb6" : "#d6a0ff",
    cyan: light ? "#087e89" : "#88edf0",
    white: light ? "#666666" : "#ededed",
    brightBlack: light ? "#777777" : "#9696a3",
    brightRed: light ? "#a61b44" : "#ffacbd",
    brightGreen: light ? "#126b33" : "#b8ffd0",
    brightYellow: light ? "#795000" : "#fff1b1",
    brightBlue: light ? "#244db5" : "#b0ceff",
    brightMagenta: light ? "#7b2ba8" : "#e7c0ff",
    brightCyan: light ? "#006d78" : "#b1ffff",
    brightWhite: light ? "#333333" : "#ffffff",
  };
}
