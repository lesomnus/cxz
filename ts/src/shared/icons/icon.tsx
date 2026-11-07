import { createElement, type SVGProps } from "react";
import {
  iconDefinitions,
  ICON_STROKE_WIDTH,
  type IconName,
} from "./icon-definitions";

export { iconDefinitions, iconMask, type IconName } from "./icon-definitions";

export type IconProps = Omit<SVGProps<SVGSVGElement>, "children" | "name"> & {
  name: IconName;
  size?: number | string;
};

export function Icon({ name, size = 16, ...props }: IconProps) {
  const definition = iconDefinitions[name];
  return (
    <svg
      width={size}
      height={size}
      viewBox={definition.viewBox}
      fill={definition.solid ? "currentColor" : "none"}
      stroke={definition.solid ? "none" : "currentColor"}
      strokeWidth={`var(--ui-icon-stroke-width, ${ICON_STROKE_WIDTH})`}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden={
        props["aria-label"] || props["aria-labelledby"] ? undefined : true
      }
      focusable="false"
      data-icon={name}
      {...props}
    >
      {definition.shapes.map(({ element, part, ...attributes }, index) =>
        createElement(element, {
          ...attributes,
          "data-icon-part": part,
          key: index,
        }),
      )}
    </svg>
  );
}
