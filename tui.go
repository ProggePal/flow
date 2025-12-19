package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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
)

// --- Model ---

type StepState int

const (
	StatePending StepState = iota
	StateRunning
	StateDone
	StateFailed
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
}

// Messages
type StepStartedMsg struct{ ID string }
type StepDoneMsg struct{ ID string }
type StepFailedMsg struct{ ID string; Err error }
type FlowFinishedMsg struct{ Result string }

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

	return FlowModel{
		Config:           conf,
		FlowName:         flowName,
		ClipboardContent: clipboard,
		InputContent:     input,
		Steps:            steps,
		Spinner:          s,
	}
}

func (m FlowModel) Init() tea.Cmd {
	return m.Spinner.Tick
}

func (m FlowModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			m.Quitting = true
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		return m, cmd
	case StepStartedMsg:
		for _, s := range m.Steps {
			if s.Step.ID == msg.ID {
				s.State = StateRunning
				s.StartTime = time.Now()
			}
		}
	case StepDoneMsg:
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

func (m FlowModel) View() string {
	if m.Err != nil {
		return fmt.Sprintf("\n%s Error: %v\n", crossMark, m.Err)
	}

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

	return "\n" + header + "\n\n" + finalTree + "\n\n" + footer + "\n"
}
