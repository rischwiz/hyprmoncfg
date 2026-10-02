package notify

import "os/exec"

// terminalLauncher is the freedesktop helper that opens a command in the
// person's preferred terminal.
const terminalLauncher = "xdg-terminal-exec"

// TUILauncher returns a function that opens the hyprmoncfg editor in a
// terminal, or nil when this session has no way to do that. A nil launcher
// means notifications are sent without an action instead of with one that
// does nothing.
func TUILauncher(lookPath func(string) (string, error), start func(name string, args ...string) error) func() {
	launcher, err := lookPath(terminalLauncher)
	if err != nil {
		return nil
	}
	editor, err := lookPath("hyprmoncfg")
	if err != nil {
		return nil
	}
	return func() { _ = start(launcher, editor) }
}

// StartDetached starts a command without waiting for it.
func StartDetached(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}
