package ansiesc

import "fmt"

func ShowCursor() string {
	return CSI + "?25h"
}

func HideCursor() string {
	return CSI + "?25l"
}

func ClearToEndOfLine() string {
	return CSI + "K"
}

func ClearLine() string {
	return CSI + "2K"
}

func SetPosition(row, column int) string {
	return CSI + fmt.Sprintf("%d;%df", row+1, column+1)
}

func MoveToBeginningOfLine() string {
	return CSI + "1000D"
}

func EnableMouse() string {
	return CSI + "?1000h"
}

func DisableMouse() string {
	return CSI + "?1000l"
}

func MoveDown(n int) string {
	return CSI + fmt.Sprintf("%dE", n)
}

func ClearScreen() string {
	return CSI + "2J"
}

const CSI = "\x1b["
