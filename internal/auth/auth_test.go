package auth_test

import (
	"testing"

	"github.com/brendanv/float/internal/auth"
)

func TestAuth(t *testing.T) {
	tests := []struct {
		name        string
		passphrase  string
		wantEnabled bool
	}{
		{name: "enabled with passphrase", passphrase: "hunter2", wantEnabled: true},
		{name: "disabled with empty passphrase", passphrase: "", wantEnabled: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := auth.New(tt.passphrase)
			if a.Enabled() != tt.wantEnabled {
				t.Fatalf("Enabled() = %v, want %v", a.Enabled(), tt.wantEnabled)
			}
			if tt.wantEnabled {
				if a.Token() == "" {
					t.Error("Token() empty for enabled auth")
				}
				if !a.VerifyPassphrase(tt.passphrase) {
					t.Error("VerifyPassphrase rejected correct passphrase")
				}
				if !a.VerifyToken(a.Token()) {
					t.Error("VerifyToken rejected own token")
				}
				if !a.VerifyCredential(tt.passphrase) || !a.VerifyCredential(a.Token()) {
					t.Error("VerifyCredential rejected valid credentials")
				}
			} else {
				if a.Token() != "" {
					t.Errorf("Token() = %q for disabled auth, want empty", a.Token())
				}
			}
			// Wrong/empty credentials are always rejected, enabled or not.
			for _, bad := range []string{"wrong", ""} {
				if a.VerifyPassphrase(bad) && bad != tt.passphrase {
					t.Errorf("VerifyPassphrase(%q) = true", bad)
				}
				if a.VerifyToken(bad) {
					t.Errorf("VerifyToken(%q) = true", bad)
				}
			}
		})
	}
}

func TestTokenDeterministic(t *testing.T) {
	a1 := auth.New("same-pass")
	a2 := auth.New("same-pass")
	a3 := auth.New("other-pass")
	if a1.Token() != a2.Token() {
		t.Error("same passphrase produced different tokens")
	}
	if a1.Token() == a3.Token() {
		t.Error("different passphrases produced the same token")
	}
	if a1.VerifyToken("same-pass") {
		t.Error("raw passphrase accepted as session token")
	}
}
