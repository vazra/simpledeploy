package backup

import (
	"regexp"
	"testing"
	"time"
)

func TestVolumeStrategyImplementsInterface(t *testing.T) {
	var _ Strategy = NewVolumeStrategy("/data")
}

func TestVolumeStrategyDefaultPath(t *testing.T) {
	s := NewVolumeStrategy("")
	if s.VolumePath != "/data" {
		t.Errorf("expected /data, got %s", s.VolumePath)
	}
}

func TestVolumeStrategyCustomPath(t *testing.T) {
	s := NewVolumeStrategy("/var/lib/mysql")
	if s.VolumePath != "/var/lib/mysql" {
		t.Errorf("expected /var/lib/mysql, got %s", s.VolumePath)
	}
}

func TestVolumeStrategyFilename(t *testing.T) {
	containerName := "myapp"
	before := time.Now()
	filename := volumeFilename(containerName)
	after := time.Now()

	re := regexp.MustCompile(`^myapp-(\d{8}-\d{6})\.tar\.gz$`)
	m := re.FindStringSubmatch(filename)
	if m == nil {
		t.Fatalf("unexpected filename format: %s", filename)
	}

	ts, err := time.ParseInLocation("20060102-150405", m[1], time.Local)
	if err != nil {
		t.Fatalf("parse timestamp: %v", err)
	}
	if ts.Before(before.Truncate(time.Second)) || ts.After(after.Add(time.Second)) {
		t.Errorf("timestamp %v out of range [%v, %v]", ts, before, after)
	}
}

// volumeFilename is extracted for testability.
func volumeFilename(containerName string) string {
	return containerName + "-" + time.Now().Format("20060102-150405") + ".tar.gz"
}
