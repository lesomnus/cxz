import {
  useId,
  useRef,
  type CSSProperties,
  type HTMLAttributes,
  type ReactNode,
} from "react";
import { useBadgeMask } from "./use-badge-mask";
import styles from "./badged-icon.module.css";

export type BadgedIconProps = HTMLAttributes<HTMLSpanElement> & {
  children: ReactNode;
  badge?: ReactNode;
  position?: "top-right" | "top-left" | "bottom-right" | "bottom-left";
  badgeClassName?: string;
  badgeStyle?: CSSProperties;
  shape?: "ellipse" | "rounded-rectangle";
  /** Local CSS pixels. Omit to follow the SVG's rendered stroke width. */
  gap?: number;
  /** Rounded-rectangle radius; omitted values follow the badge's CSS radius. */
  radius?: number;
};

// The icon is an untouched child. Only its containing layer receives the mask;
// the badge stays above it, with real transparent clearance instead of a border.
export function BadgedIcon({
  children,
  badge,
  position = "top-right",
  badgeClassName = "",
  badgeStyle,
  shape = "ellipse",
  gap,
  radius,
  className = "",
  ...props
}: BadgedIconProps) {
  const id = useId();
  const root = useRef<HTMLSpanElement>(null);
  const icon = useRef<HTMLSpanElement>(null);
  const overlay = useRef<HTMLSpanElement>(null);
  const mask = useRef<SVGMaskElement>(null);
  const full = useRef<SVGRectElement>(null);
  const ellipse = useRef<SVGEllipseElement>(null);
  const rectangle = useRef<SVGRectElement>(null);
  const present = badge !== undefined && badge !== null && badge !== false;
  useBadgeMask(
    { root, icon, overlay, mask, full, ellipse, rectangle },
    {
      present,
      shape,
      gap,
      radius,
    },
  );
  return (
    <span
      {...props}
      ref={root}
      className={`${styles.root} ${className}`}
      data-badged-icon
      data-position={position}
    >
      <span
        ref={icon}
        className={styles.icon}
        data-badged-icon-content
        style={
          present
            ? { maskImage: `url("#${id}")`, WebkitMaskImage: `url("#${id}")` }
            : undefined
        }
      >
        {children}
      </span>
      {present && (
        <>
          <span
            ref={overlay}
            className={`${styles.badge} ${badgeClassName}`}
            style={badgeStyle}
            data-badged-icon-badge
          >
            {badge}
          </span>
          <svg
            className={styles.definitions}
            width="0"
            height="0"
            aria-hidden="true"
            focusable="false"
          >
            <defs>
              <mask
                ref={mask}
                id={id}
                maskUnits="userSpaceOnUse"
                maskContentUnits="userSpaceOnUse"
                x="0"
                y="0"
                style={{ maskType: "luminance" }}
              >
                <rect ref={full} x="0" y="0" fill="white" />
                <ellipse ref={ellipse} fill="black" stroke="black" />
                <rect ref={rectangle} fill="black" stroke="black" />
              </mask>
            </defs>
          </svg>
        </>
      )}
    </span>
  );
}
