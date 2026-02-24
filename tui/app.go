package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6600")).
			Background(lipgloss.Color("#1a1a2e")).
			Padding(0, 2)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#333366"))

	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6600")).
			Underline(true).
			Padding(0, 1)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888888")).
				Padding(0, 1)

	selectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FF6600"))

	normalRowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CCCCCC"))

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#AAAAFF")).
			Underline(true)

	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAA00"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	boldStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	outputStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#88CCFF"))
	cardStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#444466")).Padding(0, 1)
	gaugeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#00CCFF"))
)

type tab int

const (
	tabDashboard tab = iota
	tabSandboxes
	tabTemplates
	tabProviders
	tabLogs
)

type mode int

const (
	modeNormal mode = iota
	modeInput
	modeConfirm
	modeExec
	modeSpawn
	modeSpawnTemplate
	modeCreateTemplate
)

// Messages
type tickMsg time.Time
type sandboxesMsg []sandboxData
type healthMsg *healthData
type providersMsg []providerData
type templatesMsg []templateData
type spawnedMsg *sandboxData
type destroyedMsg string
type execMsg *execResultData
type fileWrittenMsg struct{}
type fileReadMsg string
type templateCreatedMsg struct{}
type templateDeletedMsg string
type errMsg error

type Model struct {
	client  *apiClient
	width   int
	height  int
	activeTab tab
	mode    mode

	// Sandbox list
	sandboxes []sandboxData
	cursor    int

	// Providers
	providerList []providerData

	// Templates
	templateList   []templateData
	templateCursor int

	// Inputs
	inputs     []textinput.Model
	inputFocus int

	// State
	health      *healthData
	lastOutput  string
	lastError   string
	statusMsg   string
	confirmMsg  string
	confirmFunc func() tea.Cmd
	logs        []string
}

