package ping

import (
	"strings"
	"testing"
	"time"

	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

func TestPingPlugin_Metadata(t *testing.T) {
	p := New(&plugins.AppContext{})
	if p.Name() != "ping" {
		t.Errorf("expected plugin name 'ping', got %q", p.Name())
	}

	cmds := p.Commands()
	if len(cmds) != 1 || cmds[0].Name != "ping" {
		t.Fatalf("expected command 'ping', got %v", cmds)
	}
}

func TestBuildPingReport_DistinctTimingLabels(t *testing.T) {
	localDuration := 125 * time.Microsecond
	apiRoundTrip := 154320 * time.Microsecond
	startTime := time.Now().Add(-10 * time.Minute)

	report := BuildPingReport(localDuration, apiRoundTrip, startTime)

	// Verify Telegram round-trip latency label and value
	if !strings.Contains(report, "Telegram API Round-Trip:") {
		t.Errorf("expected report to contain 'Telegram API Round-Trip:', got:\n%s", report)
	}
	if !strings.Contains(report, "154.320 ms") {
		t.Errorf("expected report to contain 154.320 ms API latency, got:\n%s", report)
	}

	// Verify Local processing time label and value
	if !strings.Contains(report, "Local Processing Time:") {
		t.Errorf("expected report to contain 'Local Processing Time:', got:\n%s", report)
	}
	if !strings.Contains(report, "0.125 ms") {
		t.Errorf("expected report to contain 0.125 ms local time, got:\n%s", report)
	}

	// Verify Uptime label
	if !strings.Contains(report, "Bot Uptime:") {
		t.Errorf("expected report to contain 'Bot Uptime:', got:\n%s", report)
	}

	// Verify labels are strictly distinct
	if strings.Count(report, "Telegram API Round-Trip:") != 1 {
		t.Error("expected exactly one Telegram API Round-Trip label")
	}
	if strings.Count(report, "Local Processing Time:") != 1 {
		t.Error("expected exactly one Local Processing Time label")
	}
}
