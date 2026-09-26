# UX and visual bar

Features don't merge unless they meet this bar. Visual quality is not a
follow-up task.

## Principles

1. **1-2-3 muscle memory is the default.** `/` menus with first-letter
   selection, POINT mode, F2/F5, label prefixes. New power is added
   alongside, never by breaking these.
2. **Everything is discoverable.** Every action is a registered command
   (`internal/ui/commands.go`) with a title and one-line description. It is
   reachable from the slash menu where 1-2-3 would put it, from key bindings
   in `keymap`, and from help (generated from the registry). When the
   command palette lands, it lists every registered command with its keys.
3. **The screen always says what's going on.** The mode indicator is always
   correct. The third panel line tells the user what keys do in any
   non-READY state. Prompts live on the second panel line.
4. **Never lose work.** Destructive actions warn when there are unsaved
   changes and are undoable once undo exists. Errors are specific
   ("expected , or )" with the cursor on the spot), never generic.
5. **Esc backs out one level; Enter confirms.** Everywhere, no exceptions.

## Visual rules

- Style only through `theme` roles (`internal/ui/theme.go`). No inline
  `lipgloss.NewStyle()` in views. Need a new role? Add it to `theme` with a
  comment saying what it's for, in both the dark and light variants.
- Use the 16 ANSI colors so the user's terminal palette applies. No
  hard-coded RGB.
- Text on a colored background needs about 4.5:1 contrast against both
  reference palettes (`e2e/palette_test.go`). Check it in the gallery.
- Never rely on color alone: state also shows as text (mode indicator,
  `[modified]`, `ERR`, `CIRC`).
- Numbers right-aligned, labels left unless prefixed, headers centered.
  Menu items separated by two spaces. Keys shown as `F2`, `Ctrl+Z`, `Del`
  (see `keyLabel`).
- Layout works from 60x16 up to very wide terminals. Nothing jumps
  position between frames; overlays don't shift the grid.
- Render only what's visible. A keystroke on a 10,000-cell sheet must feel
  instant.

## Required with every user-facing change

- [ ] Registered command(s) with title and description; key binding in
      `keymap` if it has one; menu entry where 1-2-3 would have it.
- [ ] Unit tests in `internal/ui` driving `Update`, plus engine tests.
- [ ] An e2e test in `e2e/` for the main flow.
- [ ] Golden screen(s) in `e2e/screens_test.go` for every new visual state,
      with a `-light` variant if it introduces new colors.
- [ ] `make screens`, then look at `e2e/testdata/screens/gallery.html`
      (serve it over HTTP and screenshot it, or open it in a browser).
      Fix anything misaligned, low-contrast, cramped or clipped before
      committing.
- [ ] README keys table updated if keys changed.

## Review checklist for screenshots

- Is the mode indicator right? Does line 3 explain the keys?
- Is anything clipped at 60 columns?
- Can every piece of text be read in both palettes?
- Does the new element look like it belongs: same spacing, same roles,
  same capitalization as the rest?
