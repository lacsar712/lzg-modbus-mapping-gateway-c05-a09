package usecase

import (
	"fmt"
	"time"

	"github.com/bytecode/modbus-mapping-gateway/internal/domain"
)

// maxRecentProbes bounds the in-memory probe history shown on the diagnostics page.
const maxRecentProbes = 20

// FaultController is implemented by the dev-only fault injection decorator.
// It is optional: when no controller is wired, fault endpoints report
// supported=false and toggling is rejected.
type FaultController interface {
	SetEnabled(on bool, endpoint string)
	Supported() bool
	Enabled() bool
	Target() string
}

// SetFaultController wires the optional dev-only fault injection switch.
func (s *GatewayService) SetFaultController(fc FaultController) {
	s.fault = fc
}

// ProbeDevice runs one connectivity probe against a device (TCP dial + FC03
// read of register 0), records the result and returns it. A failed probe is a
// normal ProbeResult with OK=false, not a Go error — the error return is
// reserved for an unknown device.
func (s *GatewayService) ProbeDevice(deviceID string) (domain.ProbeResult, error) {
	cfg := s.Config()
	d, err := cfg.FindDevice(deviceID)
	if err != nil {
		return domain.ProbeResult{}, err
	}
	latency, perr := s.modbus.Ping(d.Endpoint, d.UnitID, d.TimeoutMs)
	res := domain.ProbeResult{
		DeviceID:  d.ID,
		Endpoint:  d.Endpoint,
		LatencyMs: float64(latency.Microseconds()) / 1000,
		CheckedAt: time.Now().Format(time.RFC3339),
	}
	if perr != nil {
		res.OK = false
		res.Error = perr.Error()
	} else {
		res.OK = true
	}
	s.recordProbe(res)
	return res, nil
}

func (s *GatewayService) recordProbe(res domain.ProbeResult) {
	s.probesMu.Lock()
	defer s.probesMu.Unlock()
	if s.lastProbe == nil {
		s.lastProbe = map[string]domain.ProbeResult{}
	}
	s.lastProbe[res.DeviceID] = res
	s.recentProbes = append(s.recentProbes, res)
	if len(s.recentProbes) > maxRecentProbes {
		s.recentProbes = s.recentProbes[len(s.recentProbes)-maxRecentProbes:]
	}
}

// RecentProbes returns the most recent probe attempts, newest last.
func (s *GatewayService) RecentProbes() []domain.ProbeResult {
	s.probesMu.Lock()
	defer s.probesMu.Unlock()
	out := make([]domain.ProbeResult, len(s.recentProbes))
	copy(out, s.recentProbes)
	return out
}

func (s *GatewayService) probeSummaryLocked() []domain.ProbeSummary {
	if len(s.lastProbe) == 0 {
		return nil
	}
	cfg := s.cfg
	out := make([]domain.ProbeSummary, 0, len(cfg.Devices))
	for _, d := range cfg.Devices {
		r, ok := s.lastProbe[d.ID]
		if !ok {
			continue
		}
		out = append(out, domain.ProbeSummary{
			DeviceID:  r.DeviceID,
			Endpoint:  r.Endpoint,
			OK:        r.OK,
			LatencyMs: r.LatencyMs,
			Error:     r.Error,
			CheckedAt: r.CheckedAt,
		})
	}
	return out
}

// FaultStatus reports the dev-only fault injection switch state.
func (s *GatewayService) FaultStatus() domain.FaultStatus {
	st := domain.FaultStatus{Supported: false}
	if s.fault != nil {
		st.Supported = s.fault.Supported()
		st.Enabled = s.fault.Enabled()
		st.Target = s.fault.Target()
	}
	return st
}

// SetFault toggles simulated disconnect. Empty deviceID injects for all
// devices; otherwise the device must exist in the current mapping.
func (s *GatewayService) SetFault(on bool, deviceID string) error {
	if s.fault == nil || !s.fault.Supported() {
		return fmt.Errorf("fault injection disabled: start backend with DEV_FAULT_INJECTION=1")
	}
	endpoint := ""
	if deviceID != "" {
		d, err := s.Config().FindDevice(deviceID)
		if err != nil {
			return err
		}
		endpoint = d.Endpoint
	}
	s.fault.SetEnabled(on, endpoint)
	return nil
}
