package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/aryaman/indra/pkg/types"
)

// ChatModel renders a message thread for a conversation.
type ChatModel struct {
	viewport viewport.Model
	messages []*types.Message
	myPeerID string
	width    int
	height   int
}

func newChatModel(width, height int, myPeerID string) ChatModel {
	vp := viewport.New(width, height)
	vp.SetContent("Select a conversation to start chatting.")
	return ChatModel{
		viewport: vp,
		myPeerID: myPeerID,
		width:    width,
		height:   height,
	}
}

func (m ChatModel) Update(msg tea.Msg) (ChatModel, tea.Cmd) {
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m ChatModel) View() string {
	return chatAreaStyle.Width(m.width).Render(m.viewport.View())
}

// SetMessages replaces the displayed messages and scrolls to the bottom.
func (m *ChatModel) SetMessages(msgs []*types.Message) {
	m.messages = msgs
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

// AppendMessage adds a single new message and scrolls to the bottom.
func (m *ChatModel) AppendMessage(msg *types.Message) {
	m.messages = append(m.messages, msg)
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

func (m *ChatModel) renderMessages() string {
	if len(m.messages) == 0 {
		return "No messages yet. Say hello!"
	}

	var sb strings.Builder
	for _, msg := range m.messages {
		text := string(msg.Plaintext)
		if text == "" {
			text = "[encrypted]"
		}
		ts := msg.SentAt.Format(time.Kitchen)

		if msg.Direction == types.Outbound || msg.SenderID.String() == m.myPeerID {
			// Right-aligned sent message.
			line := fmt.Sprintf("%s  %s", text, timestampStyle.Render(ts))
			sb.WriteString(sentStyle.Width(m.width - 4).Render(line))
		} else {
			// Left-aligned received message.
			line := fmt.Sprintf("%s  %s", text, timestampStyle.Render(ts))
			sb.WriteString(receivedStyle.Width(m.width - 4).Render(line))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
