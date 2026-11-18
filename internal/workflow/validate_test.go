package workflow

import (
	"strings"
	"testing"
)

// valid is a small flow: start -> work -> done, with a side end.
func valid() VersionDefinition {
	return VersionDefinition{
		Steps: []StepDef{
			{Key: "start", SortOrder: 1, IsStart: true},
			{Key: "work", SortOrder: 2, StockPhase: PhaseReserved},
			{Key: "done", SortOrder: 3, IsFinal: true, IsLocked: true, StockPhase: PhaseShipped},
			{Key: "dropped", SortOrder: 4, IsFinal: true},
		},
		Transitions: []TransitionDef{
			{From: "start", To: "work", Effect: EffectReserve},
			{From: "work", To: "done", Effect: EffectShip},
			{From: "start", To: "dropped"},
			{From: "work", To: "dropped", Effect: EffectRelease},
		},
	}
}

func TestWF06_ValidateTable(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(d *VersionDefinition)
		want   string // substring of one problem, "" means valid
	}{
		{"valid flow", func(*VersionDefinition) {}, ""},
		{"no start", func(d *VersionDefinition) { d.Steps[0].IsStart = false }, "exactly one start"},
		{"two starts", func(d *VersionDefinition) { d.Steps[1].IsStart = true }, "exactly one start"},
		{"into start", func(d *VersionDefinition) {
			d.Transitions = append(d.Transitions, TransitionDef{From: "work", To: "start", Effect: EffectRelease})
		}, "into the start step"},
		{"unreachable end", func(d *VersionDefinition) {
			d.Steps = append(d.Steps, StepDef{Key: "stuck", SortOrder: 9})
		}, "cannot reach"},
		{"leaves a final step", func(d *VersionDefinition) {
			d.Transitions = append(d.Transitions, TransitionDef{From: "dropped", To: "work", Effect: EffectReserve})
		}, "leaves a final step"},
		{"wrong effect", func(d *VersionDefinition) { d.Transitions[0].Effect = "" }, "stock phases need"},
		{"unknown step", func(d *VersionDefinition) {
			d.Transitions = append(d.Transitions, TransitionDef{From: "start", To: "ghost"})
		}, "does not exist"},
		{"duplicate key", func(d *VersionDefinition) {
			d.Steps = append(d.Steps, StepDef{Key: "work", SortOrder: 8})
		}, "more than once"},
		{"locked but not final", func(d *VersionDefinition) { d.Steps[1].IsLocked = true }, "locked but not final"},
		{"unknown phase", func(d *VersionDefinition) { d.Steps[1].StockPhase = "lost" }, "unknown stock phase"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := valid()
			c.mutate(&d)
			problems := Validate(d)
			if c.want == "" {
				if len(problems) != 0 {
					t.Fatalf("problems = %v, want none", problems)
				}
				return
			}
			for _, p := range problems {
				if strings.Contains(p, c.want) {
					return
				}
			}
			t.Fatalf("problems = %v, want one containing %q", problems, c.want)
		})
	}
}
