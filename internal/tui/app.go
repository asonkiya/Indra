// Package tui implements the Bubble Tea terminal interface for Indra.
package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/aryaman/indra/pkg/types"
)

// SendFunc is a callback for sending a direct message.
type SendFunc func(ctx context.Context, recipientID peer.ID, text []byte) error

// GroupSendFunc is a callback for sending a group message.
type GroupSendFunc func(ctx context.Context, groupID string, text []byte) error

type focus int

const (
	focusContacts focus = iota
	focusCompose
)

// App is the root Bubble Tea model.
type App struct {
	contacts ContactsModel
	chat     ChatModel
	compose  ComposeModel
	focused  focus

	myPeerID   string
	width      int
	height     int
	statusLine string

	inbound       <-chan types.Message
	sendFunc      SendFunc
	groupSendFunc GroupSendFunc
}

// inboundTickMsg is sent to the app on each poll tick to drain new messages.
type inboundTickMsg struct{}

// NewApp constructs the root TUI model.
func NewApp(
	myPeerID string,
	convos []*types.Conversation,
	inbound <-chan types.Message,
	sendFunc SendFunc,
	groupSendFunc GroupSendFunc,
) *App {
	const sidebarWidth = 32
	const defaultHeight = 40
	const defaultWidth = 120

	chatWidth := defaultWidth - sidebarWidth
	chatHeight := defaultHeight - 6

	app := &App{
		contacts:   newContactsModel(convos, defaultHeight),
		chat:       newChatModel(chatWidth, chatHeight, myPeerID),
		compose:    newComposeModel(defaultWidth),
		focused:    focusCompose, // start ready to type
		myPeerID:   myPeerID,
		width:      defaultWidth,
		height:     defaultHeight,
		statusLine:    fmt.Sprintf("Indra  •  %s  •  Tab: switch focus  •  Enter: send", truncate(myPeerID, 16)+"…"),
		inbound:       inbound,
		sendFunc:      sendFunc,
		groupSendFunc: groupSendFunc,
	}
	return app
}

func (a App) Init() tea.Cmd {
	return pollInbound()
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		chatW := msg.Width - 32
		chatH := msg.Height - 7
		a.chat.width = chatW
		a.chat.height = chatH
		a.chat.viewport.Width = chatW
		a.chat.viewport.Height = chatH
		a.compose.width = msg.Width

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return a, tea.Quit

		case "tab":
			// Toggle focus between contacts list and compose bar.
			if a.focused == focusCompose {
				a.focused = focusContacts
				a.compose.textarea.Blur()
			} else {
				a.focused = focusCompose
				a.compose.textarea.Focus()
			}
			a.updateStatusLine()
			return a, nil
		}

		// Route key events only to the focused component.
		if a.focused == focusCompose {
			var cmd tea.Cmd
			a.compose, cmd = a.compose.Update(msg)
			cmds = append(cmds, cmd)
			return a, tea.Batch(cmds...)
		}
		// focusContacts: let the list handle it
		var cmd tea.Cmd
		a.contacts, cmd = a.contacts.Update(msg)
		cmds = append(cmds, cmd)
		// Load messages for the newly selected conversation.
		if sel := a.contacts.Selected(); sel != nil {
			a.chat.SetMessages(sel.Messages)
		}
		return a, tea.Batch(cmds...)

	case SendMsg:
		if sel := a.contacts.Selected(); sel != nil {
			if sel.IsGroup {
				if a.groupSendFunc != nil {
					go func(groupID, text string) {
						_ = a.groupSendFunc(context.Background(), groupID, []byte(text))
					}(sel.ID, msg.Text)
				}
			} else {
				recipID := a.selectedRecipient(sel)
				if a.sendFunc != nil {
					go func(id peer.ID, text string) {
						_ = a.sendFunc(context.Background(), id, []byte(text))
					}(recipID, msg.Text)
				}
			}
			outMsg := &types.Message{
				Plaintext: []byte(msg.Text),
				SentAt:    time.Now(),
				Direction: types.Outbound,
			}
			a.chat.AppendMessage(outMsg)
		}

	case inboundTickMsg:
		for {
			select {
			case m := <-a.inbound:
				a.chat.AppendMessage(&m)
				a.contacts.UpdateLastMessage(m.ConversationID, &m)
			default:
				goto drained
			}
		}
	drained:
		cmds = append(cmds, pollInbound())
		return a, tea.Batch(cmds...)
	}

	// Non-key messages (window resize, ticks) update all components.
	var cmd tea.Cmd
	a.contacts, cmd = a.contacts.Update(msg)
	cmds = append(cmds, cmd)
	a.chat, cmd = a.chat.Update(msg)
	cmds = append(cmds, cmd)
	a.compose, cmd = a.compose.Update(msg)
	cmds = append(cmds, cmd)

	return a, tea.Batch(cmds...)
}

func (a App) View() string {
	sidebar := a.contacts.View()
	chatArea := a.chat.View()
	main := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, chatArea)
	compose := a.compose.View()
	status := statusBarStyle.Width(a.width).Render(a.statusLine)
	return lipgloss.JoinVertical(lipgloss.Left, main, compose, status)
}

func (a *App) updateStatusLine() {
	focusHint := "Tab: browse contacts"
	if a.focused == focusContacts {
		focusHint = "Tab: back to compose  •  ↑↓: select contact  •  Enter: open"
	}
	a.statusLine = fmt.Sprintf("Indra  •  %s  •  %s", truncate(a.myPeerID, 16)+"…", focusHint)
}

func (a App) selectedRecipient(sel *types.Conversation) peer.ID {
	for _, p := range sel.Participants {
		if p.String() != a.myPeerID {
			return p
		}
	}
	if len(sel.Participants) > 0 {
		return sel.Participants[0]
	}
	return ""
}

func pollInbound() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return inboundTickMsg{}
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
