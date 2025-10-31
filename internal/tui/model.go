package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Mode represents the current interaction mode
type Mode int

const (
	ModeNormal Mode = iota
	ModeCommand
	ModeInput
	ModeHelp
	ModeTerminal
)

// InputType represents what kind of input we're collecting
type InputType int

const (
	InputNewFile InputType = iota
	InputNewDir
	InputRename
	InputDelete
	InputCommand
)

// TerminalEntry represents a single command execution in terminal history
type TerminalEntry struct {
	Number  int    // Command number [0], [1], etc.
	Path    string // Directory where command was executed
	Command string // The command that was executed
	Output  string // Command output
	Error   string // Error message if any
}

// Model represents the state of the TUI application
type Model struct {
	// Core components
	adapter *VFSAdapter
	theme   *Theme
	keys    KeyMap
	help    help.Model

	// Navigation state
	currentPath string
	previousDir string // Name of directory we came from (for breadcrumb navigation)
	entries     []*Entry
	cursor      int
	offset      int

	// View state
	width          int
	height         int
	showPreview    bool
	previewContent string
	previewError   error
	previewGen     int // Generation counter to prevent race conditions

	// Mouse state
	lastClickTime int64 // Unix nano timestamp of last click
	lastClickY    int   // Y position of last click
	fileListTop   int   // Y position where file list starts (for click detection)

	// Mode state
	mode      Mode
	inputType InputType
	textInput textinput.Model

	// Status
	statusMsg  string
	errorMsg   string
	commandOut string

	// Terminal
	terminalHistory []*TerminalEntry
	terminalOffset  int // Scroll offset in terminal view
	commandCounter  int // Counter for command numbering

	// Clipboard
	clipboard string

	// Help
	showFullHelp bool
}

// NewModel creates a new TUI model
func NewModel(adapter *VFSAdapter) *Model {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.CharLimit = 256

	return &Model{
		adapter:         adapter,
		theme:           DefaultTheme(),
		keys:            DefaultKeyMap(),
		help:            help.New(),
		currentPath:     "/",
		showPreview:     true,
		textInput:       ti,
		showFullHelp:    false,
		terminalHistory: make([]*TerminalEntry, 0),
		commandCounter:  0,
		terminalOffset:  0,
	}
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadDirectory(),
		textinput.Blink,
	)
}

