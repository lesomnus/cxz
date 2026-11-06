import { EditableValue as NoxEditableValue } from "@lesomnus/nox";
import type { ComponentProps } from "react";
import { useErrorCenter } from "#src/shared/errors/error-context.tsx";
import { isAPIError } from "#src/shared/errors/client-errors.ts";

/** Keep validation in Nox; save API failures are presented by the workspace. */
export function EditableValue(props: ComponentProps<typeof NoxEditableValue>) {
  const errors = useErrorCenter();
  return (
    <NoxEditableValue
      {...props}
      formatError={(error) =>
        errors && isAPIError(error, errors) ? "" : String(error)
      }
    />
  );
}
