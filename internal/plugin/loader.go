// Package plugin builds the registry from server config. Every entry is a
// content plugin spawned as a subprocess, which is the one way a plugin loads.
// The node constructs its own home and transport around this call, and neither
// is a plugin.
//
// A loaded plugin reaches the registry as a namespace.Namespace, a Go value the
// router calls directly. The plugin.v1 subprocess underneath is supervised here
// (supervisor.go), so this package is the one owner of whether a plugin is
// alive, and the health the strip shows comes from it.
package plugin

import (
	"context"
	"fmt"
	"os"
	"time"

	gridwellv1 "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
	pluginv1 "github.com/josephburnett/gridwell/api/gen/plugin/v1"
	"github.com/josephburnett/gridwell/internal/config"
	"github.com/josephburnett/gridwell/internal/local/store"
	"github.com/josephburnett/gridwell/internal/namespace"
	"github.com/josephburnett/gridwell/internal/pluginhost"
)

// LoadInto registers every content plugin of the server config in reg, keyed by
// its ID: a plugin.v1 subprocess, from a binary, fronted by the pluginhost
// adapter over the node-owned store. Nothing is cached in front of it, since a
// subprocess on this machine is a call away and the node's store is the durable
// memory of what it minted. The node registers its own home and transport
// around this call.
func LoadInto(reg *Registry, cfg *config.ServerConfig, home string, st *store.Store) error {
	for i := range cfg.Plugins {
		pc := &cfg.Plugins[i]
		ns, closer, err := loadPlugin(pc, home, st)
		if err != nil {
			return fmt.Errorf("plugin %q (%s): %w", pc.Kind, pc.ID, err)
		}
		// A failing Info stops the launch, because a plugin without the
		// config it needs must not come up as an empty grid. Info is where
		// the plugin says so, with FailedPrecondition and the reason. The
		// answer itself is discarded, since the router reads a plugin's
		// declarations per request.
		ictx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, ierr := ns.Info(ictx, &gridwellv1.InfoRequest{})
		cancel()
		if ierr != nil {
			if closer != nil {
				closer()
			}
			return fmt.Errorf("plugin %q (%s): %w", pc.Kind, pc.ID, ierr)
		}
		reg.Register(pc.ID, pc.Kind, ns, closer)
		reg.SetLabel(pc.ID, pc.Label)
	}
	return nil
}

// loadPlugin materializes one plugin entry: the supervised subprocess, and the
// adapter joining it with the plugin's namespace of the node's store, as an
// ordinary namespace.Namespace the router calls in-process. home is the Gridwell
// home config.Home() derived, threaded in from the node, and the plugin's state
// directory hangs off it.
//
// The client is built over the supervisor, so a respawn swaps the process
// underneath it and nothing here ever holds a handle to a dead one. The
// supervisor is also the adapter's source of health, which is what the
// namespace's event stream announces.
func loadPlugin(pc *config.PluginConfig, home string, st *store.Store) (namespace.Namespace, func(), error) {
	cfg, err := spawnConfig(pc, home)
	if err != nil {
		return nil, nil, err
	}

	if pc.Binary == "" {
		return nil, nil, fmt.Errorf("kind %q: no binary path", pc.Kind)
	}
	sup, err := Supervise(pc.ID, pc.Kind, pc.Binary, cfg)
	if err != nil {
		return nil, nil, err
	}

	return pluginhost.New(pluginv1.NewPluginClient(sup), st.Namespace(pc.ID), sup), sup.Close, nil
}

// spawnConfig is the config map one plugin is spawned with: its own keys, its
// identity, and state_dir, the private directory the node mints for it at
// <home>/plugins/<id>, 0700. What a plugin keeps there is its own memory of its
// source, under cache.db's contract: disposable, safe to delete, rewarmed by
// use. Nothing here or anywhere else deletes one, because a plugin dropped from
// server.yaml may come back.
//
// An empty home is an error rather than a relative path, so that a plugin never
// writes into whatever directory the node happened to start in.
func spawnConfig(pc *config.PluginConfig, home string) (map[string]string, error) {
	if home == "" {
		return nil, fmt.Errorf("no home directory for the plugin's state_dir")
	}
	dir := config.PluginStateDir(home, pc.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("state dir %s: %w", dir, err)
	}
	cfg := make(map[string]string, len(pc.Config)+3)
	for k, v := range pc.Config {
		cfg[k] = v
	}
	cfg["uuid"] = pc.ID
	cfg["kind"] = pc.Kind
	cfg["state_dir"] = dir
	return cfg, nil
}