func NewModel(serverURL, apiKey string) Model {
	client := newAPIClient(serverURL, apiKey)

	// 0: image, 1: ttl, 2: command, 3: filepath, 4: filecontent
	// 5: tmpl name, 6: tmpl image, 7: tmpl desc, 8: tmpl memory, 9: tmpl cpus, 10: tmpl ttl
	imageInput := textinput.New()
	imageInput.Placeholder = "alpine:latest"
	imageInput.CharLimit = 100
	imageInput.Width = 40

	ttlInput := textinput.New()
	ttlInput.Placeholder = "30m"
	ttlInput.CharLimit = 20
	ttlInput.Width = 20

	cmdInput := textinput.New()
	cmdInput.Placeholder = "echo hello world"
	cmdInput.CharLimit = 500
	cmdInput.Width = 60

	filePathInput := textinput.New()
	filePathInput.Placeholder = "/workspace/file.txt"
	filePathInput.CharLimit = 200
	filePathInput.Width = 40

	fileContentInput := textinput.New()
	fileContentInput.Placeholder = "file contents..."
	fileContentInput.CharLimit = 5000
	fileContentInput.Width = 60

	tmplNameInput := textinput.New()
	tmplNameInput.Placeholder = "data-science"
	tmplNameInput.CharLimit = 50
	tmplNameInput.Width = 30

	tmplImageInput := textinput.New()
	tmplImageInput.Placeholder = "python:3.12-alpine"
	tmplImageInput.CharLimit = 100
	tmplImageInput.Width = 40

	tmplDescInput := textinput.New()
	tmplDescInput.Placeholder = "description..."
	tmplDescInput.CharLimit = 200
	tmplDescInput.Width = 50

	tmplMemInput := textinput.New()
	tmplMemInput.Placeholder = "512"
	tmplMemInput.CharLimit = 10
	tmplMemInput.Width = 10

	tmplCPUInput := textinput.New()
	tmplCPUInput.Placeholder = "1"
	tmplCPUInput.CharLimit = 5
	tmplCPUInput.Width = 10

	tmplTTLInput := textinput.New()
	tmplTTLInput.Placeholder = "300"
	tmplTTLInput.CharLimit = 10
	tmplTTLInput.Width = 10

	return Model{
		client: client,
		inputs: []textinput.Model{
			imageInput, ttlInput, cmdInput, filePathInput, fileContentInput,
			tmplNameInput, tmplImageInput, tmplDescInput, tmplMemInput, tmplCPUInput, tmplTTLInput,
		},
		logs: make([]string, 0, 100),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchSandboxes(),
		m.fetchHealth(),
		m.fetchProviders(),
		m.fetchTemplates(),
		m.tick(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		statusBarStyle = statusBarStyle.Width(m.width)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tickMsg:
		return m, tea.Batch(m.fetchSandboxes(), m.fetchHealth(), m.fetchProviders(), m.fetchTemplates(), m.tick())

	case sandboxesMsg:
		m.sandboxes = []sandboxData(msg)
		if m.cursor >= len(m.sandboxes) {
			m.cursor = max(0, len(m.sandboxes)-1)
		}

	case healthMsg:
		m.health = (*healthData)(msg)

	case providersMsg:
		m.providerList = []providerData(msg)

	case templatesMsg:
		m.templateList = []templateData(msg)
		if m.templateCursor >= len(m.templateList) {
			m.templateCursor = max(0, len(m.templateList)-1)
		}

	case spawnedMsg:
		sb := (*sandboxData)(msg)
		m.statusMsg = fmt.Sprintf("Spawned %s (%s)", sb.ID, sb.Image)
		m.addLog("SPAWN", fmt.Sprintf("%s image=%s", sb.ID, sb.Image))
		m.activeTab = tabSandboxes
		m.mode = modeNormal
		return m, m.fetchSandboxes()

	case destroyedMsg:
		m.statusMsg = fmt.Sprintf("Destroyed %s", string(msg))
		m.addLog("KILL", string(msg))
		m.mode = modeNormal
		return m, m.fetchSandboxes()

	case execMsg:
		r := (*execResultData)(msg)
		m.lastOutput = ""
		if r.Stdout != "" {
			m.lastOutput += r.Stdout
		}
		if r.Stderr != "" {
			m.lastOutput += errorStyle.Render(r.Stderr)
		}
		m.lastOutput += dimStyle.Render(fmt.Sprintf("\n[exit %d, %s]", r.ExitCode, r.Duration))
		m.lastError = ""
		m.addLog("EXEC", fmt.Sprintf("exit=%d dur=%s", r.ExitCode, r.Duration))

	case fileWrittenMsg:
		m.statusMsg = "File written successfully"
		m.lastError = ""
		m.addLog("WRITE", "file written")

	case fileReadMsg:
		m.lastOutput = string(msg)
		m.lastError = ""
		m.addLog("READ", fmt.Sprintf("%d bytes", len(msg)))

	case templateCreatedMsg:
		m.statusMsg = "Template created"
		m.mode = modeNormal
		m.addLog("TEMPLATE", "created")
		return m, m.fetchTemplates()

	case templateDeletedMsg:
		m.statusMsg = fmt.Sprintf("Template %s deleted", string(msg))
		m.mode = modeNormal
		m.addLog("TEMPLATE", fmt.Sprintf("deleted %s", string(msg)))
		return m, m.fetchTemplates()

	case errMsg:
		m.lastError = msg.Error()
		m.statusMsg = ""
		m.addLog("ERROR", msg.Error())
	}

	// Update active text input
	if m.mode == modeInput || m.mode == modeExec || m.mode == modeSpawn || m.mode == modeCreateTemplate {
		var cmd tea.Cmd
		idx := m.activeInputIndex()
		if idx >= 0 && idx < len(m.inputs) {
			m.inputs[idx], cmd = m.inputs[idx].Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "ctrl+c" {
		return m, tea.Quit
	}

	// Confirm mode
	if m.mode == modeConfirm {
		switch key {
		case "y", "Y":
			m.mode = modeNormal
			if m.confirmFunc != nil {
				return m, m.confirmFunc()
			}
		case "n", "N", "esc":
			m.mode = modeNormal
			m.confirmMsg = ""
		}
		return m, nil
	}

	// Input modes
	if m.mode == modeInput || m.mode == modeExec || m.mode == modeSpawn || m.mode == modeCreateTemplate || m.mode == modeSpawnTemplate {
		switch key {
		case "esc":
			m.mode = modeNormal
			m.blurAllInputs()
			return m, nil
		case "tab":
			m.cycleInputFocus(1)
			return m, nil
		case "shift+tab":
			m.cycleInputFocus(-1)
			return m, nil
		case "enter":
			return m.submitInput()
		}
		idx := m.activeInputIndex()
		if idx >= 0 && idx < len(m.inputs) {
			var cmd tea.Cmd
			m.inputs[idx], cmd = m.inputs[idx].Update(msg)
			return m, cmd
		}
		return m, nil
	}

	// Normal mode - global keys
	switch key {
	case "q":
		return m, tea.Quit
	case "1":
		m.activeTab = tabDashboard
	case "2":
		m.activeTab = tabSandboxes
	case "3":
		m.activeTab = tabTemplates
	case "4":
		m.activeTab = tabProviders
	case "5":
		m.activeTab = tabLogs
	case "r":
		return m, tea.Batch(m.fetchSandboxes(), m.fetchHealth(), m.fetchProviders(), m.fetchTemplates())
	}

	// Tab-specific keys
	switch m.activeTab {
	case tabDashboard:
		return m.handleDashboardKey(key)
	case tabSandboxes:
		return m.handleSandboxKey(key)
	case tabTemplates:
		return m.handleTemplateKey(key)
	}

	return m, nil
}

func (m *Model) handleDashboardKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "s", "n":
		m.mode = modeSpawn
		m.inputFocus = 0
		m.inputs[0].Focus()
	}
	return m, nil
}

func (m *Model) handleSandboxKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "j", "down":
		if len(m.sandboxes) > 0 {
			m.cursor = min(m.cursor+1, len(m.sandboxes)-1)
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "d", "delete":
		if len(m.sandboxes) > 0 {
			sb := m.sandboxes[m.cursor]
			m.confirmMsg = fmt.Sprintf("Destroy sandbox %s? (y/n)", sb.ID)
			m.mode = modeConfirm
			m.confirmFunc = func() tea.Cmd {
				return m.destroySandbox(sb.ID)
			}
		}
	case "e", "enter":
		if len(m.sandboxes) > 0 {
			m.mode = modeExec
			m.inputFocus = 0
			m.inputs[2].Focus()
		}
	case "s", "n":
		m.mode = modeSpawn
		m.inputFocus = 0
		m.inputs[0].Focus()
	case "f":
		if len(m.sandboxes) > 0 {
			m.mode = modeInput
			m.inputFocus = 0
			m.inputs[3].Focus()
		}
	}
	return m, nil
}

