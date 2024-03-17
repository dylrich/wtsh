package termin

type KeyType int

type Key struct {
	Type    KeyType
	Shift   bool
	Alt     bool
	Control bool
	Rune    rune
}

const (
	KeyCharacter KeyType = iota - 1
	KeyUp
	KeyDown
	KeyRight
	KeyLeft
	KeyEscape
	KeyBackspace
	KeyEnter
	KeyQuit
	KeyTab
)

type MouseKeyType int

const (
	MouseLeft MouseKeyType = iota
	MouseRight
	MouseMiddle
)

type ScrollDirection int

const (
	ScrollUp ScrollDirection = iota
	ScrollDown
)

type Point struct {
	X int
	Y int
}

type MousePress struct {
	Point     Point
	Key       MouseKeyType
	Modifiers Modifiers
}

type MouseScroll struct {
	Point     Point
	Modifiers Modifiers
	Direction ScrollDirection
}

type MouseRelease struct {
	Point     Point
	Modifiers Modifiers
}

type Modifiers struct {
	Shift   bool
	Alt     bool
	Control bool
}

type Event interface {
}
