// Unit-mass damped spring: x'' + damping*x' + stiffness*(x-target) = 0.
// Sample the analytic solution for WAAPI; CSS easing is linear between samples,
// but the displacement is a physical spring, independent of frame rate.
export const SEND_PULL_DISTANCE = 8;
export const SEND_SPRING_DURATION = 540;
export const SEND_PULL_DURATION = 16_000;

// Two critically damped responses: a small immediate tug, then slow tension
// toward the limit. Both start at rest and approach their targets monotonically.
export function sendPullState(ms: number) {
  if (ms >= SEND_PULL_DURATION)
    return { position: SEND_PULL_DISTANCE, velocity: 0 };
  const time = Math.max(0, ms) / 1000;
  let position = 0;
  let velocity = 0;
  for (const [distance, rate] of [
    [3, 14],
    [5, 0.5],
  ]) {
    const envelope = Math.exp(-rate * time);
    position += distance * (1 - (1 + rate * time) * envelope);
    velocity += distance * rate * rate * time * envelope;
  }
  return { position, velocity };
}

export function sendPullFrames(): Keyframe[] {
  const times = [
    ...Array.from({ length: 36 }, (_, i) => (i * 1000) / 60),
    ...Array.from({ length: 155 }, (_, i) => 600 + i * 100),
  ];
  return times.map((ms) => ({
    offset: ms / SEND_PULL_DURATION,
    transform: `translateY(${sendPullState(ms).position}px) scale(1)`,
    opacity: 1,
  }));
}
export function sendSpringState(
  from: number,
  target: number,
  velocity: number,
  ms: number,
) {
  const decay = 12;
  const frequency = Math.sqrt(260 - decay * decay);
  const time = Math.max(0, ms) / 1000;
  const a = from - target;
  const b = (velocity + decay * a) / frequency;
  const cos = Math.cos(frequency * time);
  const sin = Math.sin(frequency * time);
  const envelope = Math.exp(-decay * time);
  const wave = a * cos + b * sin;
  return {
    position: target + envelope * wave,
    velocity: envelope * (frequency * (-a * sin + b * cos) - decay * wave),
  };
}
export function sendSpringFrames(
  from: number,
  target: number,
  velocity = 0,
): Keyframe[] {
  return Array.from({ length: 34 }, (_, index) => {
    const offset = index / 33;
    const y =
      index === 33
        ? target
        : sendSpringState(from, target, velocity, offset * SEND_SPRING_DURATION)
            .position;
    return { offset, transform: `translateY(${y}px) scale(1)`, opacity: 1 };
  });
}