func (m *Model) handleTemplateKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "j", "down":
		if len(m.templateList) > 0 {
			m.templateCursor = min(m.templateCursor+1, len(m.templateList)-1)
		}
	case "k", "up":
		if m.templateCursor > 0 {
			m.templateCursor--
		}
	case "n", "c":
		m.mode = modeCreateTemplate
		m.inputFocus = 0
		m.inputs[5].Focus()
	case "d", "delete":
		if len(m.templateList) > 0 {
			t := m.templateList[m.templateCursor]
			m.confirmMsg = fmt.Sprintf("Delete template %s? (y/n)", t.Name)
			m.mode = modeConfirm
			m.confirmFunc = func() tea.Cmd {
				return m.deleteTemplate(t.Name)
			}
		}
	case "s", "enter":
		if len(m.templateList) > 0 {
			t := m.templateList[m.templateCursor]
			return m, m.spawnFromTemplate(t.Name)
		}
	}
	return m, nil
}

func (m *Model) submitInput() (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeSpawn:
		image := m.inputs[0].Value()
		if image == "" {
			image = "alpine:latest"
		}
		ttl := m.inputs[1].Value()
		if ttl == "" {
			ttl = "30m"
		}
		m.inputs[0].SetValue("")
		m.inputs[1].SetValue("")
		m.blurAllInputs()
		return m, m.spawnSandbox(image, ttl)

	case modeExec:
		cmd := m.inputs[2].Value()
		if cmd == "" || len(m.sandboxes) == 0 {
			return m, nil
		}
		sb := m.sandboxes[m.cursor]
		m.inputs[2].SetValue("")
		return m, m.execCommand(sb.ID, cmd)

	case modeInput:
		// File operations
		path := m.inputs[3].Value()
		content := m.inputs[4].Value()
		if path == "" || len(m.sandboxes) == 0 {
			return m, nil
		}
		sb := m.sandboxes[m.cursor]
		if content != "" {
			m.inputs[3].SetValue("")
			m.inputs[4].SetValue("")
			m.blurAllInputs()
			return m, m.writeFileCmd(sb.ID, path, content)
		}
		m.inputs[3].SetValue("")
		m.blurAllInputs()
		return m, m.readFileCmd(sb.ID, path)

	case modeCreateTemplate:
		name := m.inputs[5].Value()
		image := m.inputs[6].Value()
		if name == "" || image == "" {
			m.lastError = "Template name and image are required"
			return m, nil
		}
		desc := m.inputs[7].Value()
		mem := m.inputs[8].Value()
		cpus := m.inputs[9].Value()
		ttl := m.inputs[10].Value()
		for i := 5; i <= 10; i++ {
			m.inputs[i].SetValue("")
		}
		m.blurAllInputs()
		return m, m.createTemplate(name, image, desc, mem, cpus, ttl)
	}

	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var b strings.Builder
	w := min(m.width, 120)

	// Title bar
	title := titleStyle.Render(" HATCHIT ")
	healthStr := ""
	if m.health != nil {
		healthStr = successStyle.Render(" ONLINE") + dimStyle.Render(fmt.Sprintf(" | up %s | v%s", m.health.Uptime, m.health.Version))
	} else {
		healthStr = errorStyle.Render(" OFFLINE")
	}
	b.WriteString(title + healthStr + "\n")

	// Tabs
	tabs := []string{"[1]Dashboard", "[2]Sandboxes", "[3]Templates", "[4]Providers", "[5]Logs"}
	var tabRow strings.Builder
	for i, t := range tabs {
		if tab(i) == m.activeTab {
			tabRow.WriteString(tabActiveStyle.Render(t))
		} else {
			tabRow.WriteString(tabInactiveStyle.Render(t))
		}
		tabRow.WriteString(" ")
	}
	b.WriteString(tabRow.String() + "\n")
	b.WriteString(strings.Repeat("─", w) + "\n")

	// Content area
	contentHeight := m.height - 7
	if contentHeight < 5 {
		contentHeight = 5
	}

	var content string
	switch m.activeTab {
	case tabDashboard:
		content = m.viewDashboard(contentHeight, w)
	case tabSandboxes:
		content = m.viewSandboxes(contentHeight, w)
	case tabTemplates:
		content = m.viewTemplates(contentHeight, w)
	case tabProviders:
		content = m.viewProviders(contentHeight, w)
	case tabLogs:
		content = m.viewLogs(contentHeight)
	}
	b.WriteString(content)

	// Status bar
	b.WriteString("\n")
	b.WriteString(statusBarStyle.Render(m.viewStatusBar(w)))

	// Help line
	b.WriteString("\n")
	if m.mode == modeConfirm {
		b.WriteString(boldStyle.Render(m.confirmMsg))
	} else if m.mode != modeNormal {
		b.WriteString(dimStyle.Render("tab:next field  enter:submit  esc:cancel"))
	} else {
		switch m.activeTab {
		case tabDashboard:
			b.WriteString(dimStyle.Render("1-5:tabs  s:spawn  r:refresh  q:quit"))
		case tabSandboxes:
			b.WriteString(dimStyle.Render("j/k:nav  s:spawn  e/enter:exec  f:files  d:destroy  r:refresh  q:quit"))
		case tabTemplates:
			b.WriteString(dimStyle.Render("j/k:nav  n:new template  s/enter:spawn from template  d:delete  q:quit"))
		case tabProviders:
			b.WriteString(dimStyle.Render("r:refresh  q:quit"))
		case tabLogs:
			b.WriteString(dimStyle.Render("r:refresh  q:quit"))
		}
	}

	return b.String()
}

