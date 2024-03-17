package wtshapp

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
	"unicode/utf8"
	"wtsh/internal/ansiesc"
	"wtsh/internal/termin"
	"wtsh/internal/termout"
	"wtsh/internal/wtshmsg"

	"golang.org/x/term"
)

type Config struct {
	InitialWidth   int
	InitialHeight  int
	TermState      *term.State
	FileDescriptor int
	Stdout         io.Writer
	Cancel         context.CancelFunc
	CommandChannel chan<- string
	Logger         *log.Logger
	Home           string
	OpenConfig     string
	SessionConfig  string
	CursorConfig   string
	URI            string
}

func New(conf Config) *Program {
	w := termout.New(conf.Stdout)

	m := &model{
		height:   conf.InitialHeight,
		width:    conf.InitialWidth,
		messages: make([]string, 0, 10),
	}

	commandHandler := func(s string) {
		// skip if no one is reading
		select {
		case conf.CommandChannel <- s:
		case <-time.After(1 * time.Second):
			conf.Logger.Printf("skipped running command '%s'\n", s)
		}

	}

	onSubmit := func(p, s string) {
		commandHandler(s)
		m.addLog(p + s)
	}

	return &Program{
		invalidated:    make(chan struct{}, 1),
		actions:        make(chan func()),
		fd:             conf.FileDescriptor,
		old:            conf.TermState,
		writer:         w,
		logger:         conf.Logger,
		cancel:         conf.Cancel,
		box:            newInputbox(conf.Logger, onSubmit),
		logs:           newLogs(w),
		model:          m,
		home:           conf.Home,
		openConfig:     conf.OpenConfig,
		sessionConfig:  conf.SessionConfig,
		cursorConfig:   conf.CursorConfig,
		uri:            conf.URI,
		commandHandler: commandHandler,
	}
}

func (m *model) addLogSplit(s string) {
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		m.addLog(line)
	}
}

func (m *model) addLog(s string) {
	m.messages = append(m.messages, s)
}

type Program struct {
	invalidated    chan struct{}
	actions        chan func()
	old            *term.State
	fd             int
	writer         *termout.Writer
	logger         *log.Logger
	cancel         context.CancelFunc
	model          *model
	box            *inputBox
	logs           *logs
	home           string
	openConfig     string
	sessionConfig  string
	cursorConfig   string
	uri            string
	commandHandler func(s string)
}

type model struct {
	height   int
	width    int
	messages []string
}

func (p *Program) Input(events []termin.Event) {
	p.actions <- func() {
		for _, e := range events {
			switch v := e.(type) {
			case termin.Key:
				p.inputKey(v)

			}
			p.box.Update(e, p.model)
		}

		p.invalidate()
	}
}

func (p *Program) render() {
	s := screen{
		buffer: os.Stdout,
		height: p.model.height,
		width:  p.model.width,
	}

	p.logs.render(s, p.model)
	p.box.render(s)
}

type inputBox struct {
	index        int
	Content      string
	prompt       string
	cmdch        chan<- string
	submit       func(p, s string)
	history      []string
	historyIndex int
	logger       *log.Logger
}

func newInputbox(logger *log.Logger, submit func(p, s string)) *inputBox {
	return &inputBox{
		index:   0,
		prompt:  "$ ",
		Content: "",
		logger:  logger,
		submit:  submit,
		history: make([]string, 0, 10),
	}
}

func (b *inputBox) Update(e termin.Event, m *model) {
	switch v := e.(type) {
	case termin.Key:
		switch v.Type {
		case termin.KeyEnter:
			if len(b.Content) > 0 {
				b.history = append(b.history, b.Content)
				b.submit(b.prompt, b.Content)
			}

			b.index = 0
			b.Content = ""
			b.historyIndex = 0
		case termin.KeyUp:
			if len(b.history) == 0 {
				return
			}

			if b.historyIndex < len(b.history) {
				b.historyIndex++
			}

			i := len(b.history) - b.historyIndex
			b.Content = b.history[i]

			b.index = len(b.Content)
		case termin.KeyDown:
			if b.historyIndex == 0 {
				return
			}

			b.historyIndex--

			if b.historyIndex > 0 {
				i := len(b.history) - b.historyIndex
				b.Content = b.history[i]
			} else {
				b.Content = ""
			}

			b.index = len(b.Content)
		case termin.KeyRight:
			if b.index < len(b.Content) {
				b.index++
			}
		case termin.KeyLeft:
			if b.index > 0 {
				b.index--
			}
		case termin.KeyCharacter:
			b.Content = b.Content[:b.index] + string(v.Rune) + b.Content[b.index:]
			if len(b.Content)+len(b.prompt) > m.width {
				b.Content = b.Content[:m.width]
			} else {
				b.index++
			}
		case termin.KeyBackspace:
			if b.index == 0 {
				break
			}

			b.Content = b.Content[:b.index-1] + b.Content[b.index:]
			b.index--
		}
	}
}

