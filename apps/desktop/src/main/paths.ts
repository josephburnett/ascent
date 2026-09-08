import { app } from 'electron';
import * as path from 'node:path';
import * as fs from 'node:fs';

// Where the sidecar binary and the static-dir override live: env first, then
// process.resourcesPath, then the dev tree, whose app path is apps/desktop.

function repoRoot(): string {
  return path.resolve(app.getAppPath(), '..', '..');
}

export function sidecarBinary(): string {
  const env = process.env.GRIDWELL_SIDECAR;
  if (env && fs.existsSync(env)) return env;

  // Windows names a built binary gridwell.exe; see exeSuffixFor in
  // internal/cli/serve.go. GRIDWELL_SIDECAR is a full path, so no suffix.
  const name = process.platform === 'win32' ? 'gridwell.exe' : 'gridwell';
  const packaged = path.join(process.resourcesPath ?? '', name);
  if (fs.existsSync(packaged)) return packaged;

  const dev = path.join(repoRoot(), name);
  return dev;
}

// The GRIDWELL_STATIC override only. The gridwell binary embeds the web client
// (web/embed.go), so the override is for the e2e harness and web/ iteration.
export function staticDir(): string | null {
  const env = process.env.GRIDWELL_STATIC;
  if (env && fs.existsSync(env)) return env;
  return null;
}
