# Semantic UI icons

Use the shared component for static cxz icons:

```tsx
import { Icon } from "#src/shared/icons/icon.tsx";

<Icon name="start" />
<Icon name="stop" className={styles.actionIcon} />
```

`icon-definitions.ts` owns the geometry, fill and semantic names. Define a new
meaning there before using it in a component. Reuse a definition for aliases:
`start` and `resume`, `restart` and `refresh`, and `info` and `details` each share
one glyph. Domain components may map API states to icon names; they do not copy
SVG paths. `IconName` validates these references at compile time.

Icons inherit `currentColor`. Use `size`, `width`/`height` or a class for sizing.
`--ui-icon-stroke-width` overrides the common line weight; its fallback is
`ICON_STROKE_WIDTH` in the definition file. Ordinary button icons are decorative
by default. Provide `aria-label` when the icon itself needs an accessible name.

For icon-only page toolbar actions, use
`shared/components/toolbar-icon-button.tsx`, passing a semantic `icon`, `label`
and behavior. It owns the Nox toolbar button appearance and the icon size through
`--resource-toolbar-icon-size`; pages do not specify their own SVG dimensions.

Simple play, stop, priority and work-summary shapes are filled. Outlines that
communicate an unanswered question, an unchecked control, absence or partial
progress retain that distinction. Dynamic charts, quota rings and loading
animations stay with the components that calculate or animate their geometry.

`iconMask(name)` creates a CSS mask from the same definition used by `Icon`, so
Task priority menus, image attachment chips and their visible glyphs stay in sync.

Storybook → **Foundations / Icons / Catalogue** displays all registered names.
Company logos remain in `shared/assets/brands` with their usage rules. The
Material category source and license remain in `shared/assets/icons`.
