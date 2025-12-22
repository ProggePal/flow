package main

import (
	"encoding/json"
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

var (
	editorListStyle = lipgloss.NewStyle().
			Width(30).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("241")).
			MarginRight(1)

	editorDetailStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("241")).
				Padding(0, 1)

	editorTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).
				Bold(true).
				MarginBottom(1)
)

type stepItem struct {
	step  Step
	index int
}

func (i stepItem) Title() string       { return i.step.ID }
func (i stepItem) Description() string { return fmt.Sprintf("%s • %s", i.step.Type, i.step.Model) }
func (i stepItem) FilterValue() string { return i.step.ID }

type FlowEditorModel struct {
	Config      Config
	List        list.Model
	Viewport    viewport.Model
	TextArea    textarea.Model
	Renderer    *glamour.TermRenderer
	
	Active      bool
	Editing     bool // True if editing a specific step's prompt
	SelectedIdx int
	
	Width       int
	Height      int
	Err         error
}

func NewFlowEditor(jsonContent string) FlowEditorModel {
	var conf Config
	// Try to clean markdown if present
	clean := cleanMarkdown(jsonContent)
	err := json.Unmarshal([]byte(clean), &conf)

	items := []list.Item{}
	if err == nil {
		for i, s := range conf.Steps {
			items = append(items, stepItem{step: s, index: i})
		}
	}

	// Setup List
	l := list.New(items, list.NewDefaultDelegate(), 30, 20)
	l.Title = "Flow Steps"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)

	// Setup Viewport
	vp := viewport.New(0, 0)

	// Setup TextArea
	ta := textarea.New()
	ta.Placeholder = "Enter step prompt..."
	ta.ShowLineNumbers = true

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)

	m := FlowEditorModel{
		Config:   conf,
		List:     l,
		Viewport: vp,
		TextArea: ta,
		Renderer: renderer,
		Err:      err,
	}
	
	// Select first item and update viewport
	if len(items) > 0 {
		m.List.Select(0)
		m.updateViewport()
	}

	return m
}

func (m *FlowEditorModel) updateViewport() {
	if len(m.Config.Steps) == 0 {
		return
	}
	
	idx := m.List.Index()
	if idx < 0 || idx >= len(m.Config.Steps) {
		return
	}
	
	step := m.Config.Steps[idx]
	m.SelectedIdx = idx

	content := fmt.Sprintf("# %s\n\n**Type:** %s\n**Model:** %s\n\n---\n\n%s", 
		step.ID, step.Type, step.Model, step.Prompt)
	
	if step.Type == "file_write" {
		content += fmt.Sprintf("\n\n**Filename:** `%s`\n\n```\n%s\n```", step.Filename, step.Content)
	}

	rendered, err := m.Renderer.Render(content)
	if err != nil {
		rendered = content
	}
	
	m.Viewport.SetContent(rendered)
}

func (m FlowEditorModel) Init() tea.Cmd {
	return nil
}

func (m FlowEditorModel) Update(msg tea.Msg) (FlowEditorModel, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	// Handle resizing
	if msg, ok := msg.(tea.WindowSizeMsg); ok {
		m.Width = msg.Width
		m.Height = msg.Height
		
		listWidth := 30
		detailWidth := msg.Width - listWidth - 6 // Borders and margins
		
		m.List.SetHeight(msg.Height - 4)
		
		m.Viewport.Width = detailWidth
		m.Viewport.Height = msg.Height - 4
		
		m.TextArea.SetWidth(detailWidth)
		m.TextArea.SetHeight(msg.Height - 4)
		
		m.updateViewport()
		return m, nil
	}

	if m.Editing {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEsc:
				m.Editing = false
				return m, nil
			case tea.KeyCtrlS:
				// Save changes to step
				val := m.TextArea.Value()
				if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Config.Steps) {
					// For file_write, we might be editing content. For now assume Prompt.
					// TODO: Support editing other fields.
					m.Config.Steps[m.SelectedIdx].Prompt = val
					
					// Update list item description if needed
					// m.List.SetItem(m.SelectedIdx, stepItem{step: m.Config.Steps[m.SelectedIdx], index: m.SelectedIdx})
				}
				m.Editing = false
				m.updateViewport()
				return m, nil
			}
		}
		m.TextArea, cmd = m.TextArea.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	// Navigation Mode
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			// Start Editing
			if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Config.Steps) {
				m.Editing = true
				m.TextArea.SetValue(m.Config.Steps[m.SelectedIdx].Prompt)
				m.TextArea.Focus()
				return m, textarea.Blink
			}
		case tea.KeyCtrlS:
			// Save Flow (Return JSON)
			// Handled by parent, but we can signal here if needed
		}
	}

	// Update List
	newList, cmd := m.List.Update(msg)
	m.List = newList
	cmds = append(cmds, cmd)
	
	// Update Viewport if selection changed
	if m.List.Index() != m.SelectedIdx {
		m.updateViewport()
	}
	
	// Update Viewport scrolling
	m.Viewport, cmd = m.Viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m FlowEditorModel) View() string {
	if m.Err != nil {
		return fmt.Sprintf("Error parsing flow: %v", m.Err)
	}

	if m.Editing {
		return editorDetailStyle.Render(m.TextArea.View())
	}

	listView := editorListStyle.Render(m.List.View())
	detailView := editorDetailStyle.Render(m.Viewport.View())

	return lipgloss.JoinHorizontal(lipgloss.Top, listView, detailView)
}
