package dial

// Pins the algorithm preference hostalgos.go owns: without it a known host
// whose negotiated key type is absent from the file fails as a key mismatch.

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestHostKeyAlgorithmsFromKnownHosts(t *testing.T) {
	// A real ed25519 entry for the host, of the shape ssh saves.
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	khLine := "example.com " + string(ssh.MarshalAuthorizedKey(sshPub))
	path := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(path, []byte(khLine), 0o600); err != nil {
		t.Fatal(err)
	}
	cb, err := knownhosts.New(path)
	if err != nil {
		t.Fatal(err)
	}
	got := hostKeyAlgorithmsFor(cb, "example.com:22")
	if len(got) == 0 || got[0] != "ssh-ed25519" {
		t.Fatalf("algorithms for a known ed25519 host = %v, want ssh-ed25519 first — "+
			"without this the handshake negotiates a key type the file cannot verify (key mismatch)", got)
	}
	if got := hostKeyAlgorithmsFor(cb, "stranger.example:22"); got != nil {
		t.Fatalf("unknown host must not constrain algorithms, got %v", got)
	}
}
