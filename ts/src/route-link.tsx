import { createLink } from "@tanstack/react-router";
import { forwardRef, type AnchorHTMLAttributes } from "react";
import { ButtonContent } from "./button";

const NavigationAnchor = forwardRef<
  HTMLAnchorElement,
  AnchorHTMLAttributes<HTMLAnchorElement> & { pressTarget?: string }
>(function NavigationAnchor(
  { children, pressTarget, className = "", ...props },
  ref,
) {
  return (
    <a {...props} ref={ref} className={`route-link ${className}`}>
      <ButtonContent pressTarget={pressTarget}>{children}</ButtonContent>
    </a>
  );
});

// Real links retain native new-tab behavior and the shared button press motion.
export const RouteLink = createLink(NavigationAnchor);
