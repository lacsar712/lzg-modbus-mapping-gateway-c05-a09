package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) probeDevice(c *gin.Context) {
	res, err := s.svc.ProbeDevice(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	// A failed probe is reported as 502 so callers can branch on status,
	// but the structured result (latency/error) is still the response body.
	status := http.StatusOK
	if !res.OK {
		status = http.StatusBadGateway
	}
	c.JSON(status, res)
}

func (s *Server) listProbes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"probes": s.svc.RecentProbes()})
}

func (s *Server) getFault(c *gin.Context) {
	c.JSON(http.StatusOK, s.svc.FaultStatus())
}

func (s *Server) setFault(c *gin.Context) {
	if !s.requireEngineer(c) {
		return
	}
	var req struct {
		Enabled  bool   `json:"enabled"`
		DeviceID string `json:"deviceId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body, expect {enabled, deviceId?}"})
		return
	}
	if err := s.svc.SetFault(req.Enabled, req.DeviceID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s.svc.FaultStatus())
}
