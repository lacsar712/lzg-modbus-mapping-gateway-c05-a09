// Package faultinject provides a DEV-ONLY decorator around port.ModbusClient
// that can simulate a device disconnect. It is inert unless explicitly enabled
// at startup via DEV_FAULT_INJECTION=1; the toggle defaults to off.
package faultinject

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/bytecode/modbus-mapping-gateway/internal/port"
)

// Client wraps a ModbusClient and fails every call for the targeted endpoint
// while injection is enabled. Endpoint="" means inject for all endpoints.
type Client struct {
	inner     port.ModbusClient
	supported bool

	enabled atomic.Bool
	target  atomic.Value // string endpoint, "" = all
}

// New returns the decorator. supported must reflect DEV_FAULT_INJECTION at
// startup; when false (the default), SetEnabled refuses to turn injection on
// and the diagnostics API reports the capability as unavailable.
func New(inner port.ModbusClient, supported bool) *Client {
	c := &Client{inner: inner, supported: supported}
	c.target.Store("")
	return c
}

func (c *Client) fail(endpoint, op string) error {
	if !c.enabled.Load() {
		return nil
	}
	target, _ := c.target.Load().(string)
	if target != "" && target != endpoint {
		return nil
	}
	return fmt.Errorf("fault injection: simulated %s failure for %s", op, endpoint)
}

func (c *Client) Ping(endpoint string, unitID byte, timeoutMs int) (time.Duration, error) {
	if err := c.fail(endpoint, "connection"); err != nil {
		// report a short, deterministic probe duration for the failed attempt
		return 0, err
	}
	return c.inner.Ping(endpoint, unitID, timeoutMs)
}

func (c *Client) ReadHoldingRegisters(endpoint string, unitID byte, timeoutMs int, address, quantity uint16) ([]uint16, error) {
	if err := c.fail(endpoint, "read"); err != nil {
		return nil, err
	}
	return c.inner.ReadHoldingRegisters(endpoint, unitID, timeoutMs, address, quantity)
}

func (c *Client) WriteSingleRegister(endpoint string, unitID byte, timeoutMs int, address, value uint16) error {
	if err := c.fail(endpoint, "write"); err != nil {
		return err
	}
	return c.inner.WriteSingleRegister(endpoint, unitID, timeoutMs, address, value)
}

func (c *Client) WriteMultipleRegisters(endpoint string, unitID byte, timeoutMs int, address uint16, values []uint16) error {
	if err := c.fail(endpoint, "write"); err != nil {
		return err
	}
	return c.inner.WriteMultipleRegisters(endpoint, unitID, timeoutMs, address, values)
}

// SetEnabled toggles simulated disconnect. An empty target applies to all devices.
// It never enables injection unless the decorator was built with support on.
func (c *Client) SetEnabled(on bool, endpoint string) {
	c.target.Store(endpoint)
	c.enabled.Store(on && c.supported)
}

// Supported reports whether fault injection is compiled in for this process
// (gated by DEV_FAULT_INJECTION, default false).
func (c *Client) Supported() bool { return c.supported }

// Enabled reports whether a fault is currently injected.
func (c *Client) Enabled() bool { return c.enabled.Load() }

// Target returns the targeted endpoint ("" = all devices).
func (c *Client) Target() string {
	t, _ := c.target.Load().(string)
	return t
}
