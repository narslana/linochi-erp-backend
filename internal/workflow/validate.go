package workflow

import (
	"fmt"
	"slices"
)

func phaseOf(s StepDef) string {
	if s.StockPhase == "" {
		return PhaseNone
	}
	return s.StockPhase
}

// effectFor is the effect a transition between two phases must carry, or "" for none.
func effectFor(from, to string) string {
	switch {
	case from == PhaseNone && to == PhaseReserved:
		return EffectReserve
	case from == PhaseReserved && to == PhaseShipped:
		return EffectShip
	case from == PhaseReserved && to == PhaseNone:
		return EffectRelease
	}
	return ""
}

// Validate checks a flow before it is published (WF-06). It returns every
// problem found, not just the first. A nil result means the flow is valid.
//   - exactly one start step, and no transition into it (the way back is an override)
//   - step keys are unique and every transition joins two existing steps
//   - every non-final step reaches a locked final step (delivered)
//   - the effect on a transition matches the stock phases of its two steps
func Validate(d VersionDefinition) []string {
	var problems []string
	byKey := map[string]StepDef{}
	for _, s := range d.Steps {
		if _, dup := byKey[s.Key]; dup {
			problems = append(problems, fmt.Sprintf("step %q appears more than once", s.Key))
		}
		byKey[s.Key] = s
		if s.IsLocked && !s.IsFinal {
			problems = append(problems, fmt.Sprintf("step %q is locked but not final", s.Key))
		}
		if !slices.Contains([]string{"", PhaseNone, PhaseReserved, PhaseShipped}, s.StockPhase) {
			problems = append(problems, fmt.Sprintf("step %q has unknown stock phase %q", s.Key, s.StockPhase))
		}
	}

	var start []string
	for _, s := range d.Steps {
		if s.IsStart {
			start = append(start, s.Key)
		}
	}
	if len(start) != 1 {
		problems = append(problems, fmt.Sprintf("a flow needs exactly one start step, found %d", len(start)))
	}

	next := map[string][]string{}
	for _, tr := range d.Transitions {
		from, okF := byKey[tr.From]
		to, okT := byKey[tr.To]
		if !okF || !okT {
			problems = append(problems, fmt.Sprintf("transition %q to %q uses a step that does not exist", tr.From, tr.To))
			continue
		}
		if to.IsStart {
			problems = append(problems, fmt.Sprintf("transition %q to %q goes into the start step: only an override may do that", tr.From, tr.To))
		}
		if from.IsFinal {
			problems = append(problems, fmt.Sprintf("transition %q to %q leaves a final step", tr.From, tr.To))
		}
		if want := effectFor(phaseOf(from), phaseOf(to)); tr.Effect != want {
			problems = append(problems, fmt.Sprintf("transition %q to %q has effect %q, its stock phases need %q", tr.From, tr.To, tr.Effect, want))
		}
		next[tr.From] = append(next[tr.From], tr.To)
	}

	// Every non-final step must reach a locked final step.
	for _, s := range d.Steps {
		if s.IsFinal {
			continue
		}
		seen := map[string]bool{s.Key: true}
		queue := []string{s.Key}
		reaches := false
		for len(queue) > 0 && !reaches {
			cur := queue[0]
			queue = queue[1:]
			for _, n := range next[cur] {
				if seen[n] {
					continue
				}
				seen[n] = true
				if byKey[n].IsFinal && byKey[n].IsLocked {
					reaches = true
					break
				}
				queue = append(queue, n)
			}
		}
		if !reaches {
			problems = append(problems, fmt.Sprintf("step %q cannot reach a locked final step (delivered)", s.Key))
		}
	}
	return problems
}
