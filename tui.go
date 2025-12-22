package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"
)

// --- Styles ---

var (
	subtleStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	titleStyle      = lipgloss.NewStyle().Bold(true)
	checkMark       = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).SetString("✓")
	crossMark       = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).SetString("✕")
	waitMark        = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).SetString("•")
	runningStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	enumeratorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginRight(1)
	rootStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	itemStyle       = lipgloss.NewStyle()
	timerStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginLeft(1)
	
	// Chat Styles
	senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true)
	aiStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	toolStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
)

// --- Model ---

type StepState int

const (
	StatePending StepState = iota
	StateRunning
	StateDone
	StateFailed
	StateWaiting
)

type StepStatus struct {
	Step      Step
	State     StepState
	Err       error
	ParentID  string
	StartTime time.Time
	Duration  time.Duration
}

type FlowModel struct {
	Config           Config
	FlowName         string
	ClipboardContent string
	InputContent     string
	Steps            []*StepStatus
	Spinner          spinner.Model
	Quitting         bool
	Result           string
	Err              error
	
	// Chat Components
	Input            textarea.Model
	InputMode        bool
	CurrentStepID    string
	ChatHistory      []string // Raw markdown strings
	Viewport         viewport.Model
	Renderer         *glamour.TermRenderer
	
	// Selector Components
	List             list.Model
	ListMode         bool

	// Flow Editor Components
	FlowEditor       FlowEditorModel
	EditorMode       bool
}

// Messages
type StepStartedMsg struct{ ID string }
type StepDoneMsg struct{ ID string }
type StepFailedMsg struct{ ID string; Err error }
type FlowFinishedMsg struct{ Result string }
type StepInteractionRequiredMsg struct{ ID string }
type StepInteractionOutputMsg struct {
	ID     string
	Output string
}

type StepStreamStartMsg struct {
	ID string
}

type StepStreamMsg struct {
	ID    string
	Chunk string
}

type StepToolCallMsg struct {
	ID   string
	Name string
	Args map[string]interface{}
}

type StepToolResultMsg struct {
	ID      string
	Name    string
	Success bool
	Result  string
}

type FileInfo struct {
	Name string
	Desc string
}

type StepSelectorRequiredMsg struct {
	ID     string
	Files  []FileInfo
	Prompt string
}

type StepFlowEditorRequiredMsg struct {
	ID          string
	JSONContent string
}

// List Item
type fileItem struct {
	name, desc string
}

func (i fileItem) FilterValue() string { return i.name }
func (i fileItem) Title() string       { return i.name }
func (i fileItem) Description() string { return i.desc }

func InitialModel(conf Config, flowName, clipboard, input string) FlowModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	steps := make([]*StepStatus, len(conf.Steps))
	for i, step := range conf.Steps {
		// Find parent
		tags := regexp.MustCompile(`{{(.*?)}}`).FindAllStringSubmatch(step.Prompt, -1)
		parent := "root"
		
		// 1. Check for step dependencies (strongest link)
		for _, tag := range tags {
			dep := tag[1]
			if dep != "clipboard" && dep != "input" {
				parent = dep
				break
			}
		}

		// 2. If no step dependency, check for inputs
		if parent == "root" {
			for _, tag := range tags {
				if tag[1] == "clipboard" {
					parent = "clipboard"
					break
				}
				if tag[1] == "input" {
					parent = "input"
					break
				}
			}
		}

		steps[i] = &StepStatus{Step: step, State: StatePending, ParentID: parent}
	}

	ta := textarea.New()
	ta.Placeholder = "Type your message... (Ctrl+S to send, Esc to end chat)"
	ta.Focus()
	ta.CharLimit = 0 // Unlimited
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	vp := viewport.New(80, 20)
	vp.SetContent("Welcome to Fast Flow Chat.\n")

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)

	return FlowModel{
		Config:           conf,
		FlowName:         flowName,
		ClipboardContent: clipboard,
		InputContent:     input,
		Steps:            steps,
		Spinner:          s,
		Input:            ta,
		Viewport:         vp,
		Renderer:         renderer,
		ChatHistory:      []string{},
	}
}

func (m FlowModel) Init() tea.Cmd {
	return tea.Batch(m.Spinner.Tick, textarea.Blink)
}

