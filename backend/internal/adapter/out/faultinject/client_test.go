package faultinject

import (
	"strings"
	"testing"
	"time"

	"github.com/bytecode/modbus-mapping-gateway/internal/port"
)

type fakeModbus struct {
	pingErr error
}

func (f *fakeModbus) Ping(string, byte, int) (time.Duration, error) {
	return 5 * time.Millisecond, f.pingErr
}
func (f *fakeModbus) ReadHoldingRegisters(string, byte, int, uint16, uint16) ([]uint16, error) {
	return []uint16{1}, f.pingErr
}
func (f *fakeModbus) WriteSingleRegister(string, byte, int, uint16, uint16) error {
	return f.pingErr
}
func (f *fakeModbus) WriteMultipleRegisters(string, byte, int, uint16, []uint16) error {
	return f.pingErr
}

var _ port.ModbusClient = (*fakeModbus)(nil)

func TestUnsupportedStaysOff(t *testing.T) {
	c := New(&fakeModbus{}, false)
	c.SetEnabled(true, "")
	if c.Enabled() {
		t.Fatal("injection must not enable when unsupported")
	}
	if _, err := c.Ping("plc:502", 1, 1000); err != nil {
		t.Fatalf("ping should pass through when disabled: %v", err)
	}
}

func TestEnabledFailsTargetOnly(t *testing.T) {
	c := New(&fakeModbus{}, true)
	c.SetEnabled(true, "plc-a:502")
	if !c.Enabled() {
		t.Fatal("injection should be enabled")
	}
	if _, err := c.Ping("plc-a:502", 1, 1000); err == nil ||
		!strings.Contains(err.Error(), "fault injection") {
		t.Fatalf("targeted endpoint must fail with injection error, got %v", err)
	}
	if _, err := c.Ping("plc-b:502", 1, 1000); err != nil {
		t.Fatalf("non-targeted endpoint must pass through, got %v", err)
	}
	c.SetEnabled(false, "plc-a:502")
	if c.Enabled() {
		t.Fatal("injection should be off after disable")
	}
	if _, err := c.Ping("plc-a:502", 1, 1000); err != nil {
		t.Fatalf("endpoint must recover after disable, got %v", err)
	}
}

func TestEnabledFailsAllEndpoints(t *testing.T) {
	c := New(&fakeModbus{}, true)
	c.SetEnabled(true, "")
	for _, op := range []func() error{
		func() error { _, err := c.ReadHoldingRegisters("x", 1, 1, 0, 1); return err },
		func() error { return c.WriteSingleRegister("x", 1, 1, 0, 1) },
		func() error { return c.WriteMultipleRegisters("x", 1, 1, 0, []uint16{1}) },
	} {
		if err := op(); err == nil {
			t.Fatal("all modbus ops must fail while injecting")
		}
	}
}
