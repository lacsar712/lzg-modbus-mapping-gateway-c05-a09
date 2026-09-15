package usecase

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var errSimulated = errors.New("simulated failure")

const probeMapping = `
devices:
  - id: plc-a
    name: PLC A
    endpoint: plc-a:5020
    unitId: 1
    timeoutMs: 500
    points:
      - name: rpm
        address: 0
        type: uint16
        writable: false
        scale: 1
        offset: 0
`

type probeFakeModbus struct {
	failEndpoints map[string]bool
	pingCalls     int
}

func (f *probeFakeModbus) Ping(endpoint string, _ byte, _ int) (time.Duration, error) {
	f.pingCalls++
	if f.failEndpoints[endpoint] {
		return 2 * time.Millisecond, errSimulated
	}
	return 3 * time.Millisecond, nil
}
func (f *probeFakeModbus) ReadHoldingRegisters(string, byte, int, uint16, uint16) ([]uint16, error) {
	return []uint16{0}, nil
}
func (f *probeFakeModbus) WriteSingleRegister(string, byte, int, uint16, uint16) error {
	return nil
}
func (f *probeFakeModbus) WriteMultipleRegisters(string, byte, int, uint16, []uint16) error {
	return nil
}

type probeFault struct {
	supported bool
	enabled   bool
	target    string
}

func (p *probeFault) SetEnabled(on bool, endpoint string) {
	p.enabled = on && p.supported
	p.target = endpoint
}
func (p *probeFault) Supported() bool { return p.supported }
func (p *probeFault) Enabled() bool   { return p.enabled }
func (p *probeFault) Target() string  { return p.target }

func newProbeService(t *testing.T, mb *probeFakeModbus, fc FaultController) *GatewayService {
	t.Helper()
	store := &memStore{text: probeMapping}
	svc, err := NewGatewayService(store, mb)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if fc != nil {
		svc.SetFaultController(fc)
	}
	return svc
}

func TestProbeDeviceSuccessAndFailure(t *testing.T) {
	mb := &probeFakeModbus{failEndpoints: map[string]bool{}}
	svc := newProbeService(t, mb, nil)

	res, err := svc.ProbeDevice("plc-a")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if !res.OK || res.LatencyMs < 0 || res.Error != "" {
		t.Fatalf("unexpected successful probe: %+v", res)
	}
	if h := svc.Health(); h.Status != "ok" || len(h.ProbeSummary) != 1 || !h.ProbeSummary[0].OK {
		t.Fatalf("health should aggregate one ok probe, got %+v", h)
	}

	mb.failEndpoints["plc-a:5020"] = true
	res, err = svc.ProbeDevice("plc-a")
	if err != nil {
		t.Fatalf("failed probe should not be a Go error: %v", err)
	}
	if res.OK || !strings.Contains(res.Error, "simulated") {
		t.Fatalf("expected failed probe result, got %+v", res)
	}
	if h := svc.Health(); h.Status != "degraded" || len(h.ProbeSummary) != 1 || h.ProbeSummary[0].OK {
		t.Fatalf("health should be degraded with failing latest probe, got %+v", h)
	}

	if _, err := svc.ProbeDevice("missing"); err == nil {
		t.Fatal("unknown device must return error")
	}
}

func TestRecentProbesBounded(t *testing.T) {
	mb := &probeFakeModbus{}
	svc := newProbeService(t, mb, nil)
	for i := 0; i < maxRecentProbes+5; i++ {
		if _, err := svc.ProbeDevice("plc-a"); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(svc.RecentProbes()); got != maxRecentProbes {
		t.Fatalf("recent probes should be bounded to %d, got %d", maxRecentProbes, got)
	}
}

func TestFaultToggleUnsupportedByDefault(t *testing.T) {
	mb := &probeFakeModbus{}
	svc := newProbeService(t, mb, nil)
	if svc.FaultStatus().Supported {
		t.Fatal("fault injection must default to unsupported")
	}
	if err := svc.SetFault(true, ""); err == nil {
		t.Fatal("enabling without support must fail")
	}
}

func TestFaultToggleRoundTrip(t *testing.T) {
	mb := &probeFakeModbus{}
	fc := &probeFault{supported: true}
	svc := newProbeService(t, mb, fc)

	if err := svc.SetFault(true, "plc-a"); err != nil {
		t.Fatalf("enable fault: %v", err)
	}
	st := svc.FaultStatus()
	if !st.Enabled || st.Target != "plc-a:5020" {
		t.Fatalf("unexpected fault status: %+v", st)
	}
	if err := svc.SetFault(false, "plc-a"); err != nil {
		t.Fatalf("disable fault: %v", err)
	}
	if svc.FaultStatus().Enabled {
		t.Fatal("fault should be disabled")
	}
	if err := svc.SetFault(true, "missing"); err == nil {
		t.Fatal("targeting unknown device must fail")
	}
}
