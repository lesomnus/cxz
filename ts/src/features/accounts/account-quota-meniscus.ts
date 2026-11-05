// Reference coordinates for the inset tube; CSS controls its displayed size.
export const QUOTA_TUBE_VIEWBOX = "0 0 30 58";

export function quotaEmptyContour(remaining: number | undefined): string {
  const width = 30;
  const height =
    (58 * (100 - Math.min(100, Math.max(0, remaining ?? 100)))) / 100;
  const radius = 13;
  const topDepth = Math.min(radius, height * 0.45);
  const bottomDepth = Math.min(radius, height - topDepth);
  const opening = topDepth / radius;
  // Pull the shoulders inward before they meet the tube, keeping the ends round.
  const halfWidth =
    width / 2 -
    radius +
    radius * Math.sqrt(2 * opening - opening ** 2) -
    radius * 0.23 * Math.sqrt(opening) * (1 - opening);
  const cornerWidth = Math.min(radius, halfWidth);
  const left = width / 2 - halfWidth;
  const right = width / 2 + halfWidth;
  const circle = 0.5522847498;
  const topCurve = circle - 0.08 * (1 - opening);
  const bottomCurve = circle + 0.12 * (1 - opening);
  const n = (value: number) => Number(value.toFixed(4));
  return [
    `M ${width / 2} 0`,
    `H ${n(left + cornerWidth)}`,
    `C ${n(left + cornerWidth * (1 - topCurve))} 0 ${n(left)} ${n(topDepth * (1 - topCurve))} ${n(left)} ${n(topDepth)}`,
    `V ${n(height - bottomDepth)}`,
    `C ${n(left)} ${n(height - bottomDepth * (1 - bottomCurve))} ${n(left + cornerWidth * (1 - bottomCurve))} ${n(height)} ${n(left + cornerWidth)} ${n(height)}`,
    `H ${n(right - cornerWidth)}`,
    `C ${n(right - cornerWidth * (1 - bottomCurve))} ${n(height)} ${n(right)} ${n(height - bottomDepth * (1 - bottomCurve))} ${n(right)} ${n(height - bottomDepth)}`,
    `V ${n(topDepth)}`,
    `C ${n(right)} ${n(topDepth * (1 - topCurve))} ${n(right - cornerWidth * (1 - topCurve))} 0 ${n(right - cornerWidth)} 0`,
    "Z",
  ].join(" ");
}
