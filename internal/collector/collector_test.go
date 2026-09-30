package collector

import (
	"testing"
	"time"
)

func TestMetricsCollector_Lifecycle(t *testing.T) {
	tempDir := t.TempDir()
	c := NewMetricsCollector(tempDir)
	c.Start()
	defer c.Stop()

	// Wait briefly for sampling
	time.Sleep(100 * time.Millisecond)

	snap := c.GetSnapshot()
	if snap.Host.NumCPU <= 0 {
		t.Errorf("Expected NumCPU > 0, got %d", snap.Host.NumCPU)
	}

	if snap.Storage.TotalGB <= 0 {
		t.Logf("Storage TotalGB: %f (virtual/temp fs might report 0)", snap.Storage.TotalGB)
	}

	if snap.Container.ProcessAllocMB < 0 {
		t.Errorf("Expected non-negative ProcessAllocMB, got %f", snap.Container.ProcessAllocMB)
	}
}