// ── Dashboard View ───────────────────────────────────────

func (m Model) viewDashboard(height, width int) string {
	var b strings.Builder

	// Stats row
	active := 0
	for _, sb := range m.sandboxes {
		if sb.State == "running" {
			active++
		}
	}
	provCount := len(m.providerList)
	tmplCount := len(m.templateList)

	b.WriteString("\n")
	stats := []string{
		cardStyle.Render(fmt.Sprintf(" %s\n %s",
			boldStyle.Render("Active Sandboxes"),
			gaugeStyle.Render(fmt.Sprintf("%d", active)))),
		cardStyle.Render(fmt.Sprintf(" %s\n %s",
			boldStyle.Render("Templates"),
			gaugeStyle.Render(fmt.Sprintf("%d", tmplCount)))),
		cardStyle.Render(fmt.Sprintf(" %s\n %s",
			boldStyle.Render("Providers"),
			gaugeStyle.Render(fmt.Sprintf("%d", provCount)))),
	}
	b.WriteString("  " + lipgloss.JoinHorizontal(lipgloss.Top, stats...) + "\n\n")

	// Resource summary
	totalMem := 0
	totalCPU := 0
	for _, sb := range m.sandboxes {
		if sb.State == "running" {
			totalMem += sb.MemoryMB
			totalCPU += sb.VCPUs
		}
	}
	if active > 0 {
		b.WriteString(boldStyle.Render("  Resource Usage") + "\n")
		b.WriteString(fmt.Sprintf("  CPU cores: %s    Memory: %s\n",
			gaugeStyle.Render(fmt.Sprintf("%d", totalCPU)),
			gaugeStyle.Render(fmt.Sprintf("%d MB", totalMem))))
		b.WriteString("\n")
	}

	// Recent sandboxes
	b.WriteString(boldStyle.Render("  Recent Sandboxes") + "\n")
	if len(m.sandboxes) == 0 {
		b.WriteString(dimStyle.Render("  No sandboxes. Press [s] to spawn one.\n"))
	} else {
		max := min(5, len(m.sandboxes))
		for _, sb := range m.sandboxes[:max] {
			stateStr := stateIcon(sb.State) + " " + sb.State
			ttlStr := formatTTL(sb.ExpiresAt)
			b.WriteString(fmt.Sprintf("  %-12s  %-10s  %-20s  %s\n",
				dimStyle.Render(sb.ID), stateStr, truncate(sb.Image, 20), ttlStr))
		}
	}

	b.WriteString("\n")

	// Quick spawn overlay
	if m.mode == modeSpawn {
		b.WriteString(boldStyle.Render("  Quick Spawn\n"))
		b.WriteString(fmt.Sprintf("  Image: %s\n", m.inputs[0].View()))
		b.WriteString(fmt.Sprintf("  TTL:   %s\n", m.inputs[1].View()))
	}

	// Recent activity
	if len(m.logs) > 0 {
		b.WriteString(boldStyle.Render("  Recent Activity") + "\n")
		start := max(0, len(m.logs)-3)
		for _, line := range m.logs[start:] {
			b.WriteString("  " + line + "\n")
		}
	}

	return b.String()
}

