import styles from "./confirm-button.module.css";
import { useState, type ComponentProps } from "react";
import { Button } from "../button/button";

// Confirmation lasts only while the user stays on this control. Touch users
// can tap twice; lifting a finger must not clear the first tap.
export function ConfirmButton({
  confirmationLabel,
  onConfirm,
  className = "",
  ...props
}: Omit<ComponentProps<typeof Button>, "onClick"> & {
  confirmationLabel: string;
  onConfirm: NonNullable<ComponentProps<typeof Button>["onClick"]>;
}) {
  const [confirming, setConfirming] = useState(false);
  return (
    <Button
      {...props}
      className={`${styles.confirm_button} confirm-button ${className}`}
      data-confirming={confirming}
      aria-label={confirming ? confirmationLabel : props["aria-label"]}
      title={confirming ? confirmationLabel : props.title}
      onClick={(event) => {
        if (confirming) {
          setConfirming(false);
          onConfirm(event);
        } else setConfirming(true);
      }}
      onPointerLeave={(event) => {
        if (event.pointerType !== "touch") setConfirming(false);
        props.onPointerLeave?.(event);
      }}
      onBlur={(event) => {
        setConfirming(false);
        props.onBlur?.(event);
      }}
      onKeyDown={(event) => {
        if (event.key === "Escape" && confirming) {
          event.preventDefault();
          setConfirming(false);
        }
        props.onKeyDown?.(event);
      }}
    />
  );
}
