package tui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/crmne/hyprmoncfg/internal/apply"
	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/omarchywatch"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

type uiMode int

const (
	modeMain uiMode = iota
	modeSave
	modeSaveConfirm
	modeConfirm
	modeModePicker
	modeNumericInput
	modeProfileExecInput
	modeKeybindings
	modeDeleteConfirm
)

type mainTab int

const (
	tabLayout mainTab = iota
	tabWorkspaces
	tabProfiles
)

type layoutFocus int

const (
	layoutFocusCanvas layoutFocus = iota
	layoutFocusInspector
)

type inspectorTab int

const (
	inspectorTabDisplay inspectorTab = iota
	inspectorTabColor
)

type refreshMsg struct {
	monitors        []hypr.Monitor
	profiles        []profile.Profile
	workspaceRules  []hypr.WorkspaceRule
	workspaces      []hypr.WorkspaceState
	lidState        lid.State
	daemonOK        bool
	daemonUnknown   bool
	daemonVersion   string
	profileOverride string
	// fallbacks maps connector names to displays the daemon runs below
	// their saved settings.
	fallbacks    map[string]appstatus.MonitorFallback
	daemonClient *ipc.Client
	background   bool
	err          error
}

type saveMsg struct {
	name       string
	err        error
	profileTab bool
}

type deleteMsg struct {
	name string
	err  error
}

type applyMsg struct {
	profile       profile.Profile
	snapshot      apply.RevertState
	transactionID string
	deadline      time.Time
	remote        bool
	err           error
}

type daemonRestartMsg struct {
	err error
}

type profileAutoMsg struct {
	enabled bool
	err     error
}

type revertMsg struct {
	err    error
	reason string
}

type openURLMsg struct {
	label string
	url   string
	err   error
}

type clearToastMsg struct {
	token int
}

type clearSnapMsg struct {
	token int
}

type tickMsg time.Time

type pendingApply struct {
	profile       profile.Profile
	snapshot      apply.RevertState
	transactionID string
	deadline      time.Time
	// total is the confirmation window as it stood when the preview became
	// live, so the countdown meter can show how much of it is left.
	total  time.Duration
	remote bool
}

type pendingRevertGuard struct {
	mu       sync.Mutex
	armed    bool
	snapshot apply.RevertState
	inFlight int
	idle     chan struct{}
}

type pendingRemoteGuard struct {
	mu            sync.Mutex
	armed         bool
	transactionID string
	inFlight      int
	idle          chan struct{}
}

type toastState struct {
	message string
	err     bool
	token   int
}

type editableOutput struct {
	Key               string
	MatchKey          string
	Name              string
	Description       string
	Make              string
	Model             string
	Serial            string
	PhysicalWidth     int
	PhysicalHeight    int
	Enabled           bool
	Modes             []string
	HardwareModes     []string
	ModeIndex         int
	ModeUnsupported   bool
	Width             int
	Height            int
	Refresh           float64
	X                 int
	Y                 int
	Scale             float64
	VRR               int
	Transform         int
	Focused           bool
	DPMSStatus        bool
	IsInternal        bool
	MirrorOf          string
	ActiveWorkspace   string
	Bitdepth          int
	CM                string
	SDRBrightness     float64
	SDRSaturation     float64
	SDRMinLuminance   float64
	SDRMaxLuminance   int
	MinLuminance      float64
	MaxLuminance      int
	SupportsWideColor int
	SupportsHDR       int
	MaxAvgLuminance   int
	SDREOTF           string
	ICC               string
}

type canvasCell struct {
	ch   rune
	fg   string
	bg   string
	bold bool
}

type canvasCardColors struct {
	bg     string
	border string
	fg     string
	muted  string
}

type snapEdge int

const (
	snapEdgeLeft snapEdge = iota
	snapEdgeRight
	snapEdgeTop
	snapEdgeBottom
)

type snapDirection int

const (
	snapDirectionLeft snapDirection = iota
	snapDirectionRight
	snapDirectionUp
	snapDirectionDown
)

type snapMark struct {
	OutputIndex int
	Edge        snapEdge
}

type snapHintState struct {
	Token int
	Marks []snapMark
}

type snapAxisCandidate struct {
	pos   int
	dist  int
	marks []snapMark
}

type snapAnalysis struct {
	x snapAxisCandidate
	y snapAxisCandidate
}

