package dial

// The knownhosts algorithm bridge. Strict verification reports a key mismatch
// when negotiation picks a host-key type known_hosts does not hold, so
// hostKeyAlgorithmsFor recovers the trusted types for ClientConfig to offer.

import (
	"crypto/ed25519"
	"errors"
	"net"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// hostKeyAlgorithmsFor returns the host-key algorithms known_hosts already
// trusts for host ("host:port"). It returns nil for an unknown host, so the
// default negotiation runs and the unknown-host error surfaces normally.
func hostKeyAlgorithmsFor(cb ssh.HostKeyCallback, host string) []string {
	// A file that happens to hold this fresh key's match is not worth handling.
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil
	}
	// The callback wants the "host:port" form the ssh library passes.
	probeErr := cb(host, &net.TCPAddr{IP: net.IPv4zero, Port: 22}, signer.PublicKey())
	var ke *knownhosts.KeyError
	if !errors.As(probeErr, &ke) || len(ke.Want) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var algos []string
	for _, kk := range ke.Want {
		t := kk.Key.Type()
		if !seen[t] {
			seen[t] = true
			algos = append(algos, t)
		}
	}
	return algos
}