// ── Sandboxes View ───────────────────────────────────────

func (m Model) viewSandboxes(height, width int) string {
	var b strings.Builder

	if m.mode == modeSpawn {
		b.WriteString(boldStyle.Render("\n  Spawn New Sandbox\n\n"))
		b.WriteString(fmt.Sprintf("  Image: %s\n", m.inputs[0].View()))
		b.WriteString(fmt.Sprintf("  TTL:   %s\n", m.inputs[1].View()))
		b.WriteString(dimStyle.Render("\n  Press Enter to spawn, Esc to cancel\n"))
		return b.String()
	}

	if m.mode == modeExec {
		target := "(none)"
		if len(m.sandboxes) > 0 && m.cursor < len(m.sandboxes) {
			sb := m.sandboxes[m.cursor]
			target = fmt.Sprintf("%s (%s)", sb.ID, sb.Image)
		}
		b.WriteString(boldStyle.Render(fmt.Sprintf("\n  Execute in: %s\n\n", target)))
		b.WriteString(fmt.Sprintf("  Command: %s\n", m.inputs[2].View()))
		if m.lastOutput != "" {
			b.WriteString(boldStyle.Render("\n  Output:\n"))
			lines := strings.Split(m.lastOutput, "\n")
			maxLines := height - 8
			for i, line := range lines {
				if i >= maxLines {
					b.WriteString(dimStyle.Render(fmt.Sprintf("  ... (%d more lines)", len(lines)-i)) + "\n")
					break
				}
				b.WriteString("  " + outputStyle.Render(line) + "\n")
			}
		}
		return b.String()
	}

	if m.mode == modeInput {
		target := "(none)"
		if len(m.sandboxes) > 0 && m.cursor < len(m.sandboxes) {
			sb := m.sandboxes[m.cursor]
			target = fmt.Sprintf("%s (%s)", sb.ID, sb.Image)
		}
		b.WriteString(boldStyle.Render(fmt.Sprintf("\n  Files in: %s\n\n", target)))
		b.WriteString(fmt.Sprintf("  Path:    %s\n", m.inputs[3].View()))
		b.WriteString(fmt.Sprintf("  Content: %s\n", m.inputs[4].View()))
		b.WriteString(dimStyle.Render("\n  Enter path only = read, path+content = write\n"))
		if m.lastOutput != "" {
			b.WriteString(boldStyle.Render("\n  File contents:\n"))
			lines := strings.Split(m.lastOutput, "\n")
			maxLines := height - 10
			for i, line := range lines {
				if i >= maxLines {
					b.WriteString(dimStyle.Render(fmt.Sprintf("  ... (%d more lines)", len(lines)-i)) + "\n")
					break
				}
				b.WriteString("  " + outputStyle.Render(line) + "\n")
			}
		}
		return b.String()
	}

	// Normal list view
	if len(m.sandboxes) == 0 {
		b.WriteString(dimStyle.Render("\n  No sandboxes running. Press [s] to spawn one.\n"))
		return b.String()
	}

	header := fmt.Sprintf("  %-14s %-10s %-10s %-20s %-10s %-12s",
		"ID", "STATE", "PROVIDER", "IMAGE", "CREATED", "TTL")
	b.WriteString(headerStyle.Render(header) + "\n")

	for i, sb := range m.sandboxes {
		ttlStr := formatTTL(sb.ExpiresAt)

		line := fmt.Sprintf("  %-14s %-10s %-10s %-20s %-10s %-12s",
			sb.ID, sb.State, sb.Provider,
			truncate(sb.Image, 20),
			sb.CreatedAt.Format("15:04:05"),
			ttlStr,
		)

		if i == m.cursor {
			b.WriteString(selectedRowStyle.Render("▸ "+line[2:]) + "\n")
		} else {
			b.WriteString(normalRowStyle.Render(line) + "\n")
		}

		if i >= height-3 {
			remaining := len(m.sandboxes) - i - 1
			if remaining > 0 {
				b.WriteString(dimStyle.Render(fmt.Sprintf("  ... and %d more", remaining)) + "\n")
			}
			break
		}
	}

	return b.String()
}