// Update handles messages and updates the model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		return m, nil

	case directoryLoadedMsg:
		m.entries = msg.entries
		m.errorMsg = ""

		// Position cursor on previous directory if we just navigated back
		if m.previousDir != "" {
			for i, entry := range m.entries {
				if entry.Name == m.previousDir {
					m.cursor = i
					// Adjust offset to keep cursor visible
					visibleLines := m.getVisibleLines()
					if m.cursor >= m.offset+visibleLines {
						m.offset = m.cursor - visibleLines + 1
					} else if m.cursor < m.offset {
						m.offset = m.cursor
					}
					break
				}
			}
			m.previousDir = "" // Clear after using
		} else {
			// Normal navigation - position at top
			if len(m.entries) > 0 && m.cursor >= len(m.entries) {
				m.cursor = len(m.entries) - 1
			}
			if m.cursor < 0 {
				m.cursor = 0
			}
		}
		return m, m.updatePreview()

	case previewLoadedMsg:
		// Only update if this preview is for the current generation
		if msg.generation == m.previewGen {
			m.previewContent = msg.content
			m.previewError = msg.err
		}

		return m, nil

	case commandExecutedMsg:
		m.commandOut = msg.output
		m.errorMsg = msg.error
		m.statusMsg = "Command executed"
		return m, m.loadDirectory()

	case errorMsg:
		m.errorMsg = string(msg)
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.MouseMsg:
		return m.handleMouseEvent(msg)
	}

	// Handle text input updates when in input mode (not terminal, as terminal handles it separately)
	if m.mode == ModeCommand || m.mode == ModeInput {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

// handleKeyPress processes keyboard input based on current mode
func (m *Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle mode-specific inputs
	switch m.mode {
	case ModeCommand, ModeInput:
		return m.handleInputMode(msg)
	case ModeHelp:
		return m.handleHelpMode(msg)
	case ModeTerminal:
		return m.handleTerminalMode(msg)
	case ModeNormal:
		return m.handleNormalMode(msg)
	}

	return m, nil
}

// handleNormalMode processes keys in normal browsing mode
func (m *Model) handleNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case msg.Type == tea.KeyEscape:
		// Clear command output if visible (but only if not in terminal mode)
		if m.commandOut != "" && m.mode != ModeTerminal {
			m.commandOut = ""
			m.errorMsg = ""
			m.statusMsg = ""
			return m, nil
		}

	case key.Matches(msg, m.keys.Help):
		m.mode = ModeHelp
		m.showFullHelp = !m.showFullHelp
		return m, nil

	case key.Matches(msg, m.keys.Up):
		m.moveCursor(-1)
		return m, m.updatePreview()

	case key.Matches(msg, m.keys.Down):
		m.moveCursor(1)
		return m, m.updatePreview()

	case key.Matches(msg, m.keys.PageUp):
		m.moveCursor(-10)
		return m, m.updatePreview()

	case key.Matches(msg, m.keys.PageDown):
		m.moveCursor(10)
		return m, m.updatePreview()

	case key.Matches(msg, m.keys.Top):
		m.cursor = 0
		m.offset = 0
		return m, m.updatePreview()

	case key.Matches(msg, m.keys.Bottom):
		if len(m.entries) > 0 {
			m.cursor = len(m.entries) - 1
		}
		return m, m.updatePreview()

	case key.Matches(msg, m.keys.Enter):
		return m, m.enterDirectory()

	case key.Matches(msg, m.keys.Back):
		return m, m.goBack()

	case key.Matches(msg, m.keys.TogglePreview):
		m.showPreview = !m.showPreview
		return m, nil

	case key.Matches(msg, m.keys.Refresh):
		return m, m.loadDirectory()

	case key.Matches(msg, m.keys.NewFile):
		m.startInput(InputNewFile, "New file name:")
		return m, nil

	case key.Matches(msg, m.keys.NewDir):
		m.startInput(InputNewDir, "New directory name:")
		return m, nil

	case key.Matches(msg, m.keys.Delete):
		if m.currentEntry() != nil {
			m.startInput(InputDelete, fmt.Sprintf("Delete %s? (y/n):", m.currentEntry().Name))
		}
		return m, nil

	case key.Matches(msg, m.keys.Rename):
		if m.currentEntry() != nil {
			m.startInput(InputRename, "New name:")
			m.textInput.SetValue(m.currentEntry().Name)
		}
		return m, nil

	case key.Matches(msg, m.keys.Copy):
		if entry := m.currentEntry(); entry != nil {
			m.clipboard = entry.Path
			m.statusMsg = fmt.Sprintf("Copied: %s", entry.Name)
		}
		return m, nil

	case key.Matches(msg, m.keys.Command):
		// Toggle between Navigation and Terminal modes
		if m.mode == ModeTerminal {
			// Switch back to Navigation mode
			m.mode = ModeNormal
			m.textInput.Blur()
		} else {
			// Switch to Terminal mode
			m.mode = ModeTerminal
			m.textInput.Placeholder = "" // No placeholder in terminal mode
			m.textInput.SetValue("")
			m.textInput.Focus()
			// Reset scroll to bottom when entering terminal
			m.terminalOffset = 0
		}
		return m, nil
	}

	return m, nil
}

// handleInputMode processes keys when collecting user input
func (m *Model) handleInputMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		// If in terminal mode, just blur but stay in terminal
		if m.mode == ModeCommand {
			m.textInput.Blur()
			m.mode = ModeNormal
			return m, nil
		}
		m.cancelInput()
		return m, nil

	case tea.KeyEnter:
		return m, m.submitInput()
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// handleHelpMode processes keys in help mode
func (m *Model) handleHelpMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Help), key.Matches(msg, m.keys.Quit):
		m.mode = ModeNormal
		return m, nil
	}
	return m, nil
}

