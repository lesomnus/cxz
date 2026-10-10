import { Button } from "./button";

// The owner supplies IDs so panels and tabs can share accessible relationships.
export function TabList({
  prefix,
  panelId,
  label,
  items,
  value,
  choose,
}: {
  prefix: string;
  panelId: string;
  label: string;
  items: { id: string; label: string; answered?: boolean }[];
  value: string;
  choose: (id: string) => void;
}) {
  return (
    <div className="tab-list" role="tablist" aria-label={label}>
      {items.map((item, index) => (
        <Button
          key={item.id}
          id={`${prefix}-${index}`}
          type="button"
          role="tab"
          aria-selected={item.id === value}
          aria-controls={panelId}
          tabIndex={item.id === value ? 0 : -1}
          onClick={() => choose(item.id)}
          onKeyDown={(event) => {
            let next: number;
            if (event.key === "ArrowRight") next = (index + 1) % items.length;
            else if (event.key === "ArrowLeft")
              next = (index - 1 + items.length) % items.length;
            else if (event.key === "Home") next = 0;
            else if (event.key === "End") next = items.length - 1;
            else return;
            event.preventDefault();
            choose(items[next].id);
            document.getElementById(`${prefix}-${next}`)?.focus();
          }}
        >
          <span className="tab-label">{item.label}</span>
          {item.answered !== undefined && (
            <span
              className="tab-answered"
              data-answered={item.answered}
              aria-hidden="true"
            >
              {item.answered ? "✓" : "○"}
            </span>
          )}
        </Button>
      ))}
    </div>
  );
}
