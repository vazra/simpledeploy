package api

import (
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/vazra/simpledeploy/internal/auth"
)

// Lower bcrypt cost for the api test suite. Many tests create users via
// auth.HashPassword; at the production cost of 12 the suite exceeds the
// 10m -race timeout in CI.
func TestMain(m *testing.M) {
	auth.BcryptCost = bcrypt.MinCost
	m.Run()
}
