package boundary

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// unboundedOK names the client RPCs that are allowed to run on an unbounded
// context, by the source line that opens them. Both are long-lived streams:
// waiting is what they are for, and each has its own re-dial loop behind it.
// Everything else the wasm client calls is a unary RPC with an answer coming
// or not coming, and "not coming" is the case a bound exists for.
var unboundedOK = map[string]string{
	"client/wasm/main.go": "a.cl.Subscribe(context.Background())",
}

// TestClientRPCsAreBounded is the gate behind "a client RPC is bounded", the
// rule client/inflight owns. It reads the shim rather than the packages
// underneath because the shim is where the contexts are spelled, and the shim
// is the one place in the client with no unit tests of its own: `make check`
// compiles client/wasm and executes none of it.
//
// A bare context.Background() on a unary call is how #272 and #298 were both
// written — a read that held its dedupe claim forever, a write that could
// never park — and neither was a decision anyone made; each was a default
// nobody was asked about. Naming the two exceptions is what makes it a
// decision.
func TestClientRPCsAreBounded(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "client", "wasm")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read client/wasm: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		rel := "client/wasm/" + e.Name()
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(line, "context.Background()") {
				continue
			}
			if allowed, ok := unboundedOK[rel]; ok && strings.Contains(line, allowed) {
				continue
			}
			t.Errorf("%s:%d: an unbounded context on a client RPC: %s\n"+
				"a request the network swallows never returns, so a read holds its dedupe claim "+
				"for the life of the page and a write is never acknowledged. Use inflight.Bounded, "+
				"or the fetch set's Context, or add the call to unboundedOK if it is a stream.",
				rel, i+1, strings.TrimSpace(line))
		}
	}
}