// ── Templates View ───────────────────────────────────────

func (m Model) viewTemplates(height, width int) string {
	var b strings.Builder

	if m.mode == modeCreateTemplate {
		b.WriteString(boldStyle.Render("\n  Create Template\n\n"))
		b.WriteString(fmt.Sprintf("  Name:        %s\n", m.inputs[5].View()))
		b.WriteString(fmt.Sprintf("  Image:       %s\n", m.inputs[6].View()))
		b.WriteString(fmt.Sprintf("  Description: %s\n", m.inputs[7].View()))
		b.WriteString(fmt.Sprintf("  Memory (MB): %s\n", m.inputs[8].View()))
		b.WriteString(fmt.Sprintf("  CPU Cores:   %s\n", m.inputs[9].View()))
		b.WriteString(fmt.Sprintf("  TTL (secs):  %s\n", m.inputs[10].View()))
		b.WriteString(dimStyle.Render("\n  Press Enter to create, Esc to cancel\n"))
		return b.String()
	}

	if len(m.templateList) == 0 {
		b.WriteString(dimStyle.Render("\n  No templates configured. Press [n] to create one.\n"))
		return b.String()
	}

	header := fmt.Sprintf("  %-20s %-25s %-8s %-6s %-8s %-6s",
		"NAME", "IMAGE", "MEM(MB)", "CPUS", "TTL(s)", "POOL")
	b.WriteString(headerStyle.Render(header) + "\n")

	for i, t := range m.templateList {
		line := fmt.Sprintf("  %-20s %-25s %-8d %-6d %-8d %-6d",
			truncate(t.Name, 20),
			truncate(t.Image, 25),
			t.MemoryMB, t.CPUCores, t.TTLSeconds, t.PoolSize,
		)

		if i == m.templateCursor {
			b.WriteString(selectedRowStyle.Render("▸ "+line[2:]) + "\n")
		} else {
			b.WriteString(normalRowStyle.Render(line) + "\n")
		}

		if i >= height-3 {
			break
		}
	}

	// Detail panel for selected template
	if m.templateCursor < len(m.templateList) {
		t := m.templateList[m.templateCursor]
		b.WriteString("\n")
		b.WriteString(boldStyle.Render(fmt.Sprintf("  Template: %s", t.Name)) + "\n")
		if t.Description != "" {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  %s", t.Description)) + "\n")
		}
		b.WriteString(fmt.Sprintf("  Image: %s  |  Memory: %dMB  |  CPUs: %d  |  TTL: %ds\n",
			t.Image, t.MemoryMB, t.CPUCores, t.TTLSeconds))
	}

	return b.String()
}

