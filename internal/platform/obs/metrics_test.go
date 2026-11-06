package obs_test

import (
	"testing"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
)

func TestOBS_BuildInfoMetricCarriesVersionAndCommit(t *testing.T) {
	m := obs.NewMetrics("v0.1.0", "a1b2c3d")
	families, err := m.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != "linochi_build_info" {
			continue
		}
		got := f.GetMetric()[0]
		labels := map[string]string{}
		for _, l := range got.GetLabel() {
			labels[l.GetName()] = l.GetValue()
		}
		if labels["version"] != "v0.1.0" || labels["commit"] != "a1b2c3d" || got.GetGauge().GetValue() != 1 {
			t.Errorf("unexpected build_info: %v value=%v", labels, got.GetGauge().GetValue())
		}
		return
	}
	t.Fatal("linochi_build_info not found")
}

func TestOBS_RegistryHasGoRuntimeMetrics(t *testing.T) {
	families, err := obs.NewMetrics("dev", "unknown").Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var goroutines bool
	for _, f := range families {
		if f.GetName() == "go_goroutines" {
			goroutines = true
		}
	}
	if !goroutines {
		t.Error("go_goroutines is missing: the Go runtime collector is not registered")
	}
}
