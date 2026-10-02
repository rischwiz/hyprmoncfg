package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestConfirmApplySignalRejectsConfiguration(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	signals := make(chan os.Signal, 1)
	signals <- os.Interrupt
	var output bytes.Buffer

	keep, err := confirmApplyWithInput(10, reader, &output, signals)
	if err != nil {
		t.Fatalf("confirm after signal: %v", err)
	}
	if keep {
		t.Fatal("expected signal to reject unconfirmed configuration")
	}
}

func TestDoctorReportsMissingGeneratedMonitorConfig(t *testing.T) {
	dir := t.TempDir()
	rootPath := filepath.Join(dir, "hyprland.lua")
	monitorsPath := filepath.Join(dir, "hyprmoncfg-monitors.lua")
	if err := os.WriteFile(rootPath, []byte(config.IncludeLine(config.HyprConfigLua, monitorsPath)+"\n"), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}

	cmd := newDoctorCmd(&monitorsPath, &rootPath)
	var output bytes.Buffer
	cmd.SetOut(&output)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor: %v", err)
	}

	got := output.String()
	if !strings.Contains(got, "PROBLEM") || !strings.Contains(got, "does not exist") {
		t.Fatalf("doctor output did not report the missing generated file:\n%s", got)
	}
}

func TestCompleteProfileNamesListsSavedProfiles(t *testing.T) {
	dir := t.TempDir()
	store := profile.NewStore(dir)
	for _, name := range []string{"desk", "conference"} {
		p := profile.New(name, []profile.OutputConfig{{Key: "acme|panel|1", Name: "DP-1", Enabled: true, Scale: 1}})
		if err := store.Save(p); err != nil {
			t.Fatalf("save %s: %v", name, err)
		}
	}

	complete := completeProfileNames(&dir)
	names, directive := complete(nil, nil, "")
	if strings.Join(names, ",") != "conference,desk" {
		t.Fatalf("names = %v, want conference and desk", names)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("directive = %v, want no file completion", directive)
	}
	if names, _ := complete(nil, []string{"desk"}, ""); len(names) != 0 {
		t.Fatalf("second argument offered %v, want nothing", names)
	}
}

func TestCompleteProfileNamesDoesNotCreateTheProfileDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")

	names, _ := completeProfileNames(&dir)(nil, nil, "")
	if len(names) != 0 {
		t.Fatalf("names = %v, want none", names)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("completion created %s", dir)
	}
}

func TestProfileCommandsCompleteProfileNames(t *testing.T) {
	root := newRootCmd()
	for _, name := range []string{"save", "apply", "delete"} {
		cmd, _, err := root.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s: %v", name, err)
		}
		if cmd.ValidArgsFunction == nil {
			t.Fatalf("%s does not complete profile names", name)
		}
	}
}
