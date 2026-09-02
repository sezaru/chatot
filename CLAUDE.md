# chatot

## Mockup-driven visual work

The design mockup lives under `./mockup` (Claude-authored `.dc.html` design-canvas
files, e.g. `mockup/Chatot GTK4 Mockup.dc.html`, `Chatot Interactive.dc.html`,
`Chatot Multi-account.dc.html`). It is the source of truth for anything visual —
layout, spacing, colors, iconography, interaction states.

Before implementing or reviewing a GTK4 feature that has a visual counterpart in
the mockup, get direct visual confirmation of the target component rather than
guessing from the mockup's HTML/CSS alone:

- **If this session is running inside Tidewave** (`tidewave` CLI/host), ask the
  user to open the relevant mockup file in Tidewave and select the exact
  component being implemented — Tidewave hands you that selection directly, so
  there's no need to guess which markup corresponds to it.
- **Otherwise, use the `playwright` MCP server** (declared in `.mcp.json`, backed
  by the nix-provided Chromium via the `playwright-mcp` wrapper in
  `.nix/devenv.nix` — no npm/npx version pinning needed). Open the relevant
  `.dc.html` file under `./mockup` and navigate to the component:
  - Ask the user how to navigate/interact (which screen, which click sequence,
    which state) to reach it, if it isn't obvious.
  - Or, if confident, read the mockup's markup and `mockup/support.js` (drives
    the mockups' interactivity) to figure out the navigation/state yourself.

Take a screenshot (or otherwise inspect the rendered DOM) of the exact component
before writing GTK4 code for it — don't rely on memory of a previous look or on
reading the HTML source alone, since computed layout/spacing/colors only show up
once rendered.
