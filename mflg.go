package main

import (
    "golang.org/x/crypto/ssh/terminal"

    "os"
    "fmt"
    "time"
)

func main() {
    os.Stdout.WriteString("\033[?1049h")
    w, h, err := terminal.GetSize(int(os.Stdin.Fd()))
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
    } else {
        fmt.Printf("size: %dx%d\n", w, h)
    }
    time.Sleep(3 * time.Second)
    os.Stdout.WriteString("\033[?1049l")
}
