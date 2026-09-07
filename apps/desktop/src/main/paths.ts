import { app } from 'electron';
import * as path from 'node:path';
import * as fs from 'node:fs';

// Where the Go sidecar binary and the static (wasm) dir override live. Env
// overrides win, then the packaged resources under process.resourcesPath, then
// the dev tree.
//
// Dev layout, running `electron .` from apps/desktop:
//   <repo>/apps/desktop/dist/main/index.js   app path is apps/desktop
//   <repo>/gridwell                          sidecar binary
//   <repo>/web                               static assets

function repoRoot(): string {
  // apps/desktop up two levels is the repo root in the dev tree.
  return path.resolve(app.getAppPath(), '..', '..');
}

export function sidecarBinary(): string {
  const env = process.env.GRIDWELL_SIDECAR;
  if (env && fs.existsSync(env)) return env;

  // Windows names a built binary gridwell.exe; see exeSuffixFor in
  // internal/cli/serve.go, which owns the same fact for the plugin binaries.
  // GRIDWELL_SIDECAR is a full path, so no suffix applies to it.
  const name = process.platform === 'win32' ? 'gridwell.exe' : 'gridwell';
  const packaged = path.join(process.resourcesPath ?? '', name);
  if (fs.existsSync(packaged)) return packaged;

  const dev = path.join(repoRoot(), name);
  return dev;
}

// staticDir is the GRIDWELL_STATIC override only; null means none. The
// gridwell binary embeds the web client (web/embed.go), so the server needs no
// --static in either layout. The override is for the e2e harness and for
// iterating on web/ without rebuilding the binary.
export function staticDir(): string | null {
  const env = process.env.GRIDWELL_STATIC;
  if (env && fs.existsSync(env)) return env;
  return null;
}
