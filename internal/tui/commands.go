package tui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/crmne/hyprmoncfg/internal/apply"
	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/profileio"
)

// restartDaemonCmd hands the running daemon over to the installed build. The
// package manager cannot do this: it installs as root, while the daemon is a
// user service only the session can restart.
func (m *Model) restartDaemonCmd() tea.Cmd {
	if !m.daemonNeedsRestart() {
		return nil
	}
	m.setStatusOK("Restarting the daemon...")

	return tea.Sequence(
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, "systemctl", "--user", "restart", "hyprmoncfgd.service").CombinedOutput()
			if err != nil {
				detail := strings.TrimSpace(string(out))
				if detail == "" {
					detail = err.Error()
				}
				return daemonRestartMsg{err: fmt.Errorf("restart hyprmoncfgd: %s", detail)}
			}
			return daemonRestartMsg{}
		},
		m.refreshCmd(true),
	)
}

func (m Model) refreshCmd(background bool) tea.Cmd {
	client := m.client
	store := m.store
	ipcClient := m.ipc
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		daemon := daemonReachable(ctx, ipcClient)
		// A failure that is not a timeout means the connection is gone, not
		// that the daemon is: reconnect before calling it stopped.
		var replacement *ipc.Client
		if ipcClient != nil && !daemon.ok && !daemon.unknown {
			if replacement = redialDaemon(ctx); replacement != nil {
				daemon = daemonReachable(ctx, replacement)
			}
		}
		daemonOK, daemonUnknown, daemonVersion, profileOverride := daemon.ok, daemon.unknown, daemon.version, daemon.profileOverride

		monitors, err := client.Monitors(ctx)
		if err != nil {
			return refreshMsg{daemonOK: daemonOK, daemonUnknown: daemonUnknown, daemonVersion: daemonVersion, profileOverride: profileOverride, daemonClient: replacement, background: background, err: err}
		}
		profiles, err := store.List()
		if err != nil {
			return refreshMsg{daemonOK: daemonOK, daemonUnknown: daemonUnknown, daemonVersion: daemonVersion, profileOverride: profileOverride, daemonClient: replacement, background: background, err: err}
		}
		workspaceRules, err := client.WorkspaceRules(ctx)
		if err != nil {
			return refreshMsg{daemonOK: daemonOK, daemonUnknown: daemonUnknown, daemonVersion: daemonVersion, profileOverride: profileOverride, daemonClient: replacement, background: background, err: err}
		}
		workspaces, err := client.Workspaces(ctx)
		if err != nil {
			return refreshMsg{daemonOK: daemonOK, daemonUnknown: daemonUnknown, daemonVersion: daemonVersion, profileOverride: profileOverride, daemonClient: replacement, background: background, err: err}
		}
		lidState, err := lid.ReadState(ctx)
		if err != nil {
			lidState = lid.Unknown
		}

		return refreshMsg{
			monitors:        monitors,
			profiles:        profiles,
			workspaceRules:  workspaceRules,
			workspaces:      workspaces,
			lidState:        lidState,
			daemonOK:        daemonOK,
			daemonUnknown:   daemonUnknown,
			daemonVersion:   daemonVersion,
			profileOverride: profileOverride,
			fallbacks:       daemon.fallbacks,
			daemonClient:    replacement,
			background:      background,
		}
	}
}

// daemonReachable answers whether the daemon is running. A daemon that is busy
// applying a profile can miss the deadline while being perfectly alive, so a
// timeout or compositor_busy reply reports "unknown" and leaves the last answer
// standing; a busy reply does not mean the connection needs to be replaced.
type daemonProbe struct {
	ok, unknown     bool
	version         string
	profileOverride string
	fallbacks       map[string]appstatus.MonitorFallback
}

func daemonReachable(ctx context.Context, client *ipc.Client) daemonProbe {
	if client == nil {
		return daemonProbe{}
	}

	probeCtx, cancel := context.WithTimeout(ctx, daemonProbeTimeout)
	defer cancel()
	document, err := client.Status(probeCtx)
	if err != nil {
		return daemonProbe{unknown: isTimeout(err) || errors.Is(err, ipc.ErrCompositorBusy)}
	}
	probe := daemonProbe{
		ok:              true,
		version:         strings.TrimSpace(document.Version),
		profileOverride: strings.TrimSpace(document.Daemon.ProfileOverride),
	}
	for _, monitor := range document.Monitors {
		if monitor.Fallback != nil {
			if probe.fallbacks == nil {
				probe.fallbacks = map[string]appstatus.MonitorFallback{}
			}
			probe.fallbacks[monitor.Name] = *monitor.Fallback
		}
	}
	return probe
}

