export function observeApprovalOcclusion(
  stack: HTMLElement,
  composer: HTMLElement,
) {
  let frame = 0;
  const previous = new WeakMap<HTMLElement, string>();
  let faces: { face: HTMLElement; height: number; animations: Animation[] }[] =
    [];
  const refresh = () => {
    const elements = [
      stack.querySelector<HTMLElement>("[data-approval-exiting]"),
      stack.querySelector<HTMLElement>("[data-motion]"),
      ...stack.querySelectorAll<HTMLElement>("[data-approval-back] > div"),
    ].filter((face): face is HTMLElement => !!face);
    const before = new Set(faces.map((item) => item.face));
    const next = new Set(elements);
    for (const face of before) if (!next.has(face)) observer.unobserve(face);
    for (const face of next) if (!before.has(face)) observer.observe(face);
    faces = elements.map((face) => ({
      face,
      height: parseFloat(getComputedStyle(face).height) || 0,
      animations: face.getAnimations(),
    }));
  };
  const paint = () => {
    frame = 0;
    const boxes = faces.map((item) => ({
      ...item,
      box: item.face.getBoundingClientRect(),
    }));
    let edge = composer.getBoundingClientRect().top;
    for (const { face, box, height } of boxes) {
      const scale = height > 0 ? box.height / height : 1;
      const covered =
        scale > 0
          ? Math.max(0, Math.min(height, (box.bottom - edge) / scale))
          : height;
      const clip = `inset(0px 0px ${covered.toFixed(3)}px 0px)`;
      if (previous.get(face) !== clip) {
        face.style.clipPath = clip;
        previous.set(face, clip);
      }
      edge = Math.min(edge, box.top);
    }
    if (
      boxes.some(({ animations }) =>
        animations.some(
          (a) => a.playState === "running" || a.playState === "paused",
        ),
      )
    )
      frame = requestAnimationFrame(paint);
  };
  const schedule = () => {
    refresh();
    if (!frame) frame = requestAnimationFrame(paint);
  };
  const observer = new ResizeObserver(schedule);
  const children = new MutationObserver(schedule);
  observer.observe(stack);
  observer.observe(composer);
  children.observe(stack, { childList: true, subtree: true });
  stack.addEventListener("animationstart", schedule);
  stack.addEventListener("transitionrun", schedule);
  refresh();
  paint();
  return () => {
    cancelAnimationFrame(frame);
    observer.disconnect();
    children.disconnect();
    stack.removeEventListener("animationstart", schedule);
    stack.removeEventListener("transitionrun", schedule);
  };
}
