package main

import (
    "golang.org/x/crypto/ssh/terminal"
    "bytes"
    "io"
    "io/ioutil"
    "os"
    "fmt"
)

func enterAlternateScreen() {
    os.Stdout.WriteString("\033[?1049h")
}

func exitAlternateScreen() {
    os.Stdout.WriteString("\033[?1049l")
}

func renderBuffer(buf []byte, window io.Writer, width, height int) error {
    lines := bytes.SplitN(buf, []byte{'\n'}, height + 1)
    if len(lines) > height {
        lines = lines[:height]
    }
    const gutterSize = 4
    for i, line := range lines {
        if len(line) > width - gutterSize {
            line = line[:width - gutterSize]
        }
        if _, err := fmt.Fprintf(window, "% 3d ", i + 1); err != nil {
            return err
        }
        if _, err := window.Write(line); err != nil {
            return err
        }
        if i + 1 < height {
            if _, err := window.Write([]byte("\r\n")); err != nil {
                return err
            }
        }
    }
    return nil
}

func main() {
    if len(os.Args) < 2 {
        fmt.Fprintln(os.Stderr, "usage:", os.Args[0], "<file>")
        os.Exit(2)
    }
    fname := os.Args[1]
    f, err := os.OpenFile(fname, os.O_RDWR | os.O_CREATE, 0644)
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(2)
    }
    content, err := ioutil.ReadAll(f)
    if err != nil {
        fmt.Fprintf(os.Stderr, "error reading %s: %v", fname, err)
        os.Exit(2)
    }
    w, h, err := terminal.GetSize(int(os.Stdin.Fd()))
    if err != nil {
        fmt.Fprintln(os.Stderr, "error finding terminal size:", err)
        os.Exit(2)
    }
    oldMode, err := terminal.MakeRaw(int(os.Stdin.Fd()))
    if err != nil {
        fmt.Fprintln(os.Stderr, "error entering raw mode:", err)
        os.Exit(2)
    }
    defer terminal.Restore(int(os.Stdin.Fd()), oldMode)
    enterAlternateScreen()
    defer exitAlternateScreen()
    if err := renderBuffer(content, os.Stdout, w, h); err != nil {
        panic(err)
    }
    for {
        var b [5]byte
        n, err := os.Stdin.Read(b[:])
        if err != nil || (n == 1 && b[0] == 'q')  {
            break
        }
        fmt.Printf("%q\r\n", b[:n])
    }
}