// redialDaemon reconnects after the daemon was restarted, which the connection
// dialed at startup cannot survive. Without this a restart, an upgrade, or a
// crash would leave the session reporting a stopped daemon until it is
// relaunched.
func redialDaemon(ctx context.Context) *ipc.Client {
	path, err := ipc.SocketPath()
	if err != nil {
		return nil
	}
	dialCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	client, err := ipc.Dial(dialCtx, path)
	if err != nil {
		return nil
	}

	probeCtx, probeCancel := context.WithTimeout(ctx, daemonProbeTimeout)
	defer probeCancel()
	if _, err := client.Status(probeCtx); err != nil && !errors.Is(err, ipc.ErrCompositorBusy) {
		_ = client.Close()
		return nil
	}
	return client
}

const daemonProbeTimeout = 3 * time.Second

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func (m Model) saveCmd(p profile.Profile) tea.Cmd {
	if m.ipc != nil {
		client := m.ipc
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := client.Save(ctx, ipc.SaveParams{Profile: p}); err != nil {
				return saveMsg{err: err}
			}
			return saveMsg{name: p.Name}
		}
	}
	store := m.store
	return func() tea.Msg {
		if err := profileio.SaveWithSidecars(store, p); err != nil {
			return saveMsg{err: err}
		}
		return saveMsg{name: p.Name}
	}
}

func (m Model) saveProfileCmd(p profile.Profile) tea.Cmd {
	if m.ipc != nil {
		client := m.ipc
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := client.Save(ctx, ipc.SaveParams{Profile: p}); err != nil {
				return saveMsg{name: p.Name, err: err, profileTab: true}
			}
			return saveMsg{name: p.Name, profileTab: true}
		}
	}
	store := m.store
	return func() tea.Msg {
		if err := profileio.SaveWithSidecars(store, p); err != nil {
			return saveMsg{name: p.Name, err: err, profileTab: true}
		}
		return saveMsg{name: p.Name, profileTab: true}
	}
}

func (m Model) deleteCmd(name string) tea.Cmd {
	if m.ipc != nil {
		client := m.ipc
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := client.Delete(ctx, name); err != nil {
				return deleteMsg{name: name, err: err}
			}
			return deleteMsg{name: name}
		}
	}
	store := m.store
	return func() tea.Msg {
		if err := store.Delete(name); err != nil {
			return deleteMsg{name: name, err: err}
		}
		return deleteMsg{name: name}
	}
}

func (m Model) setProfileAutomaticCmd(enabled bool) tea.Cmd {
	client := m.ipc
	return func() tea.Msg {
		if client == nil {
			return profileAutoMsg{enabled: enabled, err: errors.New("automatic profile selection requires the daemon")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		return profileAutoMsg{enabled: enabled, err: client.SetProfileAuto(ctx, enabled)}
	}
}

func (m Model) applyCmd(p profile.Profile, allowUnmanagedOverwrite ...bool) tea.Cmd {
	if m.ipc != nil {
		client := m.ipc
		guard := m.remoteGuard
		if guard != nil {
			guard.begin()
		}
		return func() tea.Msg {
			if guard != nil {
				defer guard.finish()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			transaction, err := client.Preview(ctx, ipc.PreviewParams{
				Profile: &p,
			})
			if err != nil {
				return applyMsg{profile: p, remote: true, err: err}
			}
			if guard != nil {
				guard.arm(transaction.ID)
			}
			return applyMsg{
				profile:       transaction.Profile,
				transactionID: transaction.ID,
				deadline:      transaction.Deadline,
				remote:        true,
			}
		}
	}

	client := m.client
	engine := m.engine
	guard := m.revertGuard
	if guard != nil {
		guard.begin()
	}
	return func() tea.Msg {
		if guard != nil {
			defer guard.finish()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		monitors, err := client.Monitors(ctx)
		if err != nil {
			return applyMsg{profile: p, err: err}
		}
		applyProfile := p
		if state, err := lid.ReadState(ctx); err == nil && state == lid.Closed {
			applyProfile, _ = profile.ApplyClosedLidPolicy(p, monitors)
		}
		snapshot, err := engine.Apply(ctx, applyProfile, monitors, apply.ApplyModeInteractive)
		if err != nil {
			return applyMsg{profile: p, err: err}
		}
		if guard != nil {
			guard.arm(snapshot)
		}
		return applyMsg{profile: applyProfile, snapshot: snapshot}
	}
}

func (m *Model) armPendingRevert(snapshot apply.RevertState) {
	if m.revertGuard == nil {
		m.revertGuard = &pendingRevertGuard{}
	}
	m.revertGuard.arm(snapshot)
}

func (m *Model) disarmPendingRevert() {
	if m.revertGuard != nil {
		m.revertGuard.disarm()
	}
}

func (m *Model) armPendingRemote(transactionID string) {
	if m.remoteGuard == nil {
		m.remoteGuard = &pendingRemoteGuard{}
	}
	m.remoteGuard.arm(transactionID)
}

func (m *Model) disarmPendingRemote() {
	if m.remoteGuard != nil {
		m.remoteGuard.disarm()
	}
}

func (m Model) RevertPending(ctx context.Context) error {
	if m.remoteGuard != nil {
		transactionID, armed, err := m.remoteGuard.pending(ctx)
		if err != nil {
			return err
		}
		if armed {
			if m.ipc == nil {
				return errors.New("cannot revert daemon transaction without IPC connection")
			}
			if err := m.ipc.Revert(ctx, transactionID); err != nil && !errors.Is(err, ipc.ErrTransactionUnavailable) {
				return err
			}
			m.remoteGuard.disarm()
		}
	}
	if m.revertGuard == nil {
		return nil
	}
	snapshot, armed, err := m.revertGuard.pending(ctx)
	if err != nil || !armed {
		return err
	}
	if err := m.engine.Revert(ctx, snapshot); err != nil {
		return err
	}
	m.revertGuard.disarm()
	return nil
}

func (g *pendingRevertGuard) begin() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inFlight == 0 {
		g.idle = make(chan struct{})
	}
	g.inFlight++
}

func (g *pendingRevertGuard) finish() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inFlight--
	if g.inFlight == 0 {
		close(g.idle)
		g.idle = nil
	}
}

func (g *pendingRevertGuard) arm(snapshot apply.RevertState) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.snapshot = snapshot
	g.armed = true
}

func (g *pendingRevertGuard) disarm() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.armed = false
}

