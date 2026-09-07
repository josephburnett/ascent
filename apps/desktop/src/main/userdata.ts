import * as path from 'node:path';
import * as fs from 'node:fs';

// e2eUserDataDir returns the Electron userData path for an environment, or
// null when GRIDWELL_HOME is absent.
//
// userData derives from GRIDWELL_HOME and nothing else. A normal launch does
// not set it and keeps the default ~/.config/gridwell-desktop profile; an e2e
// run gets a private <home>/electron profile and never touches the live app's
// profile or lock file.
function e2eUserDataDir(env: Record<string, string | undefined>): string | null {
  const home = env['GRIDWELL_HOME'];
  if (!home) return null;
  return path.join(home, 'electron');
}

// applyUserDataOverride redirects Electron's userData and sessionData to the
// directory e2eUserDataDir returns. Electron ignores it after app.whenReady(),
// so it must be called before. setPath is app.setPath, passed in so the
// function stays testable with no Electron import.
export function applyUserDataOverride(
  setPath: (name: string, value: string) => void,
  env: Record<string, string | undefined>,
): void {
  const dir = e2eUserDataDir(env);
  if (!dir) return;
  fs.mkdirSync(dir, { recursive: true });
  setPath('userData', dir);
  // sessionData holds the default session's cookies, localStorage, cache and
  // IndexedDB. It inherits userData when unset; setting it keeps the two
  // co-located whatever order the paths resolve in.
  try {
    setPath('sessionData', dir);
  } catch {
    // Electron before 28 does not know 'sessionData'.
  }
}
