package inputstream

import (
	"context"
	"fmt"
	"io"
	"log"
	"wtsh/internal/inputstream/internal/termread"
	"wtsh/internal/termin"
)

type Receiver interface {
	Input([]termin.Event)
}

type Reader struct {
	in       io.Reader
	logger   *log.Logger
	receiver Receiver
}

func New(in io.Reader, logger *log.Logger, receiver Receiver) *Reader {
	return &Reader{
		in:       in,
		logger:   logger,
		receiver: receiver,
	}
}

func (r *Reader) Run(ctx context.Context) error {
	consumer := termread.New(r.in, r.logger)

	ch := make(chan []termin.Event)

	// we do this because reading from stdin is uncancellable (kinda),
	// so there is no way to guarantee teardown of the polling routine.
	// Instead, isolate the polling to a goroutine we are OK with leaking on
	// shutdown
	go func() {
		for {
			events, err := consumer.Poll()
			if err != nil {
				r.logger.Println(fmt.Errorf("poll error: %w", err))
				continue
			}

			ch <- events
		}
	}()

	done := ctx.Done()

	for {
		select {
		case <-done:
			return nil
		case events := <-ch:
			r.receiver.Input(events)
		}
	}
}