// handleTerminalMode processes keys in terminal mode
func (m *Model) handleTerminalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Command):
		// Toggle back to navigation mode
		m.mode = ModeNormal
		m.textInput.Blur()
		return m, nil

	case key.Matches(msg, m.keys.Up):
		// Scroll up in terminal history
		if m.terminalOffset < len(m.terminalHistory)*3 { // Rough estimate of lines per entry
			m.terminalOffset++
		}
		return m, nil

	case key.Matches(msg, m.keys.Down):
		// Scroll down in terminal history
		if m.terminalOffset > 0 {
			m.terminalOffset--
		}
		return m, nil

	case key.Matches(msg, m.keys.PageUp):
		// Scroll up page in terminal history
		m.terminalOffset += 10
		return m, nil

	case key.Matches(msg, m.keys.PageDown):
		// Scroll down page in terminal history
		m.terminalOffset -= 10
		if m.terminalOffset < 0 {
			m.terminalOffset = 0
		}
		return m, nil

	case msg.Type == tea.KeyEnter:
		// Execute command
		return m, m.submitTerminalCommand()

	case msg.Type == tea.KeyEscape:
		// Return to navigation mode
		m.mode = ModeNormal
		m.textInput.Blur()
		return m, nil
	}

	// Let text input handle all other keys (typing letters, backspace, etc.)
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// handleMouseEvent processes mouse input
func (m *Model) handleMouseEvent(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Only handle mouse in normal mode
	if m.mode != ModeNormal {
		return m, nil
	}

	// Handle scroll wheel
	if msg.Action == tea.MouseActionPress {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			// Scroll up (move cursor up)
			m.moveCursor(-1)
			return m, m.updatePreview()

		case tea.MouseButtonWheelDown:
			// Scroll down (move cursor down)
			m.moveCursor(1)
			return m, m.updatePreview()

		case tea.MouseButtonLeft:
			// Calculate which file entry was clicked
			// Title bar (1 line) + border top (1 line) = 2 lines before first entry
			m.fileListTop = 2

			if msg.Y < m.fileListTop {
				// Click was in title area, ignore
				return m, nil
			}

			// Calculate which entry was clicked
			clickedLine := msg.Y - m.fileListTop
			clickedIndex := m.offset + clickedLine

			if clickedIndex >= 0 && clickedIndex < len(m.entries) {
				// Check for double-click (within 500ms and same position)
				now := time.Now().UnixNano()
				doubleClickThreshold := int64(500 * time.Millisecond)

				isDoubleClick := (now-m.lastClickTime) < doubleClickThreshold &&
					msg.Y == m.lastClickY &&
					clickedIndex == m.cursor

				m.lastClickTime = now
				m.lastClickY = msg.Y

				if isDoubleClick {
					// Double-click: enter directory
					m.cursor = clickedIndex
					return m, m.enterDirectory()
				} else {
					// Single click: select item
					m.cursor = clickedIndex
					return m, m.updatePreview()
				}
			}

		case tea.MouseButtonRight:
			// Right-click: navigate back to parent directory
			return m, m.goBack()
		}
	}

	return m, nil
}

// startInput enters input mode with the specified type and prompt
func (m *Model) startInput(inputType InputType, prompt string) {
	m.mode = ModeInput
	m.inputType = inputType
	m.textInput.Placeholder = prompt
	m.textInput.SetValue("")
	m.textInput.Focus()
	m.errorMsg = ""
	m.statusMsg = ""

	if inputType == InputCommand {
		m.mode = ModeCommand
	}
}

// cancelInput exits input mode without taking action
func (m *Model) cancelInput() {
	m.mode = ModeNormal
	m.textInput.Blur()
	m.textInput.SetValue("")
}

