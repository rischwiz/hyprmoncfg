package daemon

import (
	"context"
	"errors"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/notify"
	"github.com/crmne/hyprmoncfg/internal/prefs"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

type fakeNotifier struct {
	sent []notify.Notification
	err  error
}

func (f *fakeNotifier) Notify(_ context.Context, notification notify.Notification) error {
	f.sent = append(f.sent, notification)
	return f.err
}

func noticeTestSetup() (saved profile.Profile, monitors []hypr.Monitor) {
	laptop := hypr.Monitor{Name: "eDP-1", Make: "BOE", Model: "Panel", Width: 1920, Height: 1080, Scale: 1}
	// The projector has just been plugged in: present, but not yet driven.
	projector := hypr.Monitor{Name: "HDMI-A-1", Make: "Acme", Model: "Beam 4", Width: 1920, Height: 1080, Scale: 1, Disabled: true}
	monitors = []hypr.Monitor{laptop, projector}
	saved = profile.FromMonitors("Laptop", monitors[:1])
	return saved, monitors
}

func TestNewSetupNoticeSaysWhatWasAddedAndThatTheProfileIsUnchanged(t *testing.T) {
	saved, monitors := noticeTestSetup()
	effective := profile.ExtendConnected(saved, monitors)

	notice, ok := newSetupNotice(saved, effective, monitors, profile.NewDisplayRight)
	if !ok {
		t.Fatal("a newly added display produced no notice")
	}
	if notice.Summary != "Beam 4 connected" {
		t.Fatalf("summary = %q", notice.Summary)
	}
	if notice.Body != "Added to the right of your layout. Your Laptop profile is unchanged." {
		t.Fatalf("body = %q", notice.Body)
	}
	if notice.ActionLabel != "Adjust and save…" {
		t.Fatalf("action = %q", notice.ActionLabel)
	}

	left, _ := newSetupNotice(saved, effective, monitors, profile.NewDisplayLeft)
	if left.Body != "Added to the left of your layout. Your Laptop profile is unchanged." {
		t.Fatalf("left body = %q", left.Body)
	}
}

func TestNewSetupNoticeForALayoutWithNoProfileSaysNothingIsSaved(t *testing.T) {
	_, monitors := noticeTestSetup()
	draft := profile.Profile{Name: "draft"}
	monitors[0].Disabled = true
	effective := profile.ExtendConnected(draft, monitors)

	notice, ok := newSetupNotice(draft, effective, monitors, profile.NewDisplayRight)
	if !ok || notice.Summary != "2 displays connected" {
		t.Fatalf("notice = %+v, %v", notice, ok)
	}
	if notice.Body != "Added to the right of your layout. Nothing is saved yet." {
		t.Fatalf("body = %q", notice.Body)
	}
}

func TestNewSetupNoticeStaysQuietWhenNothingNewHappened(t *testing.T) {
	saved, monitors := noticeTestSetup()

	// A known setup: the profile already covers every display.
	known := profile.FromMonitors("Desk", monitors)
	if _, ok := newSetupNotice(known, profile.ExtendConnected(known, monitors), monitors, profile.NewDisplayRight); ok {
		t.Fatal("restoring a known profile produced a notice")
	}

	// A strict profile keeps the display off, so nothing was added.
	strict := saved
	strict.DisableUnknownOutputs = true
	if _, ok := newSetupNotice(strict, profile.ExtendConnected(strict, monitors), monitors, profile.NewDisplayRight); ok {
		t.Fatal("a display a strict profile keeps off produced a notice")
	}

	// After a daemon restart the display is already live where the layout
	// puts it; re-applying that is not news.
	effective := profile.ExtendConnected(saved, monitors)
	live := append([]hypr.Monitor(nil), monitors...)
	for _, output := range effective.Outputs {
		if output.Name == "HDMI-A-1" {
			live[1].Disabled, live[1].X, live[1].Y = false, output.X, output.Y
		}
	}
	if _, ok := newSetupNotice(saved, effective, live, profile.NewDisplayRight); ok {
		t.Fatal("re-applying an unchanged extended layout produced a notice")
	}
}

func announceTestService(t *testing.T) (*Service, *fakeNotifier, *logRecorder) {
	t.Helper()
	dir := t.TempDir()
	notifier, logs := &fakeNotifier{}, &logRecorder{}
	svc := New(nil, profile.NewStore(dir), Config{ConfigDir: dir, Notifier: notifier, NotifyInline: true, Logf: logs.logf})
	return svc, notifier, logs
}

func TestAnnounceNewSetupSendsOncePerMonitorSet(t *testing.T) {
	svc, notifier, _ := announceTestService(t)
	saved, monitors := noticeTestSetup()
	effective := profile.ExtendConnected(saved, monitors)

	svc.announceNewSetup(saved, effective, monitors, "set-a")
	svc.announceNewSetup(saved, effective, monitors, "set-a")
	if len(notifier.sent) != 1 {
		t.Fatalf("sent %d notifications for one setup, want 1", len(notifier.sent))
	}

	// Back to a known setup, then the projector again: that is news again.
	svc.announceNewSetup(saved, saved, monitors[:1], "set-laptop")
	svc.announceNewSetup(saved, effective, monitors, "set-a")
	if len(notifier.sent) != 2 {
		t.Fatalf("sent %d notifications after reconnecting, want 2", len(notifier.sent))
	}
}

func TestAnnounceNewSetupRespectsThePreference(t *testing.T) {
	svc, notifier, _ := announceTestService(t)
	quiet := prefs.Default()
	quiet.NotifyNewSetup = false
	if _, err := svc.SetPreferences(quiet); err != nil {
		t.Fatal(err)
	}
	saved, monitors := noticeTestSetup()

	svc.announceNewSetup(saved, profile.ExtendConnected(saved, monitors), monitors, "set-a")
	if len(notifier.sent) != 0 {
		t.Fatalf("sent %d notifications with the preference off", len(notifier.sent))
	}
}

func TestAnnounceNewSetupOnlyLogsWhenThereIsNoNotificationServer(t *testing.T) {
	svc, notifier, logs := announceTestService(t)
	notifier.err = errors.New("org.freedesktop.Notifications was not provided")
	saved, monitors := noticeTestSetup()

	svc.announceNewSetup(saved, profile.ExtendConnected(saved, monitors), monitors, "set-a")
	if !logs.contains("new setup notification not delivered") {
		t.Fatalf("a failed delivery was not logged: %q", logs.all())
	}

	// And without a notifier at all, announcing is a no-op.
	silent := New(nil, profile.NewStore(t.TempDir()), Config{})
	silent.announceNewSetup(saved, profile.ExtendConnected(saved, monitors), monitors, "set-a")
}
