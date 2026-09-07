// Formatting for the sidecar-lifecycle notice that crosses EV.error. Kept out
// of sidecar.ts, which imports Electron through ./paths, so `node --test` can
// exercise the text with no Electron runtime.

// sidecarExitMessage formats the notice shown when the Go backend exits after
// it was already up and serving. The wasm app keeps running, so the message
// asks for a restart. A boot-time failure takes another path: startSidecar's
// promise rejects and boot() in index.ts shows dialog.showErrorBox, because no
// renderer exists yet to draw a notice into.
export function sidecarExitMessage(code: number | null, signal: string | null): string {
  const cause = signal ? `signal ${signal}` : `code ${code ?? 'unknown'}`;
  return `backend exited (${cause}) — restart the app`;
}
