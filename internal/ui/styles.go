package ui

import "github.com/charmbracelet/lipgloss"

var (
	StyleDenied  = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	StyleAllowed = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))
	StyleMeta    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)
