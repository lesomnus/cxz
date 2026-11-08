type ShapeStyle = {
  fill?: "none" | "currentColor";
  stroke?: "none" | "currentColor";
  strokeWidth?: number;
  strokeDasharray?: string;
  fillRule?: "evenodd";
  part?: string;
};

const path = (d: string, style: ShapeStyle = {}) => ({
  element: "path" as const,
  d,
  ...style,
});
const circle = (cx: number, cy: number, r: number, style: ShapeStyle = {}) => ({
  element: "circle" as const,
  cx,
  cy,
  r,
  ...style,
});
const rect = (
  x: number,
  y: number,
  width: number,
  height: number,
  rx = 0,
  style: ShapeStyle = {},
) => ({ element: "rect" as const, x, y, width, height, rx, ...style });

export type IconShape = ReturnType<typeof path | typeof circle | typeof rect>;
export type IconDefinition = {
  viewBox: string;
  solid?: boolean;
  shapes: readonly IconShape[];
};
export const ICON_STROKE_WIDTH = 1.5;
const line = (shapes: IconShape[], viewBox = "0 0 24 24"): IconDefinition => ({
  viewBox,
  shapes,
});
const solid = (shapes: IconShape[], viewBox = "0 0 16 16"): IconDefinition => ({
  viewBox,
  solid: true,
  shapes,
});
const filled = { fill: "currentColor", stroke: "none" } as const;

const errorTriangle = solid([path("M8 3 14 13H2Z")]);
const play = solid([path("m5 3 7 5-7 5Z")]);
const stop = solid([rect(4, 4, 8, 8, 1)]);
const pause = solid([rect(4.75, 4, 2.5, 8, 0.5), rect(8.75, 4, 2.5, 8, 0.5)]);
const refresh = line([path("M13 7a5 5 0 1 0-.8 3.7M13 3v4H9")], "0 0 16 16");
const search = line([circle(10, 10, 6), path("m15 15 5 5")]);
const sparkle = solid(
  [path("m12 3 2.5 6.5L21 12l-6.5 2.5L12 21l-2.5-6.5L3 12l6.5-2.5Z")],
  "0 0 24 24",
);
const stopped = line(
  [
    path(
      "M2.83 9.88a5.5 5.5 0 0 1 7.05-7.05m3.29 3.29a5.5 5.5 0 0 1-7.05 7.05M2.5 13.5l11-11",
    ),
  ],
  "0 0 16 16",
);
const question = "M6.5 6.25a1.5 1.5 0 1 1 2 1.4c-.5.2-.5.6-.5 1.1M8 11h.01";
const ring = circle(8, 8, 5.5);
const circlePause = line([ring, path("M6.5 5.75v4.5m3-4.5v4.5")], "0 0 16 16");
const unknown = line([ring, path(question)], "0 0 16 16");
const check = line([path("m3 8 3.2 3.2L13 4.5")], "0 0 16 16");
const close = line([path("m4 4 8 8M12 4l-8 8")], "0 0 16 16");
const priorityHighest = solid([rect(4, 4, 8, 8)]);
const priorityHigh = solid([path("M8 3 13 12H3Z")]);
const priorityNormal = solid([circle(8, 8, 3)]);
const priorityLow = line([path("m4.5 11.5 7-7")], "0 0 16 16");
const priorityLowest = solid([circle(8, 8, 1.5)]);
const document = line([path("M14 3H6v18h12V7zM14 3v5h4M9 12h6M9 16h6")]);
const info = line([
  circle(12, 12, 8),
  path("M12 11v6"),
  circle(12, 7.5, 0.75, filled),
]);
const duration = line([
  path(
    "M9 2h6M12 2v3M17 5l2-2M12 9v4l3 2 M20 13a8 8 0 1 1-16 0 8 8 0 0 1 16 0",
  ),
]);