func (g *pendingRevertGuard) pending(ctx context.Context) (apply.RevertState, bool, error) {
	for {
		g.mu.Lock()
		if g.inFlight == 0 {
			snapshot := g.snapshot
			armed := g.armed
			g.mu.Unlock()
			return snapshot, armed, nil
		}
		idle := g.idle
		g.mu.Unlock()

		select {
		case <-idle:
		case <-ctx.Done():
			return apply.RevertState{}, false, ctx.Err()
		}
	}
}

func (g *pendingRevertGuard) isArmed() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.armed
}

func (g *pendingRemoteGuard) begin() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inFlight == 0 {
		g.idle = make(chan struct{})
	}
	g.inFlight++
}

func (g *pendingRemoteGuard) finish() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inFlight--
	if g.inFlight == 0 {
		close(g.idle)
		g.idle = nil
	}
}

func (g *pendingRemoteGuard) arm(transactionID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.transactionID = transactionID
	g.armed = true
}

func (g *pendingRemoteGuard) disarm() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.armed = false
	g.transactionID = ""
}

func (g *pendingRemoteGuard) pending(ctx context.Context) (string, bool, error) {
	for {
		g.mu.Lock()
		if g.inFlight == 0 {
			transactionID := g.transactionID
			armed := g.armed
			g.mu.Unlock()
			return transactionID, armed, nil
		}
		idle := g.idle
		g.mu.Unlock()

		select {
		case <-idle:
		case <-ctx.Done():
			return "", false, ctx.Err()
		}
	}
}

func (m Model) postApply(p profile.Profile) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return m.engine.PostApply(ctx, p)
}

func (m Model) confirmPending(pending pendingApply) error {
	if pending.remote {
		if m.ipc == nil {
			return errors.New("cannot confirm daemon transaction without IPC connection")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return m.ipc.Confirm(ctx, pending.transactionID)
	}
	return m.postApply(pending.profile)
}

func (m Model) revertCmd(pending pendingApply, reason string) tea.Cmd {
	if pending.remote {
		client := m.ipc
		guard := m.remoteGuard
		if guard != nil {
			guard.begin()
		}
		return func() tea.Msg {
			if guard != nil {
				defer guard.finish()
			}
			if client == nil {
				return revertMsg{err: errors.New("cannot revert daemon transaction without IPC connection"), reason: reason}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := client.Revert(ctx, pending.transactionID)
			if errors.Is(err, ipc.ErrTransactionUnavailable) {
				err = nil
			}
			if err == nil && guard != nil {
				guard.disarm()
			}
			return revertMsg{err: err, reason: reason}
		}
	}

	engine := m.engine
	guard := m.revertGuard
	if guard != nil {
		guard.begin()
	}
	return func() tea.Msg {
		if guard != nil {
			defer guard.finish()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := engine.Revert(ctx, pending.snapshot)
		if err == nil && guard != nil {
			guard.disarm()
		}
		return revertMsg{err: err, reason: reason}
	}
}
