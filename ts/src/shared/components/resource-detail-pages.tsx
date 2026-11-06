import {
  Fragment,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { SlidingPage, SlidingPages } from "@lesomnus/nox";
import styles from "./resource-detail-pages.module.css";

/** Keep the list's position and the outgoing detail while resource pages slide. */
export function ResourceDetailPages({
  open,
  list,
  detail,
  scope,
}: {
  open: boolean;
  list: ReactNode;
  detail: ReactNode;
  scope: unknown;
}) {
  const page = useRef<HTMLDivElement>(null);
  const previous = useRef<{ scope: unknown; node: ReactNode } | undefined>(
    undefined,
  );
  const visited = useRef(false);
  const wasOpen = useRef(open);
  const [entry, setEntry] = useState(0);
  const [present, setPresent] = useState(open);
  const current = <Fragment key={entry}>{detail}</Fragment>;
  useLayoutEffect(() => {
    if (!open) visited.current = true;
    if (open) previous.current = { scope, node: current };
  });
  useLayoutEffect(() => {
    // Keep the outgoing editor intact, but start fresh if reopened during exit.
    if (!open && wasOpen.current) setEntry((value) => value + 1);
    wasOpen.current = open;
    if (open) {
      setPresent(true);
      return;
    }
    if (!present) return;
    let canceled = false;
    const animations = page.current!.getAnimations();
    void Promise.all(
      animations.map((animation) => animation.finished.catch(() => {})),
    ).then(() => {
      if (!canceled) setPresent(false);
    });
    return () => {
      canceled = true;
    };
  }, [open, present]);
  return (
    <SlidingPages
      className={styles.pages}
      value={open ? "detail" : "list"}
      resize="none"
      data-resource-detail-pages
    >
      <SlidingPage value="list" className={styles.page}>
        {!open || visited.current ? list : null}
      </SlidingPage>
      <SlidingPage ref={page} value="detail" className={styles.page}>
        {open
          ? current
          : present && previous.current?.scope === scope
            ? previous.current?.node
            : null}
      </SlidingPage>
    </SlidingPages>
  );
}