// submitInput processes the collected input
func (m *Model) submitInput() tea.Cmd {
	value := strings.TrimSpace(m.textInput.Value())

	// For command mode, keep terminal open and just clear input
	if m.inputType == InputCommand {
		m.textInput.SetValue("")
		if value == "" {
			return nil
		}
		return m.executeCommand(value)
	}

	// For other input types, close the input
	m.cancelInput()

	if value == "" {
		return nil
	}

	switch m.inputType {
	case InputNewFile:
		return m.createFile(value)
	case InputNewDir:
		return m.createDirectory(value)
	case InputRename:
		return m.renameEntry(value)
	case InputDelete:
		if strings.ToLower(value) == "y" || strings.ToLower(value) == "yes" {
			return m.deleteEntry()
		}
		return nil
	}

	return nil
}

// moveCursor moves the cursor by delta, handling bounds and scrolling
func (m *Model) moveCursor(delta int) {
	if len(m.entries) == 0 {
		return
	}

	m.cursor += delta

	// Clamp cursor
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.entries) {
		m.cursor = len(m.entries) - 1
	}

	// Adjust offset for scrolling
	visibleLines := m.getVisibleLines()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visibleLines {
		m.offset = m.cursor - visibleLines + 1
	}
}

// getVisibleLines returns how many file entries can be displayed
func (m *Model) getVisibleLines() int {
	// Reserve space for title, status bar, help, and padding
	reserved := 8

	available := m.height - reserved
	if available < 5 {
		return 5
	}
	return available
}

// currentEntry returns the currently selected entry
func (m *Model) currentEntry() *Entry {
	if m.cursor >= 0 && m.cursor < len(m.entries) {
		return m.entries[m.cursor]
	}
	return nil
}

// Messages for async operations
type directoryLoadedMsg struct {
	entries []*Entry
}

type previewLoadedMsg struct {
	content    string
	err        error
	generation int // Which preview request this is for
}

type commandExecutedMsg struct {
	output string
	error  string
}

type errorMsg string

// Commands for async operations
func (m *Model) loadDirectory() tea.Cmd {
	return func() tea.Msg {
		entries, err := m.adapter.ListDirectory(m.currentPath)
		if err != nil {
			return errorMsg(fmt.Sprintf("Failed to load directory: %v", err))
		}
		return directoryLoadedMsg{entries: entries}
	}
}

func (m *Model) updatePreview() tea.Cmd {
	if !m.showPreview {
		return nil
	}

	entry := m.currentEntry()

	// Increment generation counter for new preview
	m.previewGen++
	currentGen := m.previewGen

	if entry == nil || entry.IsDir {
		return func() tea.Msg {
			return previewLoadedMsg{content: "", err: nil, generation: currentGen}
		}
	}

	// Capture entry path to prevent race
	entryPath := entry.Path

	return func() tea.Msg {
		// Calculate available space for preview
		previewWidth := m.width / 2
		previewHeight := m.height - 10

		// Use new preview system that handles different file types
		content, err := m.adapter.GeneratePreview(entryPath, previewWidth, previewHeight)

		return previewLoadedMsg{content: content, err: err, generation: currentGen}
	}
}

func (m *Model) enterDirectory() tea.Cmd {
	entry := m.currentEntry()
	if entry == nil {
		return nil
	}

	if !entry.IsDir {
		m.statusMsg = fmt.Sprintf("Cannot open file: %s", entry.Name)
		return nil
	}

	m.currentPath = entry.Path
	m.previousDir = "" // Clear previous directory when entering new one
	m.cursor = 0
	m.offset = 0

	return m.loadDirectory()
}

func (m *Model) goBack() tea.Cmd {
	if m.currentPath == "/" {
		return nil
	}

	// Remember which directory we're leaving so we can position cursor on it
	m.previousDir = filepath.Base(m.currentPath)

	m.currentPath = filepath.Dir(m.currentPath)
	m.cursor = 0
	m.offset = 0
	return m.loadDirectory()
}

func (m *Model) createFile(name string) tea.Cmd {
	return func() tea.Msg {
		path := filepath.Join(m.currentPath, name)
		if err := m.adapter.CreateFile(path); err != nil {
			return errorMsg(fmt.Sprintf("Failed to create file: %v", err))
		}
		return m.loadDirectory()()
	}
}

