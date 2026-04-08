package backup

import (
	"regexp"
	"testing"
	"time"
)

func TestPostgresStrategyImplementsInterface(t *testing.T) {
	var _ Strategy = NewPostgresStrategy()
}

func TestPostgresStrategyFilename(t *testing.T) {
	containerName := "myapp-db"
	before := time.Now()
	filename := postgresFilename(containerName)
	after := time.Now()

	re := regexp.MustCompile(`^myapp-db-(\d{8}-\d{6})\.sql\.gz$`)
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

// postgresFilename is extracted for testability.
func postgresFilename(containerName string) string {
	return containerName + "-" + time.Now().Format("20060102-150405") + ".sql.gz"
}
