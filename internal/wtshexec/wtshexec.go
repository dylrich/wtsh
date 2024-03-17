package wtshexec

import (
	"context"
	"fmt"
	"log"
	"strings"
	"wtsh/internal/wtshmsg"

	"github.com/dylrich/wtgo"
)

type MessageHandler interface {
	HandleMessage(m any)
}

func New(cmdch <-chan string, logger *log.Logger, handler MessageHandler, cancel context.CancelFunc) *ConnectionHandler {
	return &ConnectionHandler{
		actions: make(chan func(), 10),
		cmdch:   cmdch,
		handler: handler,
		logger:  logger,
		cancel:  cancel,
	}
}

type ConnectionHandler struct {
	actions chan func()
	cmdch   <-chan string
	handler MessageHandler
	cancel  context.CancelFunc
	logger  *log.Logger
	state   state
}

type state struct {
	conn    *wtgo.Connection
	home    string
	session *wtgo.Session
	cursor  *wtgo.Cursor
}

func (r *ConnectionHandler) handle(s string) error {
	r.logger.Printf("running command '%s'\n", s)

	parts := strings.SplitN(s, " ", 2)
	cmd := parts[0]

	var args string
	if len(parts) > 1 {
		args = parts[1]
	}

	switch cmd {
	case "connect", "open":
		if r.state.conn != nil {
			return fmt.Errorf("already connected")
		}

		parts := strings.SplitN(args, " ", 2)

		if len(parts) == 0 {
			return fmt.Errorf("parse: open <home> [config]")
		}

		home := parts[0]

		var config string

		if len(parts) == 2 {
			config = parts[1]
		}

		conn, err := wtgo.Open(home, config)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}

		r.state.conn = conn
		r.state.home = home

		r.handler.HandleMessage(wtshmsg.DatabaseConnectedMessage{Home: home})
	case "close-session":
		if r.state.session == nil {
			return fmt.Errorf("no active session")
		}

		if err := r.state.session.Close(""); err != nil {
			return fmt.Errorf("close: %w", err)
		}

		r.state.session = nil
	case "close-cursor":
		if r.state.cursor == nil {
			return fmt.Errorf("no active cursor")
		}

		if err := r.state.cursor.Close(); err != nil {
			return fmt.Errorf("close cursor: %w", err)
		}

		r.state.cursor = nil

		r.handler.HandleMessage(wtshmsg.ClosedCursorMessage{})
	case "drop":
		if r.state.session == nil {
			return fmt.Errorf("no active session")
		}

		parts := strings.SplitN(args, " ", 2)

		if len(parts) == 0 {
			return fmt.Errorf("parse: drop <name> [config]")
		}

		name := parts[0]

		var config string

		if len(parts) == 2 {
			config = parts[1]
		}

		if err := r.state.session.Drop(name, config); err != nil {
			return fmt.Errorf("drop: %w", err)
		}

		r.handler.HandleMessage(wtshmsg.DropMessage{Name: name})
	case "insert":
		if r.state.cursor == nil {
			return fmt.Errorf("no active cursor")
		}

		parts := strings.SplitN(args, " ", 2)

		if len(parts) < 2 {
			return fmt.Errorf("parse: insert <keys> <values>")
		}

		keyss := parts[0]
		valuess := parts[1]

		keys := strings.Split(keyss, ",")
		values := strings.Split(valuess, ",")

		keysa := make([]any, 0, len(keys))
		for _, k := range keys {
			keysa = append(keysa, k)
		}

		valuesa := make([]any, 0, len(values))
		for _, k := range values {
			valuesa = append(valuesa, k)
		}

		if err := r.state.cursor.SetKey(keysa...); err != nil {
			return fmt.Errorf("set key: %w", err)
		}

		if err := r.state.cursor.SetValue(valuesa...); err != nil {
			return fmt.Errorf("set value: %w", err)
		}

		if err := r.state.cursor.Insert(); err != nil {
			return fmt.Errorf("insert: %w", err)
		}
	case "remove":
		if r.state.cursor == nil {
			return fmt.Errorf("no active cursor")
		}

		keyss := args

		keys := strings.Split(keyss, ",")

		keysa := make([]any, 0, len(keys))
		for _, k := range keys {
			keysa = append(keysa, k)
		}

		if err := r.state.cursor.SetKey(keysa...); err != nil {
			return fmt.Errorf("set key: %w", err)
		}

		if err := r.state.cursor.Remove(); err != nil {
			return fmt.Errorf("remove: %w", err)
		}
	case "reset":
		if r.state.cursor == nil {
			return fmt.Errorf("no active cursor")
		}

		if err := r.state.cursor.Reset(); err != nil {
			return fmt.Errorf("reset: %w", err)
		}
	case "set-key":
		if r.state.cursor == nil {
			return fmt.Errorf("no active cursor")
		}

		keyss := strings.Split(args, ",")
		keysa := make([]any, 0, len(keyss))
		for _, k := range keyss {
			keysa = append(keysa, k)
		}

		if err := r.state.cursor.SetKey(keysa...); err != nil {
			return fmt.Errorf("set key: %w", err)
		}
	case "set-value":
		if r.state.cursor == nil {
			return fmt.Errorf("no active cursor")
		}

		valuess := strings.Split(args, ",")
		valuesa := make([]any, 0, len(valuess))
		for _, k := range valuess {
			valuesa = append(valuesa, k)
		}

		if err := r.state.cursor.SetValue(valuesa...); err != nil {
			return fmt.Errorf("set value: %w", err)
		}
	case "search":
		if r.state.cursor == nil {
			return fmt.Errorf("no active cursor")
		}

		if len(args) != 0 {
			keyss := strings.Split(args, ",")

			keysa := make([]any, 0, len(keyss))
			for _, k := range keyss {
				keysa = append(keysa, k)
			}

			if err := r.state.cursor.Reset(); err != nil {
				return fmt.Errorf("reset: %w", err)
			}

			if err := r.state.cursor.SetKey(keysa...); err != nil {
				return fmt.Errorf("set key: %w", err)
			}
		}

		if err := r.state.cursor.Search(); err != nil {
			return fmt.Errorf("search: %w", err)
		}

		keycount := r.state.cursor.KeyCount()
		valuecount := r.state.cursor.ValueCount()

		n := keycount + valuecount
		// TODO: is there a less gross way to do this?
		keys := make([]any, keycount)
		for i := range keys {
			var d any
			keys[i] = &d
		}

		values := make([]any, valuecount)
		for i := range values {
			var d any
			values[i] = &d
		}

		if err := r.state.cursor.GetKey(keys...); err != nil {
			return fmt.Errorf("get key: %w", err)
		}

		if err := r.state.cursor.GetValue(values...); err != nil {
			return fmt.Errorf("get key: %w", err)
		}

		row := make([]string, 0, n)

		for _, k := range keys {
			d := *k.(*any)
			row = append(row, fmt.Sprintf("%v", d))
		}

		for _, v := range values {
			d := *v.(*any)
			row = append(row, fmt.Sprintf("%v", d))
		}

		r.handler.HandleMessage(wtshmsg.ResultMessage{Rows: [][]string{row}})
	case "search-all-next":
		if r.state.cursor == nil {
			return fmt.Errorf("no active cursor")
		}

		keycount := r.state.cursor.KeyCount()
		valuecount := r.state.cursor.ValueCount()

		n := keycount + valuecount

		// TODO: is there a less gross way to do this?
		keys := make([]any, keycount)
		for i := range keys {
			var d any
			keys[i] = &d
		}

		values := make([]any, valuecount)
		for i := range values {
			var d any
			values[i] = &d
		}

		rows := make([][]string, 0, 0)

		for r.state.cursor.Next() {
			if err := r.state.cursor.GetKey(keys...); err != nil {
				return fmt.Errorf("get key: %w", err)
			}

			if err := r.state.cursor.GetValue(values...); err != nil {
				return fmt.Errorf("get key: %w", err)
			}

			row := make([]string, 0, n)

			for _, k := range keys {
				d := *k.(*any)
				row = append(row, fmt.Sprintf("%v", d))
			}

			for _, v := range values {
				d := *v.(*any)
				row = append(row, fmt.Sprintf("%v", d))
			}

			rows = append(rows, row)
		}

		if err := r.state.cursor.Err(); err != nil {
			return fmt.Errorf("iteration: %w", err)
		}

		r.handler.HandleMessage(wtshmsg.ResultMessage{Rows: rows})
	case "create":
		if r.state.session == nil {
			return fmt.Errorf("no active session")
		}

		parts := strings.SplitN(args, " ", 2)

		if len(parts) == 0 {
			return fmt.Errorf("parse: create <name> [config]")
		}

		name := parts[0]

		var config string

		if len(parts) == 2 {
			config = parts[1]
		}

		if err := r.state.session.Create(name, config); err != nil {
			return fmt.Errorf("create: %w", err)
		}

		r.handler.HandleMessage(wtshmsg.CreateMessage{Name: name})
	case "open-cursor":
		if r.state.session == nil {
			return fmt.Errorf("no active session")
		}

		if r.state.cursor != nil {
			return fmt.Errorf("cursor already open")
		}

		parts := strings.SplitN(args, " ", 2)

		if len(parts) == 0 {
			return fmt.Errorf("parse: open-cursor <uri> [config]")
		}

		uri := parts[0]

		var config string

		if len(parts) == 2 {
			config = parts[1]
		}

		cursor, err := r.state.session.OpenCursor(uri, config)
		if err != nil {
			return fmt.Errorf("open cursor: %w", err)
		}

		r.state.cursor = cursor

		r.handler.HandleMessage(wtshmsg.NewCursorMessage{URI: uri})
	case "open-session":
		if r.state.conn == nil {
			return fmt.Errorf("not connected to a database")
		}

		if r.state.session != nil {
			return fmt.Errorf("session already open")
		}

		config := args

		session, err := r.state.conn.OpenSession(config)
		if err != nil {
			return fmt.Errorf("open session: %w", err)
		}

		r.state.session = session

		r.handler.HandleMessage(wtshmsg.NewSessionMessage{})
	case "disconnect", "close":
		if r.state.conn == nil {
			return fmt.Errorf("not connected to a database")
		}

		if err := r.state.conn.Close(""); err != nil {
			return fmt.Errorf("close database connection: %w", err)
		}

		r.handler.HandleMessage(wtshmsg.DatabaseDisconnectedMessage{Home: r.state.home})

		r.state.conn = nil
		r.state.session = nil
		r.state.home = ""

	case "quit":
		r.cancel()
	default:
		return fmt.Errorf("'%s' is not a valid command", cmd)
	}

	return nil
}

func (r *ConnectionHandler) close() error {
	if r.state.conn != nil {
		if err := r.state.conn.Close(""); err != nil {
			return fmt.Errorf("close conn: %w", err)
		}
	}

	return nil
}

func (r *ConnectionHandler) Run(ctx context.Context) error {
	done := ctx.Done()

	for {
		select {
		case <-done:
			if err := r.close(); err != nil {
				r.handler.HandleMessage(err)
			}

			return nil
		case s := <-r.cmdch:
			if err := r.handle(s); err != nil {
				r.handler.HandleMessage(err)
			}
		case a := <-r.actions:
			a()
		}
	}
}