export const iconDefinitions = {
  start: play,
  resume: play,
  stop,
  pause,
  refresh,
  restart: refresh,
  search,
  add: line([path("M12 5v14M5 12h14")]),
  close,
  deny: close,
  check,
  save: line([
    path("M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h12l4 4v12a2 2 0 0 1-2 2Z"),
    path("M7 3v6h10V3M7 21v-8h10v8"),
  ]),
  undo: line([path("M9 4 4 9l5 5M4 9h10a6 6 0 0 1 0 12h-2")]),
  login: line([
    path("M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4"),
    path("m10 17 5-5-5-5M15 12H3"),
  ]),
  allow: check,
  copy: line([
    rect(8, 8, 12, 12, 2),
    path("M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"),
  ]),
  remove: line(
    [path("M3 6h10v6a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1ZM6 3h4M6 9h4")],
    "0 0 16 16",
  ),
  erase: line(
    [path("M3 4h10M6 4V2h4v2M4 4l.7 9h6.6l.7-9M6.5 6.5v4M9.5 6.5v4")],
    "0 0 16 16",
  ),
  fork: line(
    [
      path("M5 11V5m0 4h3a3 3 0 0 0 3-3V5"),
      circle(5, 3, 1.5, filled),
      circle(11, 3, 1.5, filled),
      circle(5, 13, 1.5, filled),
    ],
    "0 0 16 16",
  ),
  info,
  warning: solid(
    [
      path(
        "M12 3 22 21H2ZM11.25 9v5.75h1.5V9ZM12 16.25a.85.85 0 1 0 0 1.7.85.85 0 1 0 0-1.7Z",
        { fillRule: "evenodd" },
      ),
    ],
    "0 0 24 24",
  ),
  "error-active": errorTriangle,
  "error-stale": errorTriangle,
  "error-dismissed": solid([rect(3, 3, 10, 10)]),
  "error-resolved": solid([circle(8, 8, 3.5)]),
  details: info,
  terminal: line([path("m3 4 4 4-4 4m6 0h4")], "0 0 16 16"),
  upload: line([
    path("M12 15V3m-4 4 4-4 4 4M4 15v5a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-5"),
  ]),
  folder: line([path("M2 4h5l1.5 2H14v7H2ZM8 11V7m-2 2 2-2 2 2")], "0 0 16 16"),
  "open-external": line([path("M7 17 17 7M7 7h10v10")]),
  "open-project": line([path("M5 12h14m-6-6 6 6-6 6")]),
  "previous-task": line([path("m6 15 6-6 6 6")]),
  "next-task": line([path("m6 9 6 6 6-6")]),
  "open-details": line([path("M9 4H4v5m11-5h5v5M4 15v5h5m11-5v5h-5")]),
  "run-search": line([path("M5 12h14m-5-5 5 5-5 5")]),
  "sign-out": line([path("M10 4H4v16h6M8 12h13m-5-5 5 5-5 5")]),
  "collapse-projects": line([path("m7 9 5-5 5 5M7 15l5 5 5-5")]),
  "expand-projects": line([path("m7 4 5 5 5-5M7 20l5-5 5 5")]),
  show: line([
    path("M3 12s3.5-7 9-7 9 7 9 7-3.5 7-9 7-9-7-9-7Z"),
    circle(12, 12, 3),
  ]),
  hide: line([
    path(
      "M3 3l18 18M10.6 10.6a2 2 0 0 0 2.8 2.8M9.9 5.2A12 12 0 0 1 12 5c5.5 0 9 7 9 7a18 18 0 0 1-3.2 4.1M6.2 6.2A18 18 0 0 0 3 12s3.5 7 9 7a10 10 0 0 0 4.1-.9",
    ),
  ]),
  filter: line([path("M4 6h16M7 12h10M10 18h4")]),
  sync: line([
    path(
      "M20 7v5h-5M4 17v-5h5M19 12a7 7 0 0 0-12-5L4 10m1 2a7 7 0 0 0 12 5l3-3",
    ),
  ]),
  document,
  image: line([
    rect(3, 3, 18, 18, 2),
    circle(8.5, 8.5, 1.5, filled),
    path("m21 15-5-5L5 21"),
  ]),
  "paste-text": line([path("M4 8V5h16v3M12 5v14M8 19h8")]),
  "paste-file": line([
    path("M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9ZM14 3v6h6"),
  ]),
  "send-message": line([path("M12 19V5m-6 6 6-6 6 6")]),
  dice: line([
    rect(3, 3, 18, 18, 4),
    circle(8, 8, 1, filled),
    circle(12, 12, 1, filled),
    circle(16, 16, 1, filled),
  ]),
  dot: solid([circle(8, 8, 1.5)]),
  "latest-message": line([path("M12 5v14m-6-6 6 6 6-6")]),
  microphone: line([
    rect(9, 2, 6, 12, 3),
    path("M5 10v2a7 7 0 0 0 14 0v-2M12 19v3m-4 0h8"),
  ]),
  model: line([
    rect(4, 7, 16, 13, 3),
    path("M12 7V3M2 12v4m20-4v4M9 17h6"),
    circle(8.5, 12, 1, filled),
    circle(15.5, 12, 1, filled),
  ]),
  effort: line([path("M4 18a9 9 0 1 1 16 0M12 14l4-6")]),
  expand: line([path("M6 2H2v4M10 2h4v4M14 10v4h-4M6 14H2v-4")], "0 0 16 16"),
  collapse: line([path("M2 6h4V2M10 2v4h4M14 10h-4v4M6 14v-4H2")], "0 0 16 16"),
  "expand-code": line([path("m8 8-4 4 4 4m8-8 4 4-4 4M4 12h16")]),
  "collapse-code": line([path("m4 8 4 4-4 4m16-8-4 4 4 4M2 12h6m8 0h6")]),
  "indent-tab": line([path("M3 10h11m-4-4 4 4-4 4m7-10v12")], "0 0 20 20"),
  "indent-spaces": line(
    [
      path("M3 5v10m14-10v10"),
      ...[6, 10, 14].map((x) => circle(x, 10, 0.8, filled)),
    ],
    "0 0 20 20",
  ),
  "show-controls": line(
    [path("M11 16V4H8a3 3 0 0 0 0 6h3m3-6v12M8 4h8")],
    "0 0 20 20",
  ),
  "hide-controls": line(
    [path("M11 16V4H8a3 3 0 0 0 0 6h3m3-6v12M8 4h8M3 3l14 14")],
    "0 0 20 20",
  ),
  "word-wrap": line(
    [path("M3 4h14M3 9h11a3 3 0 0 1 0 6h-4m3-3-3 3 3 3M3 14h3")],
    "0 0 20 20",
  ),
  "no-word-wrap": line(
    [path("M3 4h14M3 9h14M3 14h9m2-2 3 2-3 2")],
    "0 0 20 20",
  ),
  checkbox: line(
    [
      path("m4.5 9 3 3 6-6", { part: "check" }),
      path("M5 9h8", { part: "mixed" }),
    ],
    "0 0 18 18",
  ),
  "question-open": line([circle(7, 9, 4, { strokeWidth: 1 })], "0 0 16 16"),
  "question-answered": line(
    [
      circle(7, 9, 4, { strokeWidth: 1 }),
      path("m2 8.5 3.2 3 7.3-8", { strokeWidth: 2.6 }),
    ],
    "0 0 16 16",
  ),
  notification: solid([circle(3, 3, 3)], "0 0 6 6"),
  overflow: solid(
    [circle(0.5, 6.5, 0.5), circle(2.5, 6.5, 0.5), circle(4.5, 6.5, 0.5)],
    "0 0 6 12",
  ),
  profile: line(
    [circle(18, 18, 8), circle(18, 18, 2, filled), circle(18, 10, 2, filled)],
    "0 0 36 36",
  ),
  sessions: line([
    path("M5 4h14a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H9l-6 3V6a2 2 0 0 1 2-2Z"),
  ]),
  projects: line([
    path(
      "M3 7V5a2 2 0 0 1 2-2h5l3 3h6a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z",
    ),
  ]),
  tasks: line([path("m3 6 1.5 1.5L7 4M10 6h11M3 12h3m4 0h11M3 18h3m4 0h11")]),
  accounts: line([circle(12, 8, 4), path("M4 21v-3a8 8 0 0 1 16 0v3")]),
  memory: line([path("M4 3h16v18H4Z M8 7h8M8 11h8M8 15h5")]),
  auxiliary: sparkle,
  settings: line([
    circle(12, 12, 3),
    path("M9 3h6l1 3 3 1 2 5-2 5-3 1-1 3H9l-1-3-3-1-2-5 2-5 3-1Z"),
  ]),
  // Outer contours of the Material Symbols Rounded category glyph; see ../assets/icons/README.md.
  work: solid(
    [
      path(
        "m321.23-590.62 127.85-209.53q5.61-9.23 13.65-13.16 8.04-3.92 17.27-3.92t17.27 3.92q8.04 3.93 13.65 13.16l127.85 209.53q5.61 9.04 5.61 18.98 0 9.95-4.61 18.18-4.62 8.23-12.69 13.15-8.08 4.92-18.85 4.92H351.77q-10.77 0-18.85-4.92-8.07-4.92-12.69-13.06-4.61-8.13-4.61-18.07 0-9.95 5.61-19.18ZM700-95.39q-68.85 0-116.73-47.88-47.88-47.88-47.88-116.73t47.88-116.73q47.88-47.88 116.73-47.88t116.73 47.88q47.88 47.88 47.88 116.73t-47.88 116.73Q768.85-95.39 700-95.39Zm-564.61-56.16v-217q0-15.37 10.39-25.72 10.4-10.34 25.77-10.34h217q15.37 0 25.72 10.39 10.34 10.4 10.34 25.77v217q0 15.37-10.39 25.72-10.4 10.34-25.77 10.34h-217q-15.37 0-25.72-10.39-10.34-10.4-10.34-25.77Z",
      ),
    ],
    "0 -960 960 960",
  ),
  idle: solid([circle(8, 8, 5.5)]),
  interrupted: solid([path("m8 2.5 5.5 11h-11Z")]),
  waiting: pause,
  failed: line([ring, path("M8 4.5V8m0 3h.01")], "0 0 16 16"),
  unknown,
  "state-stopped": stopped,
  "state-running": line([ring, circle(8, 8, 2, filled)], "0 0 16 16"),
  "state-created": circlePause,
  "state-absent": line(
    [circle(8, 8, 5.5, { strokeDasharray: "2 2" })],
    "0 0 16 16",
  ),
  "task-todo": line([ring], "0 0 16 16"),
  "task-in-progress": line(
    [ring, path("M8 2.5a5.5 5.5 0 0 1 0 11Z", filled)],
    "0 0 16 16",
  ),
  "task-waiting": circlePause,
  "task-done": line([ring, path("m5 8 2 2 4-4")], "0 0 16 16"),
  "task-canceled": line([ring, path("m4 12 8-8")], "0 0 16 16"),
  "priority-highest": priorityHighest,
  "priority-high": priorityHigh,
  "priority-normal": priorityNormal,
  "priority-low": priorityLow,
  "priority-lowest": priorityLowest,
  "priority-unknown": line(
    [path("M6 6a2 2 0 1 1 3 1.75C8 8.25 8 9 8 9.5M8 12h.01")],
    "0 0 16 16",
  ),
  duration,
  "tokens-input": line([path("M20 4v16M4 12h12M11 7l5 5-5 5")]),
  "tokens-output": line([path("M4 4v16M20 12H8M13 7l-5 5 5 5")]),
  "tokens-cache-read": line([
    path(
      "M4 7V5c0-4 16-4 16 0v2M4 5c0 4 16 4 16 0M4 11v8c0 4 16 4 16 0v-8M4 19c0 4 16 4 16 0M3 12h10M9 9l4 3-4 3",
    ),
  ]),
  "tokens-cache-write": line([
    path(
      "M4 7V5c0-4 16-4 16 0v2M4 5c0 4 16 4 16 0M4 11v8c0 4 16 4 16 0v-8M4 19c0 4 16 4 16 0M21 12H11M15 9l-4 3 4 3",
    ),
  ]),
  "tokens-reasoning": sparkle,
  "tokens-total": line([path("M19 4H5l7 8-7 8h14")]),
  cost: line([
    path("M12 2v20M18 6c-2-3-12-3-12 2 0 6 12 2 12 8 0 5-10 5-12 2"),
  ]),
  "api-duration": line([
    path("M4 19a10 10 0 1 1 16 0M12 12l5-5M3 12h2M19 12h2M12 2v2"),
  ]),
  "api-turns": line([path("M20 7H5l4-4M4 17h15l-4 4M20 7v5M4 17v-5")]),
  "tool-calls": line([
    path("M14 6a5 5 0 0 0-6 6L3 17a3 3 0 0 0 4 4l5-5a5 5 0 0 0 6-6l-3 3-4-4Z"),
  ]),
} satisfies Record<string, IconDefinition>;

export type IconName = keyof typeof iconDefinitions;

export function iconMask(name: IconName) {
  const definition = iconDefinitions[name];
  const shapes = definition.shapes
    .map(({ element, part, ...attributes }) => {
      const attrs = Object.entries(attributes)
        .map(
          ([key, value]) =>
            `${key.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`)}="${value === "currentColor" ? "black" : value}"`,
        )
        .join(" ");
      return `<${element} ${attrs}/>`;
    })
    .join("");
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${definition.viewBox}" fill="${definition.solid ? "black" : "none"}" stroke="${definition.solid ? "none" : "black"}" stroke-width="${ICON_STROKE_WIDTH}" stroke-linecap="round" stroke-linejoin="round">${shapes}</svg>`;
  return `url("data:image/svg+xml,${encodeURIComponent(svg)}")`;
}
