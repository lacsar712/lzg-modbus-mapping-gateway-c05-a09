package port

import (
	"time"

	"github.com/bytecode/modbus-mapping-gateway/internal/domain"
)

// ModbusClient reads/writes holding registers over Modbus TCP.
type ModbusClient interface {
	ReadHoldingRegisters(endpoint string, unitID byte, timeoutMs int, address, quantity uint16) ([]uint16, error)
	WriteSingleRegister(endpoint string, unitID byte, timeoutMs int, address, value uint16) error
	WriteMultipleRegisters(endpoint string, unitID byte, timeoutMs int, address uint16, values []uint16) error
	// Ping verifies TCP+Modbus reachability of one device (dial + read register 0,
	// qty 1) and returns the round-trip duration. It is used by the diagnostics
	// connectivity probe only.
	Ping(endpoint string, unitID byte, timeoutMs int) (time.Duration, error)
}

// MappingStore loads/saves YAML mapping configuration.
type MappingStore interface {
	Load() (domain.MappingConfig, string, error)
	Save(yamlText string) error
	Path() string
}