func (m *Model) createDirectory(name string) tea.Cmd {
	return func() tea.Msg {
		path := filepath.Join(m.currentPath, name)
		if err := m.adapter.CreateDirectory(path); err != nil {
			return errorMsg(fmt.Sprintf("Failed to create directory: %v", err))
		}
		return m.loadDirectory()()
	}
}

func (m *Model) deleteEntry() tea.Cmd {
	entry := m.currentEntry()
	if entry == nil {
		return nil
	}

	return func() tea.Msg {
		var err error
		if entry.IsDir {
			err = m.adapter.DeleteRecursive(entry.Path)
		} else {
			err = m.adapter.Delete(entry.Path, false)
		}

		if err != nil {
			return errorMsg(fmt.Sprintf("Failed to delete: %v", err))
		}
		return m.loadDirectory()()
	}
}

func (m *Model) renameEntry(newName string) tea.Cmd {
	entry := m.currentEntry()
	if entry == nil {
		return nil
	}

	return func() tea.Msg {
		// For now, we'll implement rename as copy + delete
		// since VFS doesn't have Rename implemented yet
		newPath := filepath.Join(m.currentPath, newName)

		if !entry.IsDir {
			if err := m.adapter.CopyFile(entry.Path, newPath); err != nil {
				return errorMsg(fmt.Sprintf("Failed to rename: %v", err))
			}
			if err := m.adapter.Delete(entry.Path, false); err != nil {
				return errorMsg(fmt.Sprintf("Failed to remove old file: %v", err))
			}
		} else {
			return errorMsg("Directory rename not yet supported")
		}

		return m.loadDirectory()()
	}
}

// submitTerminalCommand executes a command in terminal mode
func (m *Model) submitTerminalCommand() tea.Cmd {
	cmdLine := strings.TrimSpace(m.textInput.Value())
	m.textInput.SetValue("")

	if cmdLine == "" {
		return nil
	}

	// Create terminal entry with current path
	entry := &TerminalEntry{
		Number:  m.commandCounter,
		Path:    m.currentPath,
		Command: cmdLine,
	}

	// Add to history immediately (will be updated with output)
	m.terminalHistory = append(m.terminalHistory, entry)
	m.commandCounter++

	return func() tea.Msg {
		// Parse command line
		args := parseCommandLine(cmdLine)
		if len(args) == 0 {
			return commandExecutedMsg{output: "", error: ""}
		}

		// Create a buffer to capture command output
		var buf strings.Builder

		exitCode, err := m.adapter.vfs.Execute(m.adapter.ctx, &buf, args...)

		// Get the captured output
		output := buf.String()
		errStr := ""

		if err != nil {
			errStr = err.Error()
		}

		if exitCode != 0 && errStr == "" {
			errStr = fmt.Sprintf("Command exited with code %d", exitCode)
		}

		// Update the entry with output
		entry.Output = output
		entry.Error = errStr

		return commandExecutedMsg{
			output: output,
			error:  errStr,
		}
	}
}

func (m *Model) executeCommand(cmdLine string) tea.Cmd {
	return func() tea.Msg {
		// Parse command line
		args := parseCommandLine(cmdLine)
		if len(args) == 0 {
			return commandExecutedMsg{output: "", error: ""}
		}

		output := ""
		errStr := ""

		// Create a buffer to capture command output
		var buf strings.Builder

		exitCode, err := m.adapter.vfs.Execute(m.adapter.ctx, &buf, args...)

		// Get the captured output
		output = buf.String()

		if err != nil {
			errStr = err.Error()
		}

		if exitCode != 0 && errStr == "" {
			errStr = fmt.Sprintf("Command exited with code %d", exitCode)
		}

		return commandExecutedMsg{
			output: output,
			error:  errStr,
		}
	}
}

// parseCommandLine splits a command line into tokens (same as original main.go)
func parseCommandLine(line string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, ch := range line {
		switch {
		case ch == '"' || ch == '\'':
			if inQuote {
				if ch == quoteChar {
					inQuote = false
					quoteChar = 0
				} else {
					current.WriteRune(ch)
				}
			} else {
				inQuote = true
				quoteChar = ch
			}

		case ch == ' ' && !inQuote:
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}

		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}
