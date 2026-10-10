export const clamp = (n: number, low: number, high: number) =>
  Math.max(low, Math.min(high, n));
