package adminauth

import (
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestManagerAuthenticationAndSession(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct horse battery staple"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := New("owner", string(hash), "01234567890123456789012345678901", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !manager.Authenticate("owner", "correct horse battery staple") {
		t.Fatal("expected valid credentials")
	}
	if manager.Authenticate("owner", "wrong") {
		t.Fatal("accepted invalid password")
	}
	now := time.Unix(1_700_000_000, 0)
	token := manager.Issue(now)
	if err := manager.Verify(token, now.Add(59*time.Minute)); err != nil {
		t.Fatalf("verify valid token: %v", err)
	}
	if err := manager.Verify(token, now.Add(time.Hour)); err == nil {
		t.Fatal("accepted expired token")
	}
}
