// Unit-mass damped spring: x'' + damping*x' + stiffness*(x-target) = 0.
// Sample the analytic solution for WAAPI; CSS easing is linear between samples,
// but the displacement is a physical spring, independent of frame rate.
export const SEND_PULL_DISTANCE = 8;
export const SEND_SPRING_DURATION = 540;
export const SEND_PULL_MINIMUM = 100;
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
