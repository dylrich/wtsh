package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"wtsh/internal/inputstream"
	"wtsh/internal/resizestream"
	"wtsh/internal/wtshapp"
	"wtsh/internal/wtshexec"

	"golang.org/x/sync/errgroup"
	"golang.org/x/term"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

type Runner interface {
	Run(ctx context.Context) error
}

type Process struct {
	Runner     Runner
	Waitgroups []*sync.WaitGroup
}

func runCancelGroup(g *errgroup.Group, parent context.Context, processes ...Process) context.CancelFunc {
	ctx, cancel := context.WithCancel(parent)

	runProcessGroup(g, ctx, processes...)

	return cancel
}

func runProcessGroup(g *errgroup.Group, ctx context.Context, processes ...Process) {
	for _, p := range processes {
		p := p

		for _, wg := range p.Waitgroups {
			wg.Add(1)
		}

		g.Go(func() (err error) {
			defer func() {
				if perr := recover(); perr != nil {
					err = panicToError(perr)
				}
			}()

			err = p.Runner.Run(ctx)

			for _, wg := range p.Waitgroups {
				wg.Done()
			}

			return err
		})
	}
}

func panicToError(a any) error {
	switch v := a.(type) {
	case nil:
		return nil
	case string:
		return fmt.Errorf("panic: %s", v)
	case error:
		return fmt.Errorf("panic: %w", v)
	default:
		return fmt.Errorf("panic: %v", v)
	}
}

func run(args []string, stdin *os.File, stdout, stderr io.Writer) error {
	flags := newFlagSet("wtsh")

	var home string
	var openConfig string
	var sessionConfig string
	var cursorConfig string
	var uri string
	var logPath string

	flags.StringVar(&home, "home", "", "")
	flags.StringVar(&openConfig, "open-config", "", "")
	flags.StringVar(&sessionConfig, "session-config", "", "")
	flags.StringVar(&cursorConfig, "cursor-config", "", "")
	flags.StringVar(&uri, "uri", "", "")
	flags.StringVar(&logPath, "log-path", "", "")

	ok, err := parseFlags(flags, args, stderr, "")
	if err != nil {
		return fmt.Errorf("parse args: %w", err)
	}

	if !ok {
		return nil
	}

	var f *os.File

	if logPath != "" {
		lf, err := os.Create(logPath)
		if err != nil {
			return fmt.Errorf("create log file: %w", err)
		}

		f = lf
	} else {
		null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0755)
		if err != nil {
			return fmt.Errorf("open /dev/null: %w", err)
		}

		f = null
	}
	defer f.Close()

	logfd := f.Fd()

	// TODO: temporary until wtgo can silence stderr/stdout logging
	if err := syscall.Dup2(int(logfd), int(syscall.Stderr)); err != nil {
		return fmt.Errorf("redirect stderr to log file")
	}

	logger := log.New(f, "", log.LUTC|log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)

	fd := int(os.Stdin.Fd())

	w, h, err := term.GetSize(fd)
	if err != nil {
		return fmt.Errorf("get initial size: %w", err)
	}

	old, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("make raw terminal: %w", err)
	}

	g, ctx := errgroup.WithContext(context.Background())
	ctx, cancel := context.WithCancel(ctx)

	cmdch := make(chan string, 2)

	wtshappconf := wtshapp.Config{
		InitialWidth:   w,
		InitialHeight:  h,
		FileDescriptor: fd,
		TermState:      old,
		Stdout:         stdout,
		Cancel:         cancel,
		CommandChannel: cmdch,
		Logger:         logger,
		Home:           home,
		OpenConfig:     openConfig,
		SessionConfig:  sessionConfig,
		CursorConfig:   cursorConfig,
		URI:            uri,
	}

	p := wtshapp.New(wtshappconf)

	defer p.Reset()

	connHandler := wtshexec.New(cmdch, logger, p, cancel)

	resizer := resizestream.New(fd, logger, p)
	reader := inputstream.New(stdin, logger, p)
	cancels := make([]context.CancelFunc, 0, 2)

	wtshappwg := &sync.WaitGroup{}

	{
		resizerp := Process{
			Runner:     resizer,
			Waitgroups: []*sync.WaitGroup{wtshappwg},
		}

		readerp := Process{
			Runner:     reader,
			Waitgroups: []*sync.WaitGroup{wtshappwg},
		}

		connhandlerp := Process{
			Runner:     connHandler,
			Waitgroups: []*sync.WaitGroup{wtshappwg},
		}

		ctx := context.Background()
		cancel := runCancelGroup(g, ctx, resizerp, readerp, connhandlerp)
		cancels = append(cancels, cancel)
	}

	{
		ctx, cancel := context.WithCancel(context.Background())
		wtshappp := Process{
			Runner:     p,
			Waitgroups: nil,
		}

		waiter := &Waiter{
			wg:      wtshappwg,
			cancels: []context.CancelFunc{cancel},
		}

		waiterp := Process{
			Runner:     waiter,
			Waitgroups: nil,
		}

		runProcessGroup(g, ctx, wtshappp, waiterp)
	}

	canceler := NewCanceler(logger, cancels...)

	{
		cancelerp := Process{
			Runner:     canceler,
			Waitgroups: nil,
		}

		runProcessGroup(g, ctx, cancelerp)
	}

	if err := g.Wait(); err != nil {
		return err
	}

	return nil
}

type Waiter struct {
	wg      *sync.WaitGroup
	cancels []context.CancelFunc
}

func (w *Waiter) Run(ctx context.Context) error {
	w.wg.Wait()

	for _, cancel := range w.cancels {
		cancel()
	}

	return nil
}

type Canceler struct {
	logger  *log.Logger
	cancels []context.CancelFunc
}

func NewCanceler(logger *log.Logger, cancels ...context.CancelFunc) *Canceler {
	return &Canceler{
		logger:  logger,
		cancels: cancels,
	}
}

func (c *Canceler) Run(ctx context.Context) error {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	select {
	case <-ctx.Done():
		c.logger.Println("application quitting, shutting down...")
	case s := <-signals:
		c.logger.Printf("\nreceived signal '%s', shutting down...\n", s)
	}

	for _, cancel := range c.cancels {
		cancel()
	}

	return nil
}

func newFlagSet(prog string) *flag.FlagSet {
	f := flag.NewFlagSet(prog, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.Usage = nil

	return f
}

func parseFlags(flags *flag.FlagSet, args []string, stderr io.Writer, usage string) (bool, error) {
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(stderr, usage)
			return false, nil
		}

		return false, fmt.Errorf("argument parsing failure: %w\n\n%s", err, usage)
	}

	return true, nil
}