func (m FlowModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Viewport.Width = msg.Width
		m.Viewport.Height = msg.Height - 10 // Leave room for header/footer/input
		m.Input.SetWidth(msg.Width)
		
		if m.EditorMode {
			m.FlowEditor, cmd = m.FlowEditor.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.MouseMsg:
		if m.InputMode {
			var vpCmd tea.Cmd
			m.Viewport, vpCmd = m.Viewport.Update(msg)
			return m, vpCmd
		}

	case tea.KeyMsg:
		if m.EditorMode {
			// Handle Editor Mode Keys
			switch msg.Type {
			case tea.KeyCtrlS:
				if !m.FlowEditor.Editing {
					// Save Flow and Exit Editor
					// Serialize config back to JSON
					bytes, _ := json.MarshalIndent(m.FlowEditor.Config, "", "  ")
					val := string(bytes)
					
					// Set state back to Running
					for _, s := range m.Steps {
						if s.Step.ID == m.CurrentStepID {
							s.State = StateRunning
						}
					}
					go func() { inputChan <- val }()
					m.EditorMode = false
					return m, nil
				}
			case tea.KeyEsc:
				if !m.FlowEditor.Editing {
					// Cancel Editor
					go func() { inputChan <- "no" }()
					m.EditorMode = false
					return m, nil
				}
			}
			m.FlowEditor, cmd = m.FlowEditor.Update(msg)
			return m, cmd
		}

		if m.ListMode {
			switch msg.Type {
			case tea.KeyEnter:
				i, ok := m.List.SelectedItem().(fileItem)
				if ok {
					val := i.name
					// Set state back to Running
					for _, s := range m.Steps {
						if s.Step.ID == m.CurrentStepID {
							s.State = StateRunning
						}
					}
					go func() { inputChan <- val }()
					return m, nil
				}
			case tea.KeyCtrlC:
				m.Quitting = true
				return m, tea.Quit
			}
			m.List, cmd = m.List.Update(msg)
			return m, cmd
		}

		if m.InputMode {
			// Handle scrolling for viewport
			var vpCmd tea.Cmd
			switch msg.Type {
			case tea.KeyPgUp, tea.KeyPgDown:
				m.Viewport, vpCmd = m.Viewport.Update(msg)
			case tea.KeyCtrlUp:
				m.Viewport.LineUp(1)
			case tea.KeyCtrlDown:
				m.Viewport.LineDown(1)
			}

			switch msg.Type {
			case tea.KeyCtrlS: // Send message
				val := m.Input.Value()
				if strings.TrimSpace(val) == "" {
					return m, nil
				}
				m.Input.Reset()
				
				// Add to history
				userMsg := fmt.Sprintf("**You**\n%s", val)
				m.ChatHistory = append(m.ChatHistory, userMsg)
				m.updateViewport()
				
				// Set state back to Running (Thinking)
				for _, s := range m.Steps {
					if s.Step.ID == m.CurrentStepID {
						s.State = StateRunning
					}
				}

				// Send to channel in a goroutine to avoid blocking Update
				go func() { inputChan <- val }()
				return m, nil
			case tea.KeyEsc:
				// Finish interaction
				go func() { inputChan <- "__END_INTERACTION__" }()
				return m, nil
			case tea.KeyCtrlC:
				m.Quitting = true
				return m, tea.Quit
			}
			m.Input, cmd = m.Input.Update(msg)
			return m, tea.Batch(cmd, vpCmd)
		}

		if msg.String() == "q" || msg.String() == "ctrl+c" {
			m.Quitting = true
			return m, tea.Quit
		}
	case spinner.TickMsg:
		m.Spinner, cmd = m.Spinner.Update(msg)
		return m, cmd
	case StepStartedMsg:
		for _, s := range m.Steps {
			if s.Step.ID == msg.ID {
				s.State = StateRunning
				s.StartTime = time.Now()
			}
		}
	case StepInteractionRequiredMsg:
		m.InputMode = true
		m.CurrentStepID = msg.ID
		for _, s := range m.Steps {
			if s.Step.ID == msg.ID {
				s.State = StateWaiting
			}
		}
		return m, textarea.Blink
	case StepInteractionOutputMsg:
		// Add AI response to history
		aiMsg := fmt.Sprintf("**AI**\n%s", msg.Output)
		m.ChatHistory = append(m.ChatHistory, aiMsg)
		m.updateViewport()
		return m, nil
	case StepStreamStartMsg:
		// Start a new AI message
		m.ChatHistory = append(m.ChatHistory, "**AI**\n")
		m.updateViewport()
		return m, nil
	case StepStreamMsg:
		// Append to the last message
		if len(m.ChatHistory) > 0 {
			m.ChatHistory[len(m.ChatHistory)-1] += msg.Chunk
			m.updateViewport()
		}
		return m, nil
	case StepToolCallMsg:
		// Add tool call to history
		toolMsg := fmt.Sprintf("🤖 Calling tool: `%s`...", msg.Name)
		m.ChatHistory = append(m.ChatHistory, toolMsg)
		m.updateViewport()
		return m, nil
	case StepToolResultMsg:
		// Add tool result to history
		var icon string
		if msg.Success {
			icon = "✅"
		} else {
			icon = "❌"
		}
		toolMsg := fmt.Sprintf("%s Tool `%s` finished", icon, msg.Name)
		m.ChatHistory = append(m.ChatHistory, toolMsg)
		m.updateViewport()
		return m, nil
	case StepSelectorRequiredMsg:
		m.ListMode = true
		m.CurrentStepID = msg.ID
		for _, s := range m.Steps {
			if s.Step.ID == msg.ID {
				s.State = StateWaiting
			}
		}
		
		items := make([]list.Item, len(msg.Files))
		for i, f := range msg.Files {
			items[i] = fileItem{name: f.Name, desc: f.Desc}
		}
		
		l := list.New(items, list.NewDefaultDelegate(), 60, 14)
		l.Title = msg.Prompt
		l.SetShowHelp(false)
		m.List = l
		return m, nil
	case StepFlowEditorRequiredMsg:
		m.EditorMode = true
		m.CurrentStepID = msg.ID
		for _, s := range m.Steps {
			if s.Step.ID == msg.ID {
				s.State = StateWaiting
			}
		}
		m.FlowEditor = NewFlowEditor(msg.JSONContent)
		// Trigger resize to set dimensions
		m.FlowEditor, _ = m.FlowEditor.Update(tea.WindowSizeMsg{Width: m.Viewport.Width, Height: m.Viewport.Height + 10})
		return m, nil
	case StepDoneMsg:
		m.InputMode = false
		m.ListMode = false
		m.EditorMode = false
		for _, s := range m.Steps {
			if s.Step.ID == msg.ID {
				s.State = StateDone
				s.Duration = time.Since(s.StartTime)
			}
		}
	case StepFailedMsg:
		for _, s := range m.Steps {
			if s.Step.ID == msg.ID {
				s.State = StateFailed
				s.Err = msg.Err
				s.Duration = time.Since(s.StartTime)
			}
		}
		m.Err = msg.Err
		m.Quitting = true
		return m, tea.Quit
	case FlowFinishedMsg:
		m.Result = msg.Result
		m.Quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *FlowModel) updateViewport() {
	// Render all history with glamour
	fullContent := strings.Join(m.ChatHistory, "\n\n---\n\n")
	rendered, err := m.Renderer.Render(fullContent)
	if err != nil {
		rendered = fullContent // Fallback to raw text
	}
	m.Viewport.SetContent(rendered)
	m.Viewport.GotoBottom()
}

func (m FlowModel) View() string {
	if m.Err != nil {
		return fmt.Sprintf("\n%s Error: %v\n", crossMark, m.Err)
	}

	// --- CHAT MODE ---
	if m.InputMode {
		header := titleStyle.Render(fmt.Sprintf("Chat: %s", m.CurrentStepID))
		return fmt.Sprintf(
			"\n%s\n\n%s\n\n%s\n%s",
			header,
			m.Viewport.View(),
			m.Input.View(),
			subtleStyle.Render("Ctrl+S to send • Esc to finish chat"),
		)
	}

	// --- EDITOR MODE ---
	if m.EditorMode {
		return "\n" + m.FlowEditor.View() + "\n" + subtleStyle.Render("Enter to edit • Ctrl+S to save step/flow • Esc to cancel edit")
	}

	// --- SELECTOR MODE ---
	if m.ListMode {
		return "\n" + lipgloss.NewStyle().Margin(1, 2).Render(m.List.View())
	}

	// --- PROGRESS MODE (Tree View) ---
	var headerIcon string
	if m.Result != "" {
		headerIcon = checkMark.String()
	} else {
		headerIcon = m.Spinner.View()
	}

	header := fmt.Sprintf("%s %s", headerIcon, titleStyle.Render(fmt.Sprintf("Flow: %s", m.FlowName))) + subtleStyle.Render(fmt.Sprintf(" | Model: %s", m.Config.Model))

	t := tree.Root("Inputs").
		Enumerator(tree.RoundedEnumerator).
		EnumeratorStyle(enumeratorStyle).
		RootStyle(rootStyle).
		ItemStyle(itemStyle)

	// Check for input usage
	hasClipboard := false
	hasInput := false
	for _, s := range m.Steps {
		if s.ParentID == "clipboard" {
			hasClipboard = true
		}
		if s.ParentID == "input" {
			hasInput = true
		}
	}

	var clipboardTree *tree.Tree
	if hasClipboard {
		label := "Clipboard"
		if len(m.ClipboardContent) > 0 {
			preview := strings.ReplaceAll(m.ClipboardContent, "\n", " ")
			if len(preview) > 30 {
				preview = preview[:30] + "..."
			}
			label = fmt.Sprintf("Clipboard: %s", preview)
		}
		clipboardTree = tree.Root(label).
			Enumerator(tree.RoundedEnumerator).
			EnumeratorStyle(enumeratorStyle).
			ItemStyle(itemStyle)
	}

	var inputTree *tree.Tree
	if hasInput {
		label := "Input"
		if len(m.InputContent) > 0 {
			preview := strings.ReplaceAll(m.InputContent, "\n", " ")
			if len(preview) > 30 {
				preview = preview[:30] + "..."
			}
			label = fmt.Sprintf("Input: %s", preview)
		}
		inputTree = tree.Root(label).
			Enumerator(tree.RoundedEnumerator).
			EnumeratorStyle(enumeratorStyle).
			ItemStyle(itemStyle)
	}

	// Helper to recursively add children
	var addChildren func(parentID string, currentTree *tree.Tree)
	addChildren = func(parentID string, currentTree *tree.Tree) {
		for _, s := range m.Steps {
			if s.ParentID == parentID {
				// Determine icon and style
				var icon string
				var style lipgloss.Style
				var timer string

				switch s.State {
				case StateRunning:
					icon = ""
					style = runningStyle
					timer = timerStyle.Render(fmt.Sprintf("%.1fs", time.Since(s.StartTime).Seconds()))
				case StateWaiting:
					icon = "➜"
					style = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
					timer = timerStyle.Render("Awaits User")
				case StateDone:
					icon = "" // No checkmark in tree
					style = itemStyle
					timer = timerStyle.Render(fmt.Sprintf("%.1fs", s.Duration.Seconds()))
				case StateFailed:
					icon = crossMark.String()
					style = itemStyle
					timer = timerStyle.Render(fmt.Sprintf("%.1fs", s.Duration.Seconds()))
				default:
					icon = waitMark.String()
					style = subtleStyle
				}

				label := fmt.Sprintf("%s %s%s", icon, style.Render(s.Step.ID), timer)
				
				// Check if this node has children
				hasChildren := false
				for _, check := range m.Steps {
					if check.ParentID == s.Step.ID {
						hasChildren = true
						break
					}
				}

				if hasChildren {
					subTree := tree.Root(label).
						Enumerator(tree.RoundedEnumerator).
						EnumeratorStyle(enumeratorStyle).
						ItemStyle(itemStyle)
					addChildren(s.Step.ID, subTree)
					currentTree.Child(subTree)
				} else {
					currentTree.Child(label)
				}
			}
		}
	}

	if hasClipboard {
		addChildren("clipboard", clipboardTree)
		t.Child(clipboardTree)
	}
	if hasInput {
		addChildren("input", inputTree)
		t.Child(inputTree)
	}
	addChildren("root", t)

	var finalTree string
	if hasClipboard && !hasInput && t.Children().Length() == 1 {
		finalTree = clipboardTree.String()
	} else if !hasClipboard && hasInput && t.Children().Length() == 1 {
		finalTree = inputTree.String()
	} else {
		finalTree = t.String()
	}

	footer := subtleStyle.Render("Press q to quit")
	if m.Result != "" {
		footer = fmt.Sprintf("%s %s", checkMark.String(), subtleStyle.Render("Flow Complete! (Result copied to clipboard)"))
	}

	view := "\n" + header + "\n\n" + finalTree + "\n\n" + footer + "\n"

	return view
}
