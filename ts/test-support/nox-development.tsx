import { useState } from "react";
import { createRoot } from "react-dom/client";
import { Button } from "@lesomnus/nox";
import "@lesomnus/nox/styles.css";

function Preview() {
  const [count, setCount] = useState(0);
  return <Button onClick={() => setCount((n) => n + 1)}>Count {count}</Button>;
}
createRoot(document.getElementById("root")!).render(<Preview />);
