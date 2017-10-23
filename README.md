# mflg

mflg is a programmer's CLI text editor designed to be lightweight and easy to set up
and use, while still having the key features other editors have to make coding easier.
In other words, more like an advanced nano and less like vi or emacs.

## Design goals

- Do not duplicate functionality found in other tools:
  - If you want multiple panes/tabs, use tmux or multiple terminals
  - If you want a file browser, use your OS's file browser
- Have as few configuration options as possible - instead, favor doing the right thing
  automatically
- Have a small, easy-to-use command set (i.e. no 4 ways to quit)
- Support extensibility only for language-specific features:
  - Syntax highlighting
  - Autocompletion
  - Inline compile errors/warnings
- First-class mouse support

## Commands

- **Quit**: Control-Q

### Movement

- **Go to Line**: Control-L
- **Find**: Control-F
- **Move cursor**: arrow keys (hold down/press repeatedly to move faster)

### Selection

To select a range of text, use **Anchor** at each end of the range consecutively, in any order.

- **Anchor**: Control-A
- **Clear Selection**: Control-X