package termout

import (
	"fmt"
	"io"
	"wtsh/internal/ansiesc"
)

type Writer struct {
	out io.Writer
}

func New(w io.Writer) *Writer {
	return &Writer{
		out: w,
	}
}

func (w *Writer) ClearScreen() {
	fmt.Fprint(w.out, ansiesc.ClearScreen())
	w.SetCursor(1, 1)
}

func (w *Writer) ClearLine() {
	fmt.Fprint(w.out, ansiesc.ClearLine())
}

func (w *Writer) SetCursor(row, column int) {
	fmt.Fprint(w.out, ansiesc.SetPosition(row, column))
}

func (w *Writer) Write(p []byte) (int, error) {
	return w.out.Write(p)
}

func (w *Writer) WriteString(s string) (int, error) {
	return w.out.Write([]byte(s))
}
