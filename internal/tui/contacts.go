package tui

import (
	"fmt"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aryaman/indra/pkg/types"
)

// conversationItem wraps a Conversation for the bubbles list.
type conversationItem struct {
	conv *types.Conversation
}

func (i conversationItem) Title() string {
	name := i.conv.Name
	if name == "" {
		name = i.conv.ID[:12] + "…"
	}
	if i.conv.IsGroup {
		name = fmt.Sprintf("# %s [%d]", name, len(i.conv.Participants))
	}
	if i.conv.UnreadCount > 0 {
		return fmt.Sprintf("%s (%d)", name, i.conv.UnreadCount)
	}
	return name
}

func (i conversationItem) Description() string {
	if i.conv.LastMessage == nil {
		return "No messages yet"
	}
	text := string(i.conv.LastMessage.Plaintext)
	if len(text) > 40 {
		text = text[:40] + "…"
	}
	return text
}

func (i conversationItem) FilterValue() string { return i.Title() }

// ContactsModel is the left sidebar showing conversations.
type ContactsModel struct {
	list     list.Model
	selected *types.Conversation
	width    int
	height   int
}

func newContactsModel(convos []*types.Conversation, height int) ContactsModel {
	items := make([]list.Item, len(convos))
	for i, c := range convos {
		items[i] = conversationItem{conv: c}
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = selectedItemStyle
	delegate.Styles.NormalTitle = normalItemStyle

	l := list.New(items, delegate, 28, height-4)
	l.Title = "Conversations"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Padding(0, 1)

	return ContactsModel{list: l, height: height, width: 30}
}

func (m ContactsModel) Update(msg tea.Msg) (ContactsModel, tea.Cmd) {
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	if item, ok := m.list.SelectedItem().(conversationItem); ok {
		m.selected = item.conv
	}
	return m, cmd
}

func (m ContactsModel) View() string {
	return sidebarStyle.Render(m.list.View())
}

func (m ContactsModel) Selected() *types.Conversation {
	return m.selected
}

// AddConversation appends a conversation to the list.
func (m *ContactsModel) AddConversation(c *types.Conversation) {
	m.list.InsertItem(0, conversationItem{conv: c})
}

// UpdateLastMessage refreshes the description for a conversation.
func (m *ContactsModel) UpdateLastMessage(convID string, msg *types.Message) {
	items := m.list.Items()
	for i, it := range items {
		ci, ok := it.(conversationItem)
		if !ok {
			continue
		}
		if ci.conv.ID == convID {
			ci.conv.LastMessage = msg
			_ = m.list.SetItem(i, ci)
			return
		}
	}
}

