package tui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/profile"
)

// workspaceStrategyChoices is the Strategy row's cycle. Off is not stored as a
// strategy: it clears Enabled and keeps the plan, so choosing a strategy again
// brings the saved settings back.
var workspaceStrategyChoices = []profile.WorkspaceStrategy{
	workspaceStrategyOff,
	profile.WorkspaceStrategyManual,
	profile.WorkspaceStrategySequential,
	profile.WorkspaceStrategyInterleave,
}

const workspaceStrategyOff profile.WorkspaceStrategy = "off"

func (m Model) workspaceStrategyChoice() profile.WorkspaceStrategy {
	if !m.workspaceEdit.Enabled {
		return workspaceStrategyOff
	}
	return blankStrategy(m.workspaceEdit.Strategy)
}

func workspaceStrategyLabel(strategy profile.WorkspaceStrategy) string {
	switch strategy {
	case workspaceStrategyOff:
		return "Off"
	case profile.WorkspaceStrategyManual:
		return "Manual"
	case profile.WorkspaceStrategyInterleave:
		return "Interleaved"
	default:
		return "Sequential"
	}
}

func (m *Model) adjustWorkspaceField(delta int) {
	switch m.workspaceEdit.SelectedField {
	case 0:
		current := 0
		for idx, strategy := range workspaceStrategyChoices {
			if strategy == m.workspaceStrategyChoice() {
				current = idx
				break
			}
		}
		next := workspaceStrategyChoices[wrapIndex(current+delta, len(workspaceStrategyChoices))]
		if m.workspaceEdit.Strategy == profile.WorkspaceStrategySequential && m.workspaceEdit.GroupSize > 0 {
			m.workspaceEdit.LastSequentialGroupSize = m.workspaceEdit.GroupSize
		}
		if next == workspaceStrategyOff {
			m.workspaceEdit.Enabled = false
			return
		}
		m.workspaceEdit.Enabled = true
		if next == profile.WorkspaceStrategySequential && m.workspaceEdit.Strategy != profile.WorkspaceStrategySequential {
			if m.workspaceEdit.LastSequentialGroupSize <= 0 {
				m.workspaceEdit.LastSequentialGroupSize = defaultWorkspaceGroupSize
			}
			m.workspaceEdit.GroupSize = m.workspaceEdit.LastSequentialGroupSize
		}
		if next == profile.WorkspaceStrategyManual && !m.workspaceEdit.ManualRulesInitialized {
			m.workspaceEdit.Rules = m.materializeManualWorkspaceRules()
			m.workspaceEdit.ManualRulesInitialized = len(m.workspaceEdit.Rules) > 0
		}
		m.workspaceEdit.Strategy = next
	case 1:
		if !m.workspaceEdit.Enabled {
			return
		}
		next := adjustPositiveInt(m.workspaceEdit.MaxWorkspaces, delta)
		if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual {
			m.resizeManualWorkspaceRules(next)
		}
		m.workspaceEdit.MaxWorkspaces = next
	case 2:
		if !m.workspaceEdit.Enabled || m.workspaceEdit.Strategy != profile.WorkspaceStrategySequential {
			return
		}
		m.workspaceEdit.GroupSize = adjustPositiveInt(m.workspaceEdit.GroupSize, delta)
		m.workspaceEdit.LastSequentialGroupSize = m.workspaceEdit.GroupSize
	case 3:
		if m.workspaceEdit.Enabled && m.workspaceEdit.Strategy != profile.WorkspaceStrategyManual {
			m.workspaceEdit.PersistAll = !m.workspaceEdit.PersistAll
		}
	}
}

func (m Model) workspaceItemCount() int {
	return len(workspaceFields) + m.workspaceListItemCount()
}

func (m Model) workspaceListItemCount() int {
	if !m.workspaceEdit.Enabled {
		return 0
	}
	if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual {
		return len(m.workspaceEdit.Rules)
	}
	return len(m.workspaceEdit.MonitorOrder)
}

func (m *Model) setWorkspaceSelection(selected int) {
	total := m.workspaceItemCount()
	if total <= 0 {
		m.workspaceEdit.SelectedField = 0
		m.workspaceEdit.SelectedOrder = 0
		return
	}
	m.workspaceEdit.SelectedField = clampInt(selected, 0, total-1)
	if m.workspaceEdit.SelectedField >= len(workspaceFields) {
		m.workspaceEdit.SelectedOrder = m.workspaceEdit.SelectedField - len(workspaceFields)
	}
}

func (m *Model) moveWorkspaceSelection(delta int, wrap bool) {
	total := m.workspaceItemCount()
	if total <= 0 {
		return
	}
	next := m.workspaceEdit.SelectedField + delta
	if wrap {
		next = wrapIndex(next, total)
	}
	m.setWorkspaceSelection(next)
}

func (m Model) workspacePageStep() int {
	inner := m.workspaceSettingsRect().inner(m.styles.activePane)
	return max(1, inner.h-2)
}

func (m *Model) adjustWorkspaceItem(delta int) {
	if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual {
		m.moveManualWorkspaceRule(delta)
		return
	}
	m.moveWorkspaceOrder(delta)
}

