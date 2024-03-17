package resizestream

import (
	"context"
	"fmt"
	"log"
	"time"

	"golang.org/x/term"
)

type Receiver interface {
	Resize(w, h int)
}

type Listener struct {
	fd       int
	logger   *log.Logger
	receiver Receiver
}

func New(fd int, logger *log.Logger, receiver Receiver) *Listener {
	return &Listener{
		fd:       fd,
		logger:   logger,
		receiver: receiver,
	}
}

func (a Listener) Run(ctx context.Context) error {
	var width, height int

	ticker := time.NewTicker(100 * time.Millisecond)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return nil
		case <-ticker.C:
			w, h, err := term.GetSize(a.fd)
			if err != nil {
				a.logger.Println(fmt.Errorf("get term size: %w", err))
				continue
			}

			if width == w && height == h {
				continue
			}

			a.receiver.Resize(w, h)

			width = w
			height = h
		}
	}
}
