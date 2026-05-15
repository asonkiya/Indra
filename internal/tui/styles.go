package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Sidebar
	sidebarStyle = lipgloss.NewStyle().
			Width(30).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderRight(true).
			Padding(0, 1)

	// Chat area
	chatAreaStyle = lipgloss.NewStyle().
			Padding(0, 1)

	// Sent messages (right-aligned)
	sentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#1a1a2e", Dark: "#a8dadc"}).
			Align(lipgloss.Right).
			Padding(0, 1)

	// Received messages (left-aligned)
	receivedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#333", Dark: "#e9c46a"}).
			Align(lipgloss.Left).
			Padding(0, 1)

	// Timestamp
	timestampStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#999", Dark: "#666"}).
			Italic(true)

	// Status bar at the bottom
	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#fff", Dark: "#fff"}).
			Background(lipgloss.AdaptiveColor{Light: "#2a9d8f", Dark: "#264653"}).
			Padding(0, 1)

	// Selected list item
	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#fff", Dark: "#fff"}).
				Background(lipgloss.AdaptiveColor{Light: "#2a9d8f", Dark: "#264653"}).
				Padding(0, 1)

	normalItemStyle = lipgloss.NewStyle().
			Padding(0, 1)

	// Compose box
	composeStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderTop(true).
			Padding(0, 1)
)