// ── Providers View ───────────────────────────────────────

func (m Model) viewProviders(height, width int) string {
	var b strings.Builder

	if len(m.providerList) == 0 {
		b.WriteString(dimStyle.Render("\n  No providers configured.\n"))
		return b.String()
	}

	header := fmt.Sprintf("  %-20s %-10s %-10s",
		"NAME", "DEFAULT", "HEALTHY")
	b.WriteString(headerStyle.Render(header) + "\n")

	for _, p := range m.providerList {
		defaultStr := ""
		if p.IsDefault {
			defaultStr = successStyle.Render("default")
		}
		healthStr := successStyle.Render("healthy")
		if !p.Healthy {
			healthStr = errorStyle.Render("unhealthy")
		}

		line := fmt.Sprintf("  %-20s %-10s %-10s",
			p.Name, defaultStr, healthStr,
		)
		b.WriteString(normalRowStyle.Render(line) + "\n")
	}

	return b.String()
}

// ── Logs View ────────────────────────────────────────────

func (m Model) viewLogs(height int) string {
	var b strings.Builder
	b.WriteString(boldStyle.Render("\n  Activity Log\n\n"))

	if len(m.logs) == 0 {
		b.WriteString(dimStyle.Render("  No activity yet.\n"))
		return b.String()
	}

	start := len(m.logs) - (height - 4)
	if start < 0 {
		start = 0
	}
	for _, line := range m.logs[start:] {
		b.WriteString("  " + line + "\n")
	}

	return b.String()
}

// ── Status Bar ───────────────────────────────────────────

func (m Model) viewStatusBar(width int) string {
	left := fmt.Sprintf(" %d sandbox(es) | %d template(s) | %d provider(s)",
		len(m.sandboxes), len(m.templateList), len(m.providerList))
	right := ""
	if m.statusMsg != "" {
		right = successStyle.Render(m.statusMsg)
	} else if m.lastError != "" {
		right = errorStyle.Render(truncate(m.lastError, 50))
	}
	padding := width - lipgloss.Width(left) - lipgloss.Width(right)
	if padding < 1 {
		padding = 1
	}
	return left + strings.Repeat(" ", padding) + right
}

// ── Commands ─────────────────────────────────────────────

func (m Model) tick() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) fetchSandboxes() tea.Cmd {
	return func() tea.Msg {
		sbs, err := m.client.listSandboxes()
		if err != nil {
			return errMsg(err)
		}
		return sandboxesMsg(sbs)
	}
}

func (m Model) fetchHealth() tea.Cmd {
	return func() tea.Msg {
		h, err := m.client.health()
		if err != nil {
			return errMsg(err)
		}
		return healthMsg(h)
	}
}

