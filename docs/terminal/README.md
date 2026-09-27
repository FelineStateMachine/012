---
title: "The terminal"
sidebar_position: 1
---

# The terminal

012 uses what the terminal it runs in can do, and degrades quietly where
it can't: charts as images or text, links, the clipboard, the mouse,
colors that follow light and dark.

| Page | For |
|---|---|
| [Themes](themes.md) | Your terminal's colors, a built-in scheme, or your own |
| [Serving over SSH](ssh.md) | `012 serve`: a 012 per session, on one directory |

## Terminal features 012 uses

| Feature | What for | Where it works |
|---|---|---|
| Kitty keyboard protocol | telling apart keys like Ctrl+I and Tab, Shift+Enter | kitty, Ghostty, WezTerm, foot, Alacritty, iTerm2 |
| Mouse, all motion | hover highlights, drag, resize | most terminals |
| Pointer shape (OSC 22) | resize and text cursors over the grid | Ghostty, kitty, foot, xterm |
| Kitty graphics | chart images | kitty, Ghostty, WezTerm |
| Hyperlinks (OSC 8) | links in cells | most modern terminals |
| Clipboard (OSC 52) | copying ranges | most terminals; tmux needs `set-clipboard on` |
| Progress (OSC 9;4) | JEV and import progress in the tab | Ghostty, Windows Terminal, iTerm2 |
| Notifications (OSC 9) | when JEV or an import finishes in the background | iTerm2, Ghostty, WezTerm |
| Curly, colored underlines | error cells | kitty, Ghostty, WezTerm, iTerm2, Alacritty |
| Background color query and mode 2031 | light and dark themes, following system changes | most terminals; 2031 in Ghostty and kitty |
| Synchronized output (2026) | flicker-free redraws | kitty, Ghostty, WezTerm, iTerm2, Alacritty |
