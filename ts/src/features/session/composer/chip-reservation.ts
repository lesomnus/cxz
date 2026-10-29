const join = "\u2060";
const boundary = "\u200b";
const divider = "\u2063";
const spaces = [
  "\u2007",
  "\u00a0",
  "\u202f",
  "\u2009",
  "\u200a",
  "\u2002",
  "\u2004",
  "\u2005",
  "\u2006",
];
const reservations = new Map<string, string>();

// The visible chip owns its layout. Only its measured advance belongs in the
// textarea; these nonbreaking spaces reproduce that advance in the same font.
export function reserveChip(parent: HTMLElement, width: number) {
  const probeWidth = 100;
  const probe = document.createElement("span");
  Object.assign(probe.style, {
    position: "fixed",
    left: "-100000px",
    top: "0",
    visibility: "hidden",
    display: "inline-block",
    width: `${probeWidth}px`,
    whiteSpace: "pre",
    pointerEvents: "none",
  });
  parent.append(probe);
  // Covered Questions animate their scale. Keep reservation text in CSS pixels
  // so a visual transform cannot change the textarea's native undo positions.
  const scale = probe.getBoundingClientRect().width / probeWidth;
  const measure = (text: string) => {
    probe.textContent = text;
    const range = document.createRange();
    range.selectNodeContents(probe);
    return Math.round((range.getBoundingClientRect().width / scale) * 64);
  };
  try {
    const target = Math.round(width * 64);
    const body = boundary + divider + boundary;
    const base = measure(body);
    const coins = spaces
      .map((space) => ({
        text: join + space + join,
        width: measure(join + space + join),
      }))
      .filter((coin) => coin.width > 0);
    const budget = Math.max(0, target - base);
    const key = JSON.stringify([target, base, coins.map((coin) => coin.width)]);
    const cached = reservations.get(key);
    if (cached) return cached;
    // A monospace font can give every candidate the same advance. Reserving
    // the closest smaller sum would put the native caret inside the chip.
    const smallest = Math.min(...coins.map((coin) => coin.width));
    const limit = budget + smallest;
    const count = new Uint16Array(limit + 1).fill(65535);
    const previous = new Int16Array(limit + 1).fill(-1);
    count[0] = 0;
    for (let total = 1; total <= limit; total++)
      for (let i = 0; i < coins.length; i++) {
        const before = total - coins[i].width;
        if (before >= 0 && count[before] + 1 < count[total]) {
          count[total] = count[before] + 1;
          previous[total] = i;
        }
      }
    let best = budget;
    while (count[best] === 65535) best++;
    let padding = "";
    while (best > 0) {
      const coin = coins[previous[best]];
      padding += coin.text;
      best -= coin.width;
    }
    let result = boundary + padding + divider + boundary;
    // Verify the complete run, rather than assuming separately rounded glyph
    // advances add up identically after shaping and browser zoom.
    const minimum = coins.find((coin) => coin.width === smallest)!;
    while (measure(result) < target) {
      padding += minimum.text;
      result = boundary + padding + divider + boundary;
    }
    reservations.set(key, result);
    if (reservations.size > 500)
      reservations.delete(reservations.keys().next().value!);
    return result;
  } finally {
    probe.remove();
  }
}