func (m Model) fetchProviders() tea.Cmd {
	return func() tea.Msg {
		p, err := m.client.listProviders()
		if err != nil {
			return errMsg(err)
		}
		return providersMsg(p)
	}
}

func (m Model) fetchTemplates() tea.Cmd {
	return func() tea.Msg {
		t, err := m.client.listTemplates()
		if err != nil {
			return errMsg(err)
		}
		return templatesMsg(t)
	}
}

func (m Model) spawnSandbox(image, ttl string) tea.Cmd {
	return func() tea.Msg {
		sb, err := m.client.spawn(image, ttl)
		if err != nil {
			return errMsg(err)
		}
		return spawnedMsg(sb)
	}
}

func (m Model) spawnFromTemplate(name string) tea.Cmd {
	return func() tea.Msg {
		sb, err := m.client.spawnTemplate(name)
		if err != nil {
			return errMsg(err)
		}
		return spawnedMsg(sb)
	}
}

func (m Model) destroySandbox(id string) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.destroy(id); err != nil {
			return errMsg(err)
		}
		return destroyedMsg(id)
	}
}

func (m Model) execCommand(id, cmd string) tea.Cmd {
	return func() tea.Msg {
		r, err := m.client.exec(id, cmd)
		if err != nil {
			return errMsg(err)
		}
		return execMsg(r)
	}
}

func (m Model) writeFileCmd(id, path, content string) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.writeFile(id, path, content); err != nil {
			return errMsg(err)
		}
		return fileWrittenMsg{}
	}
}

func (m Model) readFileCmd(id, path string) tea.Cmd {
	return func() tea.Msg {
		data, err := m.client.readFile(id, path)
		if err != nil {
			return errMsg(err)
		}
		return fileReadMsg(data)
	}
}

func (m Model) createTemplate(name, image, desc, mem, cpus, ttl string) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.createTemplate(name, image, desc, mem, cpus, ttl); err != nil {
			return errMsg(err)
		}
		return templateCreatedMsg{}
	}
}

func (m Model) deleteTemplate(name string) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.deleteTemplate(name); err != nil {
			return errMsg(err)
		}
		return templateDeletedMsg(name)
	}
}

// ── Helpers ──────────────────────────────────────────────

func (m Model) activeInputIndex() int {
	switch m.mode {
	case modeSpawn:
		return m.inputFocus // 0=image, 1=ttl
	case modeExec:
		return 2
	case modeInput:
		return 3 + m.inputFocus // 3=path, 4=content
	case modeCreateTemplate:
		return 5 + m.inputFocus // 5-10
	}
	return -1
}

func (m *Model) blurAllInputs() {
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
}

func (m *Model) cycleInputFocus(dir int) {
	m.blurAllInputs()
	switch m.mode {
	case modeSpawn:
		m.inputFocus = (m.inputFocus + dir + 2) % 2
		m.inputs[m.inputFocus].Focus()
	case modeInput:
		m.inputFocus = (m.inputFocus + dir + 2) % 2
		m.inputs[3+m.inputFocus].Focus()
	case modeCreateTemplate:
		m.inputFocus = (m.inputFocus + dir + 6) % 6
		m.inputs[5+m.inputFocus].Focus()
	}
}

func (m *Model) addLog(kind, msg string) {
	ts := time.Now().Format("15:04:05")
	entry := dimStyle.Render(ts) + " " + boldStyle.Render(kind) + " " + msg
	m.logs = append(m.logs, entry)
	if len(m.logs) > 100 {
		m.logs = m.logs[1:]
	}
}

func stateIcon(state string) string {
	switch state {
	case "running":
		return successStyle.Render("●")
	case "creating":
		return warnStyle.Render("◐")
	case "idle":
		return dimStyle.Render("○")
	case "error":
		return errorStyle.Render("✗")
	default:
		return dimStyle.Render("·")
	}
}

func formatTTL(expiresAt time.Time) string {
	ttl := time.Until(expiresAt).Round(time.Second)
	if ttl < 0 {
		return errorStyle.Render("expired")
	}
	if ttl < 5*time.Minute {
		return warnStyle.Render(ttl.String())
	}
	return dimStyle.Render(ttl.String())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
