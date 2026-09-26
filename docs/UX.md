# UX and visual bar

Features don't merge unless they meet this bar. Visual quality is not a
follow-up task.

## Principles

1. **Inside the grid, behave like Google Sheets.** Assume users know
   Sheets, not 1-2-3. Typing replaces a cell, `=` starts a formula, Enter
   commits and moves down (back to the starting column after Tabs), Tab
   moves right, Shift+arrows and the mouse select, Ctrl+arrows jump to the
   data edge, and Sheets shortcuts do what Sheets does. When Sheets and
   1-2-3 disagree, Sheets wins. 1-2-3 lives on as the visual identity:
   the control panel doubling as the formula bar, the mode indicator, the
   crisp character grid.
2. **Familiar, but native to the terminal.** Sheets tells us which jobs
   users expect to get done and which keys they reach for, not what the
   screen must look like. This is a Bubble Tea program: prefer patterns
   that feel at home in a terminal when they serve the same need as well
   or better. Bars and prompts on the context line or status line over
   floating dialogs, incremental search that highlights as you type,
   toggles shown as key chips, pickers in the style of fzf or lazygit, and
   no fake GUI chrome (drop shadows, faux buttons) unless it earns its
   place. Overlays are fine when content genuinely needs room (menus,
   long lists), and they stay keyboard-first.
3. **Everything is discoverable.** Every action is a registered command
   (`internal/ui/commands.go`) with a title and one-line description. It is
   reachable from the menu where Sheets would put it, from key bindings in
   `keymap`, and from help (generated from the registry). The command
   palette lists every registered command with its keys.
4. **The screen always says what's going on.** The mode indicator is always
   correct. The control panel is the menu bar (with the mode indicator),
   the formula bar, and the context line, which holds prompts, small
   confirmations and the keys that apply in any non-READY state. While a
   menu, the palette or the shortcuts are open, the status line says what
   the highlighted item does and which keys apply.
5. **Never lose work.** Destructive actions warn when there are unsaved
   changes and are undoable once undo exists. Errors are specific
   ("expected , or )" with the cursor on the spot), never generic.
6. **Esc backs out one level; Enter confirms.** Everywhere, no exceptions.

## Visual rules

- Style only through `theme` roles (`internal/ui/theme.go`). No inline
  `lipgloss.NewStyle()` in views. Need a new role? Add it to `theme` with a
  comment saying what it's for, in both the dark and light variants.
- Use the 16 ANSI colors so the user's terminal palette applies. No
  hard-coded RGB.
- Text on a colored background needs about 4.5:1 contrast against both
  reference palettes (`e2e/palette_test.go`). Check it in the gallery.
- Never rely on color alone: state also shows as text (mode indicator,
  `modified`, `#DIV/0!`, `Circular reference`).
- Numbers right-aligned, text left, booleans and errors centered, one
  column of padding, text overflowing into empty neighbors: as Sheets.
  Headers centered. Menu bar titles separated by two spaces. Keys shown
  as `F2`, `Ctrl+Z`, `Del` (see `keyLabel`), drawn as key chips
  (`chip`) in hints, menus, the palette and the shortcuts.
- Menus, the palette and the shortcuts are overlays: boxes framed with
  light box-drawing lines (`frame`), composited over the grid without
  moving it, with a title in the top border and a position or count in
  the bottom one. Small questions (quit with unsaved changes) go on the
  context line as a choice bar, not in a box.
- The active cell is always distinct from the rest of a selection, and
  the headers of selected rows and columns are highlighted.
- Layout works from 60x16 up to very wide terminals. Nothing jumps
  position between frames; overlays don't shift the grid.
- Render only what's visible. A keystroke on a 10,000-cell sheet must feel
  instant.

## Required with every user-facing change

- [ ] Registered command(s) with title and description; key binding in
      `keymap` if Sheets has one; menu entry where Sheets would have it.
- [ ] Unit tests in `internal/ui` driving `Update`, plus engine tests.
- [ ] An e2e test in `e2e/` for the main flow.
- [ ] Golden screen(s) in `e2e/screens_test.go` for every new visual state,
      with a `-light` variant if it introduces new colors.
- [ ] `make screens`, then look at `e2e/testdata/screens/gallery.html`
      (serve it over HTTP and screenshot it, or open it in a browser).
      Fix anything misaligned, low-contrast, cramped or clipped before
      committing. Screens are drawn from libghostty's cell grid, so they
      match what a terminal shows.
- [ ] README keys table updated if keys changed.

## Review checklist for screenshots

- Is the mode indicator right? Does line 3 explain the keys?
- Is anything clipped at 60 columns?
- Can every piece of text be read in both palettes?
- Does the new element look like it belongs: same spacing, same roles,
  same capitalization as the rest?
