import { IdentifierValue as NoxIdentifierValue } from "@lesomnus/nox";
export function IdentifierValue({
  label,
  onCopy,
  ...props
}: {
  value?: string;
  label?: string;
  prefixLength?: number;
  copying?: boolean;
  onCopy?: () => Promise<void>;
}) {
  return (
    <NoxIdentifierValue
      {...props}
      copyLabel={label}
      onCopy={onCopy}
      copyTrigger={onCopy ? "value" : "button"}
    />
  );
}