func (m Model) materializeManualWorkspaceRules() []profile.WorkspaceRule {
	settings := m.workspaceEdit.settings()
	settings.Enabled = true
	if settings.Strategy == profile.WorkspaceStrategyManual || settings.Strategy == "" {
		settings.Strategy = profile.WorkspaceStrategySequential
	}
	p := profile.Profile{Outputs: m.currentProfileOutputs(), Workspaces: settings}
	return normalizeManualWorkspaceDefaults(profile.ResolveWorkspaceRules(p, nil))
}

func (m Model) manualWorkspaceOutputKeys() []string {
	available := make(map[string]bool, len(m.editOutputs))
	for _, output := range m.editOutputs {
		if output.Enabled && output.MirrorOf == "" {
			available[output.Key] = true
		}
	}

	keys := make([]string, 0, len(available))
	seen := make(map[string]bool, len(available))
	for _, key := range m.workspaceEdit.MonitorOrder {
		if available[key] && !seen[key] {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	for _, output := range m.editOutputs {
		if available[output.Key] && !seen[output.Key] {
			keys = append(keys, output.Key)
			seen[output.Key] = true
		}
	}
	return keys
}

func (m Model) manualWorkspaceRuleOutputLabel(rule profile.WorkspaceRule) string {
	if rule.OutputKey != "" {
		if label := m.outputLabelForKey(rule.OutputKey); label != rule.OutputKey {
			return label
		}
	}
	return blankFallback(rule.OutputName, rule.OutputKey)
}

func (m *Model) moveManualWorkspaceRule(delta int) {
	idx := m.workspaceEdit.SelectedOrder
	if idx < 0 || idx >= len(m.workspaceEdit.Rules) {
		return
	}
	keys := m.manualWorkspaceOutputKeys()
	if len(keys) == 0 {
		return
	}

	rule := &m.workspaceEdit.Rules[idx]
	current := -1
	for pos, key := range keys {
		if key == rule.OutputKey {
			current = pos
			break
		}
	}
	if current < 0 {
		current = 0
		if delta < 0 {
			current = len(keys) - 1
		}
	} else {
		current = wrapIndex(current+delta, len(keys))
	}

	rule.OutputKey = keys[current]
	rule.OutputName = outputConnector(keys[current], m.currentProfileOutputs())
	m.workspaceEdit.Rules = normalizeManualWorkspaceDefaults(m.workspaceEdit.Rules)
}

func (m *Model) resizeManualWorkspaceRules(maximum int) {
	byWorkspace := make(map[string]profile.WorkspaceRule, len(m.workspaceEdit.Rules))
	named := make([]profile.WorkspaceRule, 0, len(m.workspaceEdit.Rules))
	for _, rule := range m.workspaceEdit.Rules {
		number, err := strconv.Atoi(strings.TrimSpace(rule.Workspace))
		if err != nil || number < 1 {
			named = append(named, rule)
			continue
		}
		if number <= maximum {
			byWorkspace[strconv.Itoa(number)] = rule
		}
	}

	keys := m.manualWorkspaceOutputKeys()
	rules := make([]profile.WorkspaceRule, 0, maximum+len(named))
	for number := 1; number <= maximum; number++ {
		workspace := strconv.Itoa(number)
		rule, ok := byWorkspace[workspace]
		if !ok {
			rule.Workspace = workspace
			if len(keys) > 0 {
				rule.OutputKey = keys[0]
				rule.OutputName = outputConnector(keys[0], m.currentProfileOutputs())
			}
		}
		rules = append(rules, rule)
	}
	rules = append(rules, named...)
	m.workspaceEdit.Rules = normalizeManualWorkspaceDefaults(rules)
}

func normalizeManualWorkspaceDefaults(rules []profile.WorkspaceRule) []profile.WorkspaceRule {
	normalized := append([]profile.WorkspaceRule(nil), rules...)
	sort.SliceStable(normalized, func(i, j int) bool {
		left, leftErr := strconv.Atoi(strings.TrimSpace(normalized[i].Workspace))
		right, rightErr := strconv.Atoi(strings.TrimSpace(normalized[j].Workspace))
		if leftErr == nil && rightErr == nil {
			return left < right
		}
		if leftErr == nil {
			return true
		}
		if rightErr == nil {
			return false
		}
		return normalized[i].Workspace < normalized[j].Workspace
	})

	seen := make(map[string]bool, len(normalized))
	for idx := range normalized {
		target := normalized[idx].OutputKey
		if target == "" {
			target = normalized[idx].OutputName
		}
		normalized[idx].Default = false
		if target != "" && !seen[target] {
			normalized[idx].Default = true
			normalized[idx].Persistent = true
			seen[target] = true
		}
	}
	return normalized
}

func (m *Model) moveWorkspaceOrder(delta int) {
	idx := m.workspaceEdit.SelectedOrder
	next := idx + delta
	if idx < 0 || idx >= len(m.workspaceEdit.MonitorOrder) || next < 0 || next >= len(m.workspaceEdit.MonitorOrder) {
		return
	}
	m.workspaceEdit.MonitorOrder[idx], m.workspaceEdit.MonitorOrder[next] = m.workspaceEdit.MonitorOrder[next], m.workspaceEdit.MonitorOrder[idx]
	m.workspaceEdit.SelectedOrder = next
	m.workspaceEdit.SelectedField = len(workspaceFields) + next
}
