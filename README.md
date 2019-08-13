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
- **Cut**: Control-X
- **Paste**: Control-V
- **Undo**: Control-Z
- **Undo All/Discard Changes**: Control-U (will ask for confirmation)
- **Replace**: Control-R, then type a regex, then the replacement. You may use $1, $2, $3... to refer to captured groups, and $name or ${name} to refer to named groups. To insert a literal $, use $$ (see [the Go regexp docs][go-regexp]). If you have some text selected, only that text is affected.
- **Format**: Control-F - pipes the contents of the buffer through the formatter program for the current file's language, then replaces the buffer with the result.
- **Quit**: Control-Q

mflg saves your files automatically as you make changes, so there is no Save command as in other editors; except for a small delay, what you see on screen is what is on disk.
Hence, **Quit** exits the editor unconditionally.
If you want to throw away the changes you've made to a file since opening it, use the **Undo All** command; if you want to make absolutely sure you don't lose the original version, make a backup before editing the file.
(If the file is tracked by a version control system, the VCS provides such a backup.)

[go-regexp]: https://golang.org/pkg/regexp/#Regexp.Expand

### Movement

When opening mflg, the argument follows the same syntax as the **Go to Location** command. If you
launch mflg with no arguments, it opens a new scratch file; its location is displayed in the terminal
title as well as printed to standard error, which you can see after exiting the editor.

- **Back**: Control-B - goes back to the last location from where **Go to Location** was used
- **Go to Location**: Control-L
  - Typing a filename alone navigates to the start of that file
  - Typing a string of the form "filename:loc" (colon-separated) navigates to the file, then:
    - If loc is a positive integer, jumps to the line loc
    - Otherwise, it treats it as a regex and jumps to its first occurrence
    - If the filename part is empty, the command navigates in the current file. (ex.: you can use ":20" to go to line 20)
  - Environment variables (using $VAR or ${VAR} syntax) in filenames are expanded to their values, and ~ expands to your home directory, just like in a shell
  - Filenames are interpreted relatively to the current file's parent directory, or the working directory when starting up
- **Find Next**: Control-G - if the last use of **Go to Location** specified a regex, goes to the next occurrence of that regex after the line the cursor is on. Wraps around the end of the file if necessary.
- **Move cursor**: arrow keys (hold down/press repeatedly to move faster)

_Caveat_: To use the **Go to Location** command to find a number, enclose it in a group (ex.: `(666)`) so that it isn't
mistaken for a line number.

### Selection

To select a range of text, use **Anchor** at each end of the range consecutively, in any order.
Alternatively, you may click and drag the mouse to select, like in a GUI editor.
You can also double-click and double-click-and-drag to select by words.

- **Anchor**: Control-A
- **Clear Selection**: ESC (cancels any in-progress selection as well)

## Configuration

mflg's configuration file is written in [TOML][]. It is found in the following location:

- on macOS, ~/Library/Application Support/mflg/config.toml
- on other Unixes, ~/.config/mflg/config.toml, or $XDG_CONFIG_DIR/mflg/config.toml if $XDG_CONFIG_DIR is set

The following keys are available:

- TabWidth: how many spaces a tab character is rendered as
- ScrollSpeed: how many lines to scroll for each tick of the scroll wheel

The text styles for highlighting go in the `[textstyle]` section. Each key maps to a style descriptor with the following keys:

- Foreground, Background: hex color codes for the text color
- Bold, Italic, Underline: self-explanatory booleans

Style descriptors can be set for keys `Comment` and `String`.

Language-specific settings are put under `[lang.AAA]` sections, where AAA is the file name extension used for that language's source files. Keys:

- Formatter: array containing the name of a formatter program (ex.: gofmt for Go), followed by optional arguments.

[TOML]: https://github.com/toml-lang/toml