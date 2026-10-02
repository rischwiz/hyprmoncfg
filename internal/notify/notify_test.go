package notify

import (
	"reflect"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestNotifyArgsOfferTheActionOnlyWhenItCanBeHandled(t *testing.T) {
	notification := Notification{Summary: "Projector connected", Body: "Added to the right.", ActionLabel: "Adjust and save…"}

	args := notifyArgs(notification, true)
	if len(args) != 8 {
		t.Fatalf("Notify takes 8 arguments, got %d", len(args))
	}
	if args[0] != "hyprmoncfg" || args[1] != uint32(0) || args[3] != "Projector connected" || args[4] != "Added to the right." {
		t.Fatalf("unexpected arguments: %#v", args)
	}
	if got := args[5].([]string); !reflect.DeepEqual(got, []string{"default", "Adjust and save…"}) {
		t.Fatalf("actions = %v", got)
	}
	if args[7] != int32(-1) {
		t.Fatalf("expiry = %v, want the server default", args[7])
	}

	// With nothing to run, or no label, the notification carries no action.
	for name, got := range map[string][]any{
		"no handler": notifyArgs(notification, false),
		"no label":   notifyArgs(Notification{Summary: "s"}, true),
	} {
		if actions := got[5].([]string); len(actions) != 0 {
			t.Fatalf("%s: actions = %v, want none", name, actions)
		}
	}
}

func TestInvokedMatchesOnlyOurNotificationsDefaultAction(t *testing.T) {
	signal := func(body ...any) *dbus.Signal {
		return &dbus.Signal{Name: "org.freedesktop.Notifications.ActionInvoked", Body: body}
	}
	if !invoked(signal(uint32(7), "default"), 7) {
		t.Fatal("our notification's action was not recognised")
	}
	for name, s := range map[string]*dbus.Signal{
		"another client's notification": signal(uint32(8), "default"),
		"another action":                signal(uint32(7), "dismiss"),
		"short body":                    signal(uint32(7)),
		"wrong types":                   signal("7", "default"),
		"another signal":                {Name: "org.freedesktop.Notifications.NotificationClosed", Body: []any{uint32(7), "default"}},
		"nil":                           nil,
	} {
		if invoked(s, 7) {
			t.Fatalf("%s was treated as our action", name)
		}
	}
	if invoked(signal(uint32(0), "default"), 0) {
		t.Fatal("an action matched before any notification was sent")
	}
}

func TestTUILauncherNeedsATerminalHelperAndTheEditor(t *testing.T) {
	found := func(available ...string) func(string) (string, error) {
		return func(name string) (string, error) {
			for _, candidate := range available {
				if candidate == name {
					return "/usr/bin/" + name, nil
				}
			}
			return "", errNotFound
		}
	}
	var started [][]string
	start := func(name string, args ...string) error {
		started = append(started, append([]string{name}, args...))
		return nil
	}

	if TUILauncher(found("hyprmoncfg"), start) != nil {
		t.Fatal("a launcher was offered without a terminal helper")
	}
	if TUILauncher(found("xdg-terminal-exec"), start) != nil {
		t.Fatal("a launcher was offered without the editor on PATH")
	}

	launch := TUILauncher(found("xdg-terminal-exec", "hyprmoncfg"), start)
	if launch == nil {
		t.Fatal("no launcher although both commands exist")
	}
	launch()
	want := [][]string{{"/usr/bin/xdg-terminal-exec", "/usr/bin/hyprmoncfg"}}
	if !reflect.DeepEqual(started, want) {
		t.Fatalf("started %v, want %v", started, want)
	}
}

var errNotFound = &lookupError{}

type lookupError struct{}

func (*lookupError) Error() string { return "not found" }
