package tui

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// SendMsg is emitted when the user presses Enter to send a message.
type SendMsg struct {
	Text string
}

// ComposeModel is the bottom message compose bar.
type ComposeModel struct {
	textarea textarea.Model
	width    int
}

func newComposeModel(width int) ComposeModel {
	ta := textarea.New()
	ta.Placeholder = "Type a message… (Enter to send, Shift+Enter for newline)"
	ta.Focus()
	ta.SetWidth(width)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.CharLimit = 4096

	// Disable the default newline on Enter so we can intercept it.
	ta.KeyMap.InsertNewline.SetKeys("shift+enter")

	return ComposeModel{textarea: ta, width: width}
}

func (m ComposeModel) Update(msg tea.Msg) (ComposeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			text := m.textarea.Value()
			if text == "" {
				return m, nil
			}
			m.textarea.Reset()
			return m, func() tea.Msg {
				return SendMsg{Text: text}
			}
		}
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

func (m ComposeModel) View() string {
	return composeStyle.Width(m.width).Render(m.textarea.View())
}