type screen struct {
	buffer io.Writer
	height int
	width  int
}

func (b *inputBox) render(s screen) {
	fmt.Fprint(s.buffer, ansiesc.SetPosition(s.height, 0)+ansiesc.ClearToEndOfLine()+b.prompt+b.Content+ansiesc.SetPosition(s.height, b.index+utf8.RuneCountInString(b.prompt)))
}

func (p *Program) Quit() {
	p.actions <- func() {
		p.cancel()
	}
}

func (p *Program) inputKey(k termin.Key) {
	switch k.Type {
	case termin.KeyQuit:
		p.cancel()
	}
}

func (p *Program) Resize(w, h int) {
	p.actions <- func() {
		p.model.height = h
		p.model.width = w
		p.invalidate()
	}
}

func (p *Program) HandleMessage(m any) {
	p.actions <- func() {
		switch v := m.(type) {
		case error:
			p.model.addLog(v.Error())
		case string:
			p.model.addLog(v)
		case wtshmsg.DatabaseConnectedMessage:
			p.model.addLog(v.String())
			p.box.prompt = fmt.Sprintf("[%s]$ ", v.Home)
		case wtshmsg.DatabaseDisconnectedMessage:
			p.model.addLog(v.String())
			p.box.prompt = "$ "
		case wtshmsg.NewSessionMessage:
			p.model.addLog(v.String())
		case wtshmsg.ClosedCursorMessage:
			p.model.addLog(v.String())
		case wtshmsg.NewCursorMessage:
			p.model.addLog(v.String())
		case wtshmsg.CreateMessage:
			p.model.addLog(v.String())
		case wtshmsg.ResultMessage:
			p.model.addLogSplit(v.String())
		case wtshmsg.DropMessage:
			p.model.addLog(v.String())
		}

		p.invalidate()
	}
}

func (p *Program) invalidate() {
	select {
	case p.invalidated <- struct{}{}:
		return
	default:
		return
	}
}

func (p *Program) Reset() {
	fmt.Print(ansiesc.DisableMouse() + ansiesc.ClearScreen() + ansiesc.ShowCursor() + ansiesc.SetPosition(0, 0))
	term.Restore(p.fd, p.old)
	p.logger.Println("RESET")
}

func (p *Program) Run(ctx context.Context) error {
	fmt.Print(ansiesc.ClearScreen() + ansiesc.EnableMouse())

	done := ctx.Done()

	if p.home != "" {
		cmd := fmt.Sprintf("open %s %s %s", p.home, p.openConfig, p.sessionConfig)
		p.commandHandler(cmd)

		cmd = "open-session"
		if p.sessionConfig != "" {
			cmd += " " + p.sessionConfig
		}
		p.commandHandler(cmd)

		if p.uri != "" {
			cmd = fmt.Sprintf("open-cursor %s", p.uri)
			if p.cursorConfig != "" {
				cmd += " " + p.cursorConfig
			}

			p.commandHandler(cmd)
		}
	}

	for {
		select {
		case <-done:
			return nil
		case <-p.invalidated:
			p.render()
		case a := <-p.actions:
			a()

			select {
			case <-p.invalidated:
				p.render()
			default:
				continue
			}
		}
	}
}

type logs struct {
	w *termout.Writer
}

func newLogs(w *termout.Writer) *logs {
	return &logs{
		w: w,
	}
}

func (l *logs) render(s screen, m *model) {
	space := m.height - 1 // subtract 1 to leave space for input box

	for i, n := 0, len(m.messages)-1; i < space && n > -1; i, n = i+1, n-1 {
		line := m.messages[n]
		l.w.SetCursor(space-(i+1), 0)
		l.w.ClearLine()
		l.w.WriteString(line)
	}
}
