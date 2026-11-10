import { createElement, type SVGProps } from "react";
import {
  iconDefinitions,
  projectIconDefinitions,
  ICON_STROKE_WIDTH,
  type IconName,
  type ProjectIconName,
} from "./icon-definitions";

export {
  iconDefinitions,
  iconMask,
  projectIconDefinitions,
  projectIconNames,
  type IconName,
  type ProjectIconName,
} from "./icon-definitions";

// One renderer for both maps. The names stay in separate types so that a
// component reaching for an action icon is not offered a leaf, and a palette
// never grows because something needed a new meaning.
const definitions: Record<string, (typeof iconDefinitions)[IconName]> = {
  ...iconDefinitions,
  ...projectIconDefinitions,
};

export type IconProps = Omit<SVGProps<SVGSVGElement>, "children" | "name"> & {
  name: IconName | ProjectIconName;
  size?: number | string;
};

export function Icon({ name, size = 16, ...props }: IconProps) {
  const definition = definitions[name];
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
