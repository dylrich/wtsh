package termread

import (
	"fmt"
	"io"
	"log"
	"unicode/utf8"
	"wtsh/internal/termin"
)

type Consumer struct {
	in     io.Reader
	buf    []byte
	logger *log.Logger
}

func New(r io.Reader, logger *log.Logger) *Consumer {
	return &Consumer{
		in:     r,
		buf:    make([]byte, 128),
		logger: logger,
	}
}

func (r *Consumer) Poll() ([]termin.Event, error) {
	n, err := r.in.Read(r.buf)
	if err != nil {
		return nil, fmt.Errorf("reader: %w", err)
	}

	b := r.buf[:n]

	runes := make([]rune, 0)
	for i := 0; i < len(b); {
		r, w := utf8.DecodeRune(b[i:])
		runes = append(runes, r)
		i += w
	}

	keys := make([]termin.Event, 0)

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch r {
		case 3:
			keys = append(keys, termin.Key{Type: termin.KeyQuit})
		case 9:
			keys = append(keys, termin.Key{Type: termin.KeyTab})
		case 13:
			keys = append(keys, termin.Key{Type: termin.KeyEnter})
		case 27:
			if len(runes) == 1 {
				keys = append(keys, termin.Key{Type: termin.KeyEscape})
				continue
			}

			j, k := runes[i+1], runes[i+2]
			i += 2

			switch j {
			case 91:
				switch k {
				case 49:

				}
			}

			switch j {
			case 91:
				switch k {
				case 77:
					// mouse events! wee!
					p := termin.Point{
						X: int(runes[i+2]) - 1,
						Y: int(runes[i+3]) - 1,
					}
					b := runes[i+1]
					b -= 32
					i += 3

					var mods termin.Modifiers

					if b&(1<<2) != 0 {
						mods.Control = true
					}

					if b&(1<<3) != 0 {
						mods.Alt = true
					}

					if b&(1<<4) != 0 {
						mods.Shift = true
					}

					if b&(1<<6) != 0 {
						switch b & 3 {
						case 0:
							keys = append(keys, termin.MouseScroll{Point: p, Modifiers: mods, Direction: termin.ScrollUp})
						case 1:
							keys = append(keys, termin.MouseScroll{Point: p, Modifiers: mods, Direction: termin.ScrollDown})
						}
					} else {
						switch b & 3 {
						case 0:
							keys = append(keys, termin.MousePress{Point: p, Modifiers: mods, Key: termin.MouseLeft})
						case 1:
							keys = append(keys, termin.MousePress{Point: p, Modifiers: mods, Key: termin.MouseMiddle})
						case 2:
							keys = append(keys, termin.MousePress{Point: p, Modifiers: mods, Key: termin.MouseRight})
						case 3:
							keys = append(keys, termin.MouseRelease{Point: p, Modifiers: mods})
						}
					}
				case 65:
					keys = append(keys, termin.Key{Type: termin.KeyUp})
				case 66:
					keys = append(keys, termin.Key{Type: termin.KeyDown})
				case 67:
					keys = append(keys, termin.Key{Type: termin.KeyRight})
				case 68:
					keys = append(keys, termin.Key{Type: termin.KeyLeft})
				}
			}
		case 127:
			keys = append(keys, termin.Key{Type: termin.KeyBackspace})
		default:
			keys = append(keys, termin.Key{Type: termin.KeyCharacter, Rune: r})
		}
	}

	return keys, nil
}