type workspaceEditor struct {
	PersistAll              bool
	Enabled                 bool
	Strategy                profile.WorkspaceStrategy
	MaxWorkspaces           int
	GroupSize               int
	LastSequentialGroupSize int
	MonitorOrder            []string
	Rules                   []profile.WorkspaceRule
	ManualRulesInitialized  bool
	SelectedField           int
	SelectedOrder           int
}

type Model struct {
	client  *hypr.Client
	store   *profile.Store
	engine  apply.Engine
	ipc     *ipc.Client
	openURL func(string) error

	styles styles

	mode        uiMode
	tab         mainTab
	layoutFocus layoutFocus

	monitors       []hypr.Monitor
	profiles       []profile.Profile
	workspaceRules []hypr.WorkspaceRule
	workspaces     []hypr.WorkspaceState
	lidState       lid.State

	editOutputs       []editableOutput
	workspaceEdit     workspaceEditor
	selectedOutput    int
	inspectorField    int
	inspectorTab      inspectorTab
	selectedProfile   int
	deleteProfileName string

	pending       *pendingApply
	revertGuard   *pendingRevertGuard
	remoteGuard   *pendingRemoteGuard
	saveDialog    *saveDialogState
	saveOverwrite string
	picker        *modePickerState
	input         *numericInputState
	execInput     *profileExecInputState
	drag          *canvasDragState
	toast         *toastState
	snap          *snapHintState
	snapSeq       int
	toastSeq      int

	resetRequested        bool
	status                string
	statusErr             bool
	dirty                 bool
	draftSaved            bool
	draftProfileName      string
	matchedProfileName    string
	activeProfileName     string
	draftExec             string
	disableUnknownOutputs bool
	history               undoHistory
	daemonOK              bool
	daemonVersion         string
	profileOverride       string
	fallbacks             map[string]appstatus.MonitorFallback
	profileModePending    bool
	refreshInFlight       bool
	applying              bool
	quitAfterApply        bool
	quitAfterRevert       bool

	width  int
	height int

	// now is the clock for preview countdowns; nil means time.Now.
	now func() time.Time

	layoutErr error
}

func (m Model) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

const defaultWorkspaceGroupSize = 3

func NewModel(client *hypr.Client, store *profile.Store, monitorsConfPath string, hyprlandConfigPath string) Model {
	return Model{
		client: client,
		store:  store,
		engine: apply.Engine{
			Client:             client,
			WakeConfig:         omarchywatch.NewWakeConfig(),
			MonitorsConfPath:   monitorsConfPath,
			HyprlandConfigPath: hyprlandConfigPath,
			Logf: func(format string, args ...any) {
				fmt.Fprintf(os.Stderr, format, args...)
			},
		},
		openURL:     openExternalURL,
		revertGuard: &pendingRevertGuard{},
		styles:      newStyles(),
		mode:        modeMain,
		tab:         tabLayout,
		layoutFocus: layoutFocusCanvas,
		status:      "Loading Hyprland state...",
		workspaceEdit: workspaceEditor{
			Strategy:                profile.WorkspaceStrategySequential,
			MaxWorkspaces:           9,
			GroupSize:               defaultWorkspaceGroupSize,
			LastSequentialGroupSize: defaultWorkspaceGroupSize,
		},
	}
}

