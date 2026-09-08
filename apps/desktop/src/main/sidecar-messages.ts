// The sidecar-lifecycle notice that crosses EV.error, kept out of sidecar.ts,
// which imports Electron through ./paths, so `node --test` can exercise it.
// This is for a backend that was already up: the wasm app keeps running, so it
// asks for a restart. A boot failure takes index.ts's dialog instead.
export function sidecarExitMessage(code: number | null, signal: string | null): string {
  const cause = signal ? `signal ${signal}` : `code ${code ?? 'unknown'}`;
  return `backend exited (${cause}) — restart the app`;
}
