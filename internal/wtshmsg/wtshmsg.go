package wtshmsg

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"
)

type DatabaseConnectedMessage struct {
	Home string
}

func (m DatabaseConnectedMessage) String() string {
	return fmt.Sprintf("connected to database at '%s'", m.Home)
}

type DatabaseDisconnectedMessage struct {
	Home string
}

func (m DatabaseDisconnectedMessage) String() string {
	return fmt.Sprintf("disconnected from '%s'", m.Home)
}

type CreateMessage struct {
	Name string
}

func (m CreateMessage) String() string {
	return fmt.Sprintf("created '%s'", m.Name)
}

type DropMessage struct {
	Name string
}

func (m DropMessage) String() string {
	return fmt.Sprintf("dropped '%s'", m.Name)
}

type NewSessionMessage struct {
}

func (m NewSessionMessage) String() string {
	return "new session started"
}

type NewCursorMessage struct {
	URI string
}

func (m NewCursorMessage) String() string {
	return fmt.Sprintf("new cursor on '%s' opened", m.URI)
}

type ClosedCursorMessage struct {
}

func (m ClosedCursorMessage) String() string {
	return "cursor closed"
}

type ResultMessage struct {
	Rows [][]string
}

func (m ResultMessage) String() string {
	buf := bytes.NewBuffer([]byte{})
	w := tabwriter.NewWriter(buf, 10, 0, 2, ' ', 0)
	for i, row := range m.Rows {
		fmt.Fprint(w, strings.Join(row, "\t"))
		if i == len(m.Rows)-1 {
			continue
		}

		fmt.Fprint(w, "\n")
	}
	w.Flush()
	return buf.String()
}
