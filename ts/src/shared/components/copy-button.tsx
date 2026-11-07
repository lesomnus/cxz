import { CopyButton as NoxCopyButton } from "@lesomnus/nox";
export function CopyButton({
  value,
  className = "",
}: {
  value: string;
  className?: string;
}) {
  return (
    <NoxCopyButton
      value={value}
      className="copy"
      containerClassName={className}
    />
  );
}
