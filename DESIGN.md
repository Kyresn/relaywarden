# RelayWarden design system

Direction: Midnight Transit. A working gateway console read as a transit diagram:
feature lines, auth stations, service states. The world contributes type,
palette, density, and one signature move (the route strip); navigation and
controls stay standard web controls.

## Color

| Token | Value | Use |
|---|---|---|
| `--ground` | `#0b1722` | page ground |
| `--ground-2` | `#0e1d2b` | top bar, rail, event tape |
| `--panel` | `#112436` | active row / expanded detail |
| `--panel-2` | `#162c40` | selected segment |
| `--ink` | `#eef2ee` | primary text |
| `--muted` | `#8fa3b8` | secondary text |
| `--dim` | `#7f93a6` | labels, meta |
| `--line` | `#294054` | rules and borders |
| `--line-soft` | `#1c3143` | row dividers |
| `--porcelain` | `#f2f4f1` | strongest text, active tab |
| `--green` | `#4f9d69` | compression line, healthy |
| `--amber` | `#d9a441` | guard line, warning |
| `--red` | `#d64545` | alerts line |
| `--red-text` | `#e06a5a` | critical numerals and alert tape |
| `--cobalt` | `#3d6fb4` | routing line |
| `--cobalt-text` | `#6f9fe0` | routing numerals and config tape |

State is never color alone: every lamp has a text label, every switch says
ON/OFF, every auth row says Healthy/Watch/Degraded/Cold.

## Type

- UI: `"Segoe UI Variable Text", "Segoe UI", system-ui, sans-serif`.
- Data, IDs, logs, numbers: `"Cascadia Mono", "Cascadia Code", ui-monospace`.
- Body 13px / 1.45; section heading 15px / 600; readout number 22px mono.
- No external fonts. Windows-native faces keep the build offline and CSP-safe.

## Layout

- Shell rows: 56px top bar, fluid body, 44px event tape.
- Body columns: 96px line rail + main. Below 1100px the rail becomes a
  horizontal line bar; below 760px it wraps to a 2x2 grid.
- Main: tab row, scrolling page. Sections are separated by 1px rules, not cards.
- Tables are dense; horizontal scroll below 760px.

## Components

- Switch: 34x18 track, 12px thumb, 160ms ease. Tone follows the line.
- Segmented control: 1px border, active segment uses `--panel-2` plus a 2px
  amber inset rule.
- Buttons: 3px radius, 1px border; primary cobalt, danger red, quiet transparent.
- Tables: sticky header, 1px row rules, right-aligned tabular numbers.
- Confirmations for dangerous off-switches are inline rows, never modals.
- Event tape: bottom strip, newest four signals, severity colors on the code.

## Motion

One authored moment: flipping a line updates the route strip and appends a config
change to the tape. Transitions are 150–220ms; `prefers-reduced-motion` disables
them.

## Accessibility

- `role="switch"` + `aria-checked`; Space/Enter work.
- Amber focus ring, 2px offset.
- Body text ≥4.5:1; large text ≥3:1.
- Tables remain operable without hover.
- Language toggle is text (`中` / `EN`), not an icon.
- Language and last-config-change preferences use `localStorage`; the management key never does.

## Anti-patterns refused

- Same-size card grids and nested cards.
- Kicker/eyebrow labels above headings.
- Gradient text, glass decoration, neon-on-black.
- Monospace as a costume for labels; mono is reserved for data and code.
- A modal for routine configuration.
