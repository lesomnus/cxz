# UI icons

`category-rounded-wght300.svg` is the official Google Material Symbols Rounded
`category` icon, using the lighter weight requested for the work summary.
The source SVG is stored unchanged. The `work` icon in
`shared/icons/icon-definitions.ts` derives solid shapes from its outer contours;
the inner outline cutouts are removed to match cxz's filled basic shapes.
`shared/components/category-icon.tsx` references that semantic icon. It inherits
`currentColor` in both themes without an icon font or a runtime download.

- [Official SVG](https://fonts.gstatic.com/s/i/short-term/release/materialsymbolsrounded/category/wght300/24px.svg)
- [Material Symbols guide](https://developers.google.com/fonts/docs/material_symbols)
- [Source repository](https://github.com/google/material-design-icons)

Material Symbols use Apache License 2.0. The license is stored beside the asset
and included in the deployed `public/third-party-notices.txt`.

Unlike hand-drawn stroke icons, this SVG describes the outline as filled geometry.
Its weight is part of the asset and cannot be adjusted with CSS `stroke-width`.
