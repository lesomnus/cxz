// Keep the field in the fixed composer layer, above the transcript scroll fades
// and behind the translucent toolbar. Its position follows layout without JS.
export function ComposerAurora({ active }: { active: boolean }) {
  return (
    <div className="composer-aurora" data-active={active} aria-hidden="true">
      <span />
      <span />
      <span />
    </div>
  );
}
