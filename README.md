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
- Have a small, easy-to-use, orthogonal command set
- Support extensibility only for language-specific features that make sense in an editor:
  - Syntax highlighting
  - Autocompletion
  - Inline compile errors/warnings
- First-class mouse support

## Commands

- **Copy**: Control-C
- **Paste**: Control-V
- **Replace**: Control-R, then type a regex, then the replacement. You may use $1, $2, $3... to refer to captured groups, and $name or ${name} to refer to named groups. To insert a literal $, use $$ (see [the Go regexp docs][go-regexp]).
- **Save**: Control-S
- **Quit**: Control-Q

[go-regexp]: https://golang.org/pkg/regexp/#Regexp.Expand

### Movement

- **Go to Location**: Control-L
  - Type line number to go to that line
  - Type a regex to go to the first occurrence of that regex
- **Move cursor**: arrow keys (hold down/press repeatedly to move faster)

_Caveat_: Right now, it isn't possible to use the **Go to Location** command to find a number, because it will be interpreted as a line number. This may change at some point.

### Selection

To select a range of text, use **Anchor** at each end of the range consecutively, in any order.
Alternatively, you may click and drag the mouse to select, like in a GUI editor.

- **Anchor**: Control-A
- **Clear Selection**: Control-X (cancels any in-progress selection as well)