func NewModelWithIPC(client *hypr.Client, store *profile.Store, monitorsConfPath string, hyprlandConfigPath string, ipcClient *ipc.Client) Model {
	model := NewModel(client, store, monitorsConfPath, hyprlandConfigPath)
	model.ipc = ipcClient
	model.remoteGuard = &pendingRemoteGuard{}
	model.daemonOK = ipcClient != nil
	return model
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(false), tickCmd())
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.picker != nil {
			m.picker.List.SetSize(m.modePickerWidth(), m.modePickerHeight())
		}
		if m.saveDialog != nil {
			m.saveDialog.List.SetSize(m.saveDialogListWidth(), clampInt(defaultHeight(m.height)-18, 3, 10))
			m.saveDialog.Input.Width = m.saveDialogInputWidth()
		}
		if m.input != nil {
			m.input.Input.Width = m.numericInputWidthFor(m.input.Kind)
		}
		if m.execInput != nil {
			m.execInput.Input.Width = clampInt(m.modalMaxWidth()-16, 24, 72)
		}
		return m, nil

	case refreshMsg:
		m.refreshInFlight = false
		if msg.daemonClient != nil {
			if m.ipc != nil {
				_ = m.ipc.Close()
			}
			m.ipc = msg.daemonClient
		}
		if !msg.daemonUnknown {
			m.daemonOK = msg.daemonOK
			m.daemonVersion = msg.daemonVersion
			m.profileOverride = msg.profileOverride
			m.fallbacks = msg.fallbacks
		}
		if msg.err != nil {
			m.setStatusErr(msg.err.Error())
			return m, nil
		}

		prevSig := m.liveConfigSignature()
		nextSig := liveConfigSignature(msg.monitors, msg.lidState)
		liveChanged := prevSig != nextSig
		wasDirty := m.dirty

		m.monitors = msg.monitors
		m.profiles = msg.profiles
		m.workspaceRules = msg.workspaceRules
		m.workspaces = msg.workspaces
		m.lidState = msg.lidState

		reloadLive := len(m.editOutputs) == 0 || liveChanged || (!msg.background && !m.dirty)
		if reloadLive {
			m.loadLiveState()
			if liveChanged && wasDirty {
				m.markClean()
				m.setStatusOK("Monitor configuration changed. Reloaded live state.")
				m.syncSelections()
				return m, nil
			}
		}
		m.syncSelections()
		if !msg.background {
			m.status = ""
		}
		return m, nil

	case saveMsg:
		if msg.err != nil {
			m.quitAfterApply = false
			m.setStatusErr(msg.err.Error())
			m.mode = modeMain
			return m, nil
		}
		if msg.profileTab {
			m.setStatusOK(fmt.Sprintf("Saved profile %q", msg.name))
			return m, m.refreshCmd(false)
		}
		action := saveActionOnly
		if m.saveDialog != nil {
			action = m.saveDialog.Action
		}
		m.saveDialog = nil
		m.saveOverwrite = ""
		m.draftProfileName = msg.name
		m.matchedProfileName = msg.name
		m.draftSaved = true
		m.mode = modeMain
		m.quitAfterApply = false
		if action == saveActionCancel {
			m.setStatusOK("Save cancelled")
			return m, nil
		}
		if action == saveActionSaveQuit {
			m.quitAfterApply = true
			m.applying = true
			return m, m.applyCmd(m.currentProfile(msg.name))
		}
		if action == saveActionApply {
			m.applying = true
			return m, tea.Batch(
				m.refreshCmd(false),
				m.applyCmd(m.currentProfile(msg.name)),
			)
		}
		m.setStatusOK(fmt.Sprintf("Saved profile %q", msg.name))
		return m, m.refreshCmd(false)

	case daemonRestartMsg:
		if msg.err != nil {
			m.setStatusErr(msg.err.Error())
			return m, nil
		}
		m.setStatusOK("Daemon restarted")
		return m, m.refreshCmd(true)

	case profileAutoMsg:
		m.profileModePending = false
		if msg.err != nil {
			m.setStatusErr(msg.err.Error())
			return m, nil
		}
		if msg.enabled {
			m.profileOverride = ""
			m.setStatusOK("Automatic profile selection on")
		} else {
			m.profileOverride = m.activeProfileName
			m.setStatusOK("Automatic profile selection off")
		}
		return m, m.refreshCmd(false)

	case clearSnapMsg:
		if m.snap != nil && msg.token == m.snap.Token {
			m.snap = nil
		}
		return m, nil

	case clearToastMsg:
		if m.toast != nil && msg.token == m.toast.token {
			m.toast = nil
		}
		return m, nil

	case deleteMsg:
		if msg.err != nil {
			m.setStatusErr(msg.err.Error())
			return m, nil
		}
		if strings.EqualFold(strings.TrimSpace(msg.name), strings.TrimSpace(m.draftProfileName)) {
			m.draftProfileName = ""
			m.draftExec = ""
		}
		if strings.EqualFold(strings.TrimSpace(msg.name), strings.TrimSpace(m.matchedProfileName)) {
			m.matchedProfileName = ""
		}
		m.setStatusOK(fmt.Sprintf("Deleted profile %q", msg.name))
		m.selectedProfile = clampIndex(m.selectedProfile, len(m.profiles))
		return m, m.refreshCmd(false)

	case applyMsg:
		m.applying = false
		if msg.err != nil {
			if m.quitAfterRevert {
				m.quitAfterRevert = false
				m.quitAfterApply = false
				return m, tea.Quit
			}
			m.quitAfterApply = false
			m.setStatusErr(msg.err.Error())
			m.mode = modeMain
			return m, nil
		}
		deadline := msg.deadline
		if deadline.IsZero() {
			deadline = m.clock().Add(apply.DefaultPreviewTimeout)
		}
		m.pending = &pendingApply{
			profile:       msg.profile,
			snapshot:      msg.snapshot,
			transactionID: msg.transactionID,
			deadline:      deadline,
			total:         deadline.Sub(m.clock()),
			remote:        msg.remote,
		}
		if msg.remote {
			m.armPendingRemote(msg.transactionID)
		} else {
			m.armPendingRevert(msg.snapshot)
		}
		m.mode = modeConfirm
		m.statusErr = false
		m.status = fmt.Sprintf("%s applied. Changes are live until you confirm or revert.", targetLabel(msg.profile.Name))
		if m.quitAfterRevert {
			return m, m.revertCmd(*m.pending, "quit")
		}
		return m, tickCmd()

	case revertMsg:
		quitAfterRevert := m.quitAfterRevert
		m.quitAfterRevert = false
		if msg.err != nil {
			m.mode = modeConfirm
			if m.pending != nil {
				m.pending.deadline = m.clock().Add(apply.DefaultPreviewTimeout)
				m.pending.total = apply.DefaultPreviewTimeout
			}
			m.setStatusErr(fmt.Sprintf("Revert failed: %v", msg.err))
			return m, nil
		}
		m.mode = modeMain
		m.pending = nil
		m.quitAfterApply = false
		m.disarmPendingRevert()
		m.disarmPendingRemote()
		m.markClean()
		m.draftProfileName = ""
		m.matchedProfileName = ""
		m.draftExec = ""
		m.setStatusOK("Configuration reverted: " + msg.reason)
		if quitAfterRevert {
			return m, tea.Quit
		}
		return m, m.refreshCmd(false)

	case openURLMsg:
		if msg.err != nil {
			m.setStatusErr(fmt.Sprintf("Failed to open %s link: %v", msg.label, msg.err))
		}
		return m, nil

	case tickMsg:
		if m.mode == modeConfirm && m.pending != nil {
			if m.clock().After(m.pending.deadline) {
				return m, m.revertCmd(*m.pending, "timeout")
			}
		}
		cmds := []tea.Cmd{tickCmd()}
		if !m.refreshInFlight {
			m.refreshInFlight = true
			cmds = append(cmds, m.refreshCmd(true))
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		switch m.mode {
		case modeSave:
			return m.updateSaveKeys(msg)
		case modeSaveConfirm:
			return m.updateSaveConfirmKeys(msg)
		case modeDeleteConfirm:
			name := m.deleteProfileName
			if msg.String() == "y" {
				m.mode, m.deleteProfileName = modeMain, ""
				return m, m.deleteCmd(name)
			}
			if msg.String() == "esc" || msg.String() == "n" || msg.String() == "enter" {
				m.mode, m.deleteProfileName = modeMain, ""
			}
			return m, nil
		case modeConfirm:
			return m.updateConfirmKeys(msg)
		case modeModePicker:
			return m.updateModePickerKeys(msg)
		case modeNumericInput:
			return m.updateNumericInputKeys(msg)
		case modeProfileExecInput:
			return m.updateProfileExecInputKeys(msg)
		case modeKeybindings:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			m.mode = modeMain
			return m, nil
		default:
			return m.updateMainKeys(msg)
		}

	case tea.MouseMsg:
		return m.updateMouse(msg)
	}

	// Forward unhandled messages (e.g. cursor blinks) to the active text input.
	switch m.mode {
	case modeSave:
		if m.saveDialog != nil {
			var cmd tea.Cmd
			m.saveDialog.Input, cmd = m.saveDialog.Input.Update(msg)
			return m, cmd
		}
	case modeNumericInput:
		if m.input != nil {
			var cmd tea.Cmd
			m.input.Input, cmd = m.input.Input.Update(msg)
			return m, cmd
		}
	case modeProfileExecInput:
		if m.execInput != nil {
			var cmd tea.Cmd
			m.execInput.Input, cmd = m.execInput.Input.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m Model) View() string {
	switch m.mode {
	case modeSave:
		return m.renderModalScreen(m.renderSavePrompt())
	case modeSaveConfirm:
		return m.renderModalScreen(m.renderSaveConfirm())
	case modeDeleteConfirm:
		return m.renderModalScreen(m.renderModalFrame("Delete profile?", []string{
			m.styles.warning.Render(fmt.Sprintf("Delete %q?", m.deleteProfileName)),
			"Your live layout will not change.",
			"",
			// Cancel carries the focus: it is what Enter does.
			m.styles.focused.UnsetPadding().Render(deleteCancelLabel) + "  " + m.styles.warning.Render(deleteConfirmLabel),
			"",
			m.styles.help.Render("y deletes. Enter, Esc or n cancels."),
		}))
	case modeConfirm:
		return m.renderModalScreen(m.renderConfirm())
	case modeModePicker:
		return m.renderModalScreen(m.renderModePicker())
	case modeNumericInput:
		if m.inlineEntryActive(m.input.FieldIndex) {
			return m.renderMain()
		}
		return m.renderModalScreen(m.renderNumericInput())
	case modeProfileExecInput:
		return m.renderModalScreen(m.renderProfileExecInput())
	case modeKeybindings:
		return m.renderModalScreen(m.renderKeybindings())
	default:
		return m.renderMain()
	}
}

func (m Model) renderMain() string {
	tabs := m.renderTabs()
	toast := m.renderToast()
	toastHeight := 0
	if toast != "" {
		toastHeight = lipgloss.Height(toast) + 1
	}

	footerText := m.renderFooterBar()
	bodyHeight := max(3, m.mainBodyHeight(tabs, "", footerText)-toastHeight)

	var body string
	switch m.tab {
	case tabLayout:
		body = m.renderLayoutView(bodyHeight)
	case tabProfiles:
		body = m.renderProfilesView(bodyHeight)
	case tabWorkspaces:
		body = m.renderWorkspaceView(bodyHeight)
	}
	body = lipgloss.NewStyle().Height(bodyHeight).MaxHeight(bodyHeight).Render(body)

	styledFooter := m.decorateFooterBar(footerText)
	content := strings.Join([]string{
		tabs,
		body,
	}, "\n")
	if toast != "" {
		content = strings.Join([]string{
			content,
			lipgloss.PlaceHorizontal(m.footerContentWidth(), lipgloss.Center, toast),
		}, "\n")
	}
	content = strings.Join([]string{
		content,
		styledFooter,
	}, "\n")
	app := m.styles.app
	return app.Width(max(1, m.terminalWidth()-app.GetHorizontalFrameSize())).
		Height(max(1, m.terminalHeight()-app.GetVerticalFrameSize())).
		MaxHeight(max(1, m.terminalHeight()-app.GetVerticalFrameSize())).
		Render(content)
}

func (m Model) renderTabs() string {
	labels := []string{"Layout", "Workspaces", "Profiles"}
	parts := make([]string, 0, len(labels)*2+1)
	lineStyle := withFG(lipgloss.NewStyle(), m.styles.palette.paneBorder)
	parts = append(parts, lineStyle.Render("─"))
	// Only the selected tab carries the accent color, numeral included, so the
	// current tab reads at a glance.
	for idx, label := range labels {
		number := fmt.Sprintf("%d", idx+1)
		if int(m.tab) == idx {
			// The current page is a filled pill, so it reads without color too.
			pill := withBG(lipgloss.NewStyle().Bold(true), m.styles.palette.tabPillBg)
			parts = append(parts, pill.Render(" ")+withFG(pill, m.styles.palette.tabActiveFg).Render(number)+pill.Render(" "+label+" "))
		} else {
			numStyle := withFG(lipgloss.NewStyle().Bold(true), m.styles.palette.tabInactiveFg)
			parts = append(parts, m.styles.tabInactive.Render(fmt.Sprintf(" %s %s ", numStyle.Render(number), label)))
		}
		parts = append(parts, lineStyle.Render("─"))
	}

	left := lipgloss.JoinHorizontal(lipgloss.Center, parts...)
	status := m.renderTopStatus()
	width := m.footerContentWidth()
	availableStatus := max(1, width-lipgloss.Width(left)-2)
	if lipgloss.Width(status) > availableStatus {
		status = m.renderCompactTopStatus()
	}
	if lipgloss.Width(status) > availableStatus {
		status = ansi.Truncate(status, availableStatus, "")
	}
	statusStart := width - lipgloss.Width(status) - 1
	gap := max(1, statusStart-lipgloss.Width(left))
	return left + lineStyle.Render(strings.Repeat("─", gap)) + status + lineStyle.Render("─")
}

func (m Model) mainBodyHeight(tabs string, status string, help string) int {
	reserved := lipgloss.Height(tabs) + lipgloss.Height(help)
	return max(3, m.terminalHeight()-reserved)
}

func (m Model) useCompactLayout(bodyHeight int) bool {
	return bodyHeight < 14 || m.terminalWidth() < 96
}

func (m Model) inspectorDetailLines(output editableOutput) []string {
	return m.hardwareDetailLines(output)
}
