// Package plugintest is the seam harness for a test that needs a plugin.v1
// client.
//
// Spawn is the door for a shipped plugin. A plugin lives in its own repository
// and reaches this one only through compose.LoadPlugin, which spawns
// gridwell-plugin-<kind> and speaks plugin.v1 to it, so a test that needs a real
// plugin behind the adapter spawns the real binary with the config server.yaml
// would have given it. Binary is the one place that locates a built binary and
// Spawn the one place that launches one. Both answer with a t.Fatal naming what
// to build rather than a skip, which would leave the seam unexercised while the
// suite stayed green.
//
// Loopback is the door for a plugin the test itself declares, a stub that
// answers one way so a test can pin what the adapter does with the answer. It is
// a real gRPC server on an in-memory listener, so the marshalling the wire does
// still happens: a message the plugin cannot serialize fails here too, and no
// answer is shared by pointer across the seam.
package plugintest

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/josephburnett/gridwell/api/compose"
	gridwellv1 "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
	pluginv1 "github.com/josephburnett/gridwell/api/gen/plugin/v1"
)

// Loopback serves impl over an in-memory gRPC connection and returns the
// connected client plus its closer. No socket, so nothing outside the process
// can reach it.
func Loopback(impl pluginv1.PluginServer) (pluginv1.PluginClient, func(), error) {
	lis := bufconn.Listen(1 << 20)

	srv := grpc.NewServer()
	pluginv1.RegisterPluginServer(srv, impl)
	go srv.Serve(lis)

	cc, err := grpc.NewClient("passthrough:///plugintest",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }))
	if err != nil {
		srv.Stop()
		return nil, nil, fmt.Errorf("plugintest loopback dial: %w", err)
	}

	closer := func() {
		cc.Close()
		srv.GracefulStop()
	}
	return pluginv1.NewPluginClient(cc), closer, nil
}

// repoRoot finds this repository's root by walking up from the test's
// working directory to the go.mod that declares the root module.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			if strings.HasPrefix(string(data), "module github.com/josephburnett/gridwell\n") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("plugintest: no gridwell go.mod above the test directory")
		}
		dir = parent
	}
}

// Binary resolves gridwell-plugin-<kind> the way the loader does
// (resolveBinary in internal/cli): GRIDWELL_PLUGIN_DIR names the directory, and
// otherwise it is the repository root, which is where `make build` writes the
// binaries it builds out of the plugins repo.
func Binary(t *testing.T, kind string) string {
	t.Helper()
	name := "gridwell-plugin-" + kind
	dir := os.Getenv("GRIDWELL_PLUGIN_DIR")
	if dir == "" {
		dir = repoRoot(t)
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("plugintest: %s not found at %s: run `make plugins` (it builds from PLUGINS_DIR, the gridwell-plugins checkout beside this one) or set GRIDWELL_PLUGIN_DIR", name, path)
	}
	return path
}

// Spawn launches the shipped gridwell-plugin-<kind> with cfg, the production
// spawn down to the config map, and returns the connected client, killed at the
// end of the test.
//
// The guest inherits this process's environment, so the test's own home is
// redirected first. An fs plugin trashes a deleted file into
// $XDG_DATA_HOME/Trash, and a test must never write into the developer's. Its
// state_dir is redirected for the same reason, see withStateDir.
func Spawn(t *testing.T, kind string, cfg map[string]string) pluginv1.PluginClient {
	t.Helper()
	cp, _ := SpawnCloser(t, kind, cfg)
	return cp
}

// SpawnCloser is Spawn with the kill handed back, for a test that must stop the
// plugin mid-test to stage a restart or a crash. The kill also runs at the end
// of the test, and running it twice is harmless.
func SpawnCloser(t *testing.T, kind string, cfg map[string]string) (pluginv1.PluginClient, func()) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	proc, err := compose.LoadPlugin(Binary(t, kind), withStateDir(t, cfg))
	if err != nil {
		t.Fatalf("spawn gridwell-plugin-%s: %v", kind, err)
	}
	t.Cleanup(proc.Kill)
	return pluginv1.NewPluginClient(proc.Conn), proc.Kill
}

// withStateDir copies cfg with a state_dir, the private directory the loader
// hands a plugin in production (<home>/plugins/<id>). Here it is a per-test temp
// directory, so no test writes into a real home. A test that keeps a plugin's
// memory across a restart passes its own directory and this leaves it alone. The
// copy keeps the caller's map its own.
func withStateDir(t *testing.T, cfg map[string]string) map[string]string {
	t.Helper()
	out := make(map[string]string, len(cfg)+1)
	for k, v := range cfg {
		out[k] = v
	}
	if out["state_dir"] == "" {
		out["state_dir"] = t.TempDir()
	}
	return out
}

// Landing is the grid a plugin's single declared collection serves, for a test
// descending into a plugin that has exactly one. A plugin declares no root of
// its own, only one menu entry per collection, so there is no RootGridId to
// read. A plugin with several collections has no single landing, which is why
// this fails rather than picking one.
func Landing(t *testing.T, info *gridwellv1.InfoResponse) string {
	t.Helper()
	if info.RootGridId != "" {
		t.Fatalf("this row names a grid of its own (%q) — it is a node, not a plugin", info.RootGridId)
	}
	if len(info.MenuEntries) != 1 {
		t.Fatalf("%d collections declared, want exactly one to land in: %+v", len(info.MenuEntries), info.MenuEntries)
	}
	return info.MenuEntries[0].GridId
}

// LandingOf is Landing over a handshake's menu row.
func LandingOf(t *testing.T, pl *gridwellv1.PluginInfo) string {
	t.Helper()
	if pl.RootGridId != "" {
		t.Fatalf("row %q names a grid of its own (%q) — it is a node, not a plugin", pl.Label, pl.RootGridId)
	}
	if len(pl.MenuEntries) != 1 {
		t.Fatalf("row %q declares %d collections, want exactly one to land in", pl.Label, len(pl.MenuEntries))
	}
	return pl.MenuEntries[0].GridId
}
