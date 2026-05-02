## Waffle

A recreation of Windoes. It's WIP!

- If you install snry Shell fully, you can press `Super+Alt+W` to switch to this style.
- If you're just copying the Quickshell config, run the config as usual (`qs -c ii`) then run `qs -c ii ipc call panelFamily cycle`

## From EWW version to Quickshell

A reflection on the migration. Originally written for the EWW widget system, Waffle was ported to Quickshell to take advantage of QtQuick's richer feature set.

### Improvements

- QtQuick's `Button` has `{top/bottom/left/right}Inset` properties, so clickable regions can expand beyond the button background for free. With EWW it required wrapping button content with an `eventbox` and juggling CSS selectors for hover effects.

- **Fancy effects**: GTK3 CSS does not support transformations. In QtQuick we can apply `rotation` and `scale` almost everywhere, making bouncy icons and rotating chevrons straightforward.

- Quickshell provides a built-in system tray service, eliminating the need for a separate Waybar instance just for the tray.

- QtQuick has `Loader`s, so Waffle can be live-switched from the main style without killing the widget system, moving files around, and relaunching.

- This time around there's enough hardware to run a VM for pixel-perfect reference screenshots — critical because hardcoded sizes remain, but Qt's `QT_SCALE_FACTOR` env var handles scaling cleanly.

### Challenges

- Qt is not GTK and definitely not React
  - QtQuick `Rectangle`s don't have directional borders like CSS. Workaround: manual drawing.

- Fluent Icons is harder to use than Material Symbols
  - No React library, no searchable codepoint cheatsheet like Nerd Fonts, no ligatures.
  - Resorted to downloading individual SVGs via fluenticon.com and fluenticons.co.
  - Icons are awkwardly named — the reload/refresh icon is "arrow-sync". From Fluent Design's [Iconography page](https://fluent2.microsoft.design/iconography):

    > Fluent system icons are literal metaphors and are named for the shape or object they represent, not the functionality they provide.
