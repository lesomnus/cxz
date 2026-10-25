import { createLink } from "@tanstack/react-router";
import { forwardRef } from "react";
import {
  ButtonAnchor,
  type ButtonAnchorProps,
  PanelItemAnchor,
} from "@lesomnus/nox";

const NavigationAnchor = forwardRef<HTMLAnchorElement, ButtonAnchorProps>(
  function NavigationAnchor(
    { children, pressTarget, className = "", ...props },
    ref,
  ) {
    return (
      <ButtonAnchor
        {...props}
        ref={ref}
        pressTarget={pressTarget}
        className={`route-link ${className}`}
      >
        {children}
      </ButtonAnchor>
    );
  },
);

// Real links retain native new-tab behavior and the shared button press motion.
export const RouteLink = createLink(NavigationAnchor);
export const PanelLink = createLink(PanelItemAnchor);
