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
  const probe = document.createElement("span");
  Object.assign(probe.style, {
    position: "fixed",
    left: "-100000px",
    top: "0",
    visibility: "hidden",
    display: "inline-block",
    width: "max-content",
    whiteSpace: "pre",
    pointerEvents: "none",
  });
  parent.append(probe);
  const measure = (text: string) => {
    probe.textContent = text;
    const range = document.createRange();
    range.selectNodeContents(probe);
    return Math.round(range.getBoundingClientRect().width * 64);
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
    const limit = budget;
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
    let best = 0;
    for (let total = 1; total <= limit; total++)
      if (
        count[total] < 65535 &&
        (Math.abs(total - budget) < Math.abs(best - budget) ||
          (Math.abs(total - budget) === Math.abs(best - budget) &&
            count[total] < count[best]))
      )
        best = total;
    let padding = "";
    while (best > 0) {
      const coin = coins[previous[best]];
      padding += coin.text;
      best -= coin.width;
    }
    const result = boundary + padding + divider + boundary;
    reservations.set(key, result);
    if (reservations.size > 500)
      reservations.delete(reservations.keys().next().value!);
    return result;
  } finally {
    probe.remove();
  }
}
