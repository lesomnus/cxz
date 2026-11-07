import { AuroraGlow } from "@lesomnus/nox";
export function ComposerAurora({ active }: { active: boolean }) {
  return (
    <div className="composer-aurora-viewport">
      <AuroraGlow
        className="composer-aurora"
        active={active}
        palette={[
          "var(--aurora-green)",
          "var(--aurora-cyan)",
          "var(--aurora-violet)",
        ]}
      />
    </div>
  );
}
