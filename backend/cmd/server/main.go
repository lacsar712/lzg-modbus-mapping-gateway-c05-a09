package main

import (
	"log"
	"os"
	"strings"

	"github.com/bytecode/modbus-mapping-gateway/internal/adapter/in/httpapi"
	"github.com/bytecode/modbus-mapping-gateway/internal/adapter/out/faultinject"
	"github.com/bytecode/modbus-mapping-gateway/internal/adapter/out/modbus"
	"github.com/bytecode/modbus-mapping-gateway/internal/adapter/out/yamlstore"
	"github.com/bytecode/modbus-mapping-gateway/internal/port"
	"github.com/bytecode/modbus-mapping-gateway/internal/usecase"
)

func main() {
	addr := env("HTTP_ADDR", ":8080")
	mappingFile := env("MAPPING_FILE", "configs/mapping.yaml")
	defaultFile := env("MAPPING_DEFAULT", "configs/mapping.yaml")

	if err := yamlstore.EnsureDefault(mappingFile, defaultFile); err != nil {
		// if same path, ignore; otherwise try continue when file already exists
		if mappingFile != defaultFile {
			log.Printf("ensure default mapping: %v", err)
		}
	}

	store := yamlstore.New(mappingFile)
	rawClient := modbus.NewClient()

	// DEV-ONLY fault injection (simulated device disconnect). Disabled unless
	// DEV_FAULT_INJECTION is explicitly truthy; never set it in production.
	faultClient := faultinject.New(rawClient, truthy(os.Getenv("DEV_FAULT_INJECTION")))
	if faultClient.Supported() {
		log.Printf("WARNING: dev-only fault injection ENABLED (DEV_FAULT_INJECTION=1)")
	}

	var mbClient port.ModbusClient = faultClient
	svc, err := usecase.NewGatewayService(store, mbClient)
	if err != nil {
		log.Fatalf("load mapping: %v", err)
	}
	svc.SetFaultController(faultClient)

	srv := httpapi.NewServer(svc)
	log.Printf("modbus mapping gateway listening on %s, mapping=%s", addr, mappingFile)
	if err := srv.Router().Run(addr); err != nil {
		log.Fatal(err)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
