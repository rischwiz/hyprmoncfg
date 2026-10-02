package apply

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestEnginePlanRendersWhatApplyWritesWithoutWriting(t *testing.T) {
	p := newTestProfile()
	p.Workspaces = profile.WorkspaceSettings{
		Enabled:  true,
		Strategy: profile.WorkspaceStrategyManual,
		Rules: []profile.WorkspaceRule{
			{Workspace: "1", OutputKey: monitors[0].HardwareKey(), OutputName: monitors[0].Name, Default: true},
		},
	}
	engine, logPath, err := initTestEngine(t)
	if err != nil {
		t.Fatalf("init test engine: %v", err)
	}
	before, err := os.ReadFile(engine.MonitorsConfPath)
	if err != nil {
		t.Fatalf("read monitors config: %v", err)
	}
	rootBefore, err := os.ReadFile(engine.HyprlandConfigPath)
	if err != nil {
		t.Fatalf("read root config: %v", err)
	}

	ctx := context.Background()
	plan, err := engine.Plan(ctx, p, monitors)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	after, _ := os.ReadFile(engine.MonitorsConfPath)
	rootAfter, _ := os.ReadFile(engine.HyprlandConfigPath)
	if string(after) != string(before) || string(rootAfter) != string(rootBefore) {
		t.Fatal("plan changed a config file")
	}
	log, _ := os.ReadFile(logPath)
	for _, line := range strings.Split(strings.TrimSpace(string(log)), "\n") {
		if line != "" && !strings.HasPrefix(line, "-j ") {
			t.Fatalf("plan sent a command other than a read: %q", line)
		}
	}
	if plan.MonitorsPath != engine.MonitorsConfPath {
		t.Fatalf("monitors path = %q, want %q", plan.MonitorsPath, engine.MonitorsConfPath)
	}
	if len(plan.WorkspaceCommands) == 0 {
		t.Fatal("plan lists no workspace commands for a profile with workspace rules")
	}

	if _, err := engine.Apply(ctx, p, monitors); err != nil {
		t.Fatalf("apply: %v", err)
	}
	written, err := os.ReadFile(engine.MonitorsConfPath)
	if err != nil {
		t.Fatalf("read applied monitors config: %v", err)
	}
	if plan.Rendered != string(written) {
		t.Fatalf("plan differs from what apply wrote.\nplan:\n%s\napplied:\n%s", plan.Rendered, written)
	}
}

func TestEnginePlanRejectsAnOverlappingLayout(t *testing.T) {
	p := newTestProfile()
	for i := range p.Outputs {
		p.Outputs[i].X, p.Outputs[i].Y = 0, 0
	}
	engine, _, err := initTestEngine(t)
	if err != nil {
		t.Fatalf("init test engine: %v", err)
	}
	if _, err := engine.Plan(context.Background(), p, monitors); err == nil {
		t.Fatal("plan accepted a layout apply would refuse")
	}
}
