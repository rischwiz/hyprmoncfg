// Package notify sends desktop notifications over the session bus.
package notify

import (
	"context"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	busName       = "org.freedesktop.Notifications"
	objectPath    = dbus.ObjectPath("/org/freedesktop/Notifications")
	notifyMethod  = busName + ".Notify"
	actionSignal  = "ActionInvoked"
	appName       = "hyprmoncfg"
	icon          = "video-display"
	defaultAction = "default"
	// expireDefault lets the notification server pick how long to show it.
	expireDefault = int32(-1)
)

// Notification is one message for the person at the desk.
type Notification struct {
	Summary string
	Body    string
	// ActionLabel names the single action offered. Empty offers none.
	ActionLabel string
}

// Notifier delivers notifications. Delivery is best effort: a desktop without
// a notification server is not an error worth failing anything over.
type Notifier interface {
	Notify(ctx context.Context, notification Notification) error
}

// DBus is a Notifier for org.freedesktop.Notifications. It connects on first
// use, so a session without a notification server costs nothing at startup.
type DBus struct {
	// OnAction runs when the person chooses the notification's action. Nil
	// means no action is offered, whatever the notification asks for.
	OnAction func()
	Logf     func(format string, args ...any)

	mu     sync.Mutex
	conn   *dbus.Conn
	lastID uint32
}

func (d *DBus) Notify(ctx context.Context, notification Notification) error {
	conn, err := d.connect()
	if err != nil {
		return err
	}
	var id uint32
	call := conn.Object(busName, objectPath).CallWithContext(ctx, notifyMethod, 0, notifyArgs(notification, d.OnAction != nil)...)
	if err := call.Store(&id); err != nil {
		return err
	}
	d.mu.Lock()
	d.lastID = id
	d.mu.Unlock()
	return nil
}

// notifyArgs are the arguments of org.freedesktop.Notifications.Notify. The
// action list is pairs of key and label; the "default" key is what a click on
// the notification itself invokes.
func notifyArgs(notification Notification, actionAvailable bool) []any {
	actions := []string{}
	if actionAvailable && notification.ActionLabel != "" {
		actions = []string{defaultAction, notification.ActionLabel}
	}
	return []any{
		appName, uint32(0), icon, notification.Summary, notification.Body,
		actions, map[string]dbus.Variant{}, expireDefault,
	}
}

func (d *DBus) connect() (*dbus.Conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn != nil && d.conn.Connected() {
		return d.conn, nil
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	d.conn = conn
	if d.OnAction != nil {
		signals := make(chan *dbus.Signal, 4)
		if err := conn.AddMatchSignal(dbus.WithMatchInterface(busName), dbus.WithMatchMember(actionSignal)); err != nil {
			d.logf("notifications: cannot listen for actions: %v", err)
		} else {
			conn.Signal(signals)
			go d.watchActions(signals)
		}
	}
	return conn, nil
}

func (d *DBus) watchActions(signals <-chan *dbus.Signal) {
	for signal := range signals {
		d.mu.Lock()
		last := d.lastID
		d.mu.Unlock()
		if invoked(signal, last) {
			d.OnAction()
		}
	}
}

// invoked reports whether a signal is the action of the notification this
// process sent last. Every client on the bus sees every ActionInvoked, so the
// id has to match.
func invoked(signal *dbus.Signal, id uint32) bool {
	if signal == nil || signal.Name != busName+"."+actionSignal || len(signal.Body) < 2 {
		return false
	}
	got, ok := signal.Body[0].(uint32)
	if !ok || id == 0 || got != id {
		return false
	}
	key, ok := signal.Body[1].(string)
	return ok && key == defaultAction
}

func (d *DBus) logf(format string, args ...any) {
	if d.Logf != nil {
		d.Logf(format, args...)
	}
}
