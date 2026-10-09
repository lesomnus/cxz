// Independent, staggered CSS fields give the work indicator an organic drift
// without frame callbacks, random layout changes or intercepting pointer input.
export function ComposerAurora({ active }: { active: boolean }) {
  return (
    <div className="composer-aurora" data-active={active} aria-hidden="true">
      <span />
      <span />
      <span />
    </div>
  );
}
