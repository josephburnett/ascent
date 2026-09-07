import { spawn, ChildProcess } from 'node:child_process';
import * as fs from 'node:fs';
import { freePort } from './freeport';
import { sidecarBinary, staticDir } from './paths';
import { makeLineSplitter, parseServingLine, windowOrigin } from './lines';

export interface Sidecar {
  // The window origin, read back from the serve banner. Loopback by default,
  // but server.yaml `web.bind` may pin another address, such as a Tailscale IP
  // shared with a phone browser.
  origin: string;
  // The web auth token from the banner; lines.ts owns the banner contract.
  // index.ts pre-sets it as the auth cookie so this window never prompts, since
  // the gate is for other browsers reaching the shared origin.
  auth?: string;
  // external means the server was already running, holding the home's serve
  // lock (internal/cli/servelock.go), and this app only connected to it. child
  // is then the exited probe process: never watch it, and kill nothing on stop.
  external: boolean;
  child: ChildProcess;
  stop: () => void;
}

interface StartOptions {
  // Override the default bind port; otherwise a free ephemeral port is chosen.
  // Passed as --bind-default, so an explicit `web.bind` in server.yaml still
  // wins: the server owns the listen-address decision.
  port?: number;
  // Milliseconds of silence to tolerate before giving up. Every line the
  // sidecar prints resets it, so a slow but talking process, such as a
  // migration chain over real data, is never killed for taking its time.
  silenceMs?: number;
  // Sink for sidecar stdout/stderr lines (defaults to console).
  onLog?: (line: string) => void;
  // noServer: never start a server. Runs `gridwell status` instead of
  // `gridwell serve` and connects to a separately-run one, for the split where
  // `gridwell serve` runs in a terminal and the app is launched with
  // --no-server. Rejects with a clear message when nothing is running.
  noServer?: boolean;
  // Test seams (sidecar.test.ts): a fake child process and fixed paths, so the
  // spawn, ready, error, exit and timeout settle rules run under `node --test`
  // with no binary and no Electron. Production callers leave these unset.
  spawnFn?: (bin: string, args: string[]) => ChildProcess;
  binaryPath?: string;
  staticPath?: string;
}

// startSidecar spawns the Go backend and resolves once it announces its actual
// bound address in the serve banner. It rejects if the process exits first or
// goes silent, and the returned stop() terminates the child.
//
// The wait bounds silence, and every line the sidecar prints resets it. A boot
// step that outlasts the window has to keep talking, because a fixed deadline
// SIGTERMs a live, working server, and killing one mid-write to the store tears
// a home in half.
export async function startSidecar(opts: StartOptions = {}): Promise<Sidecar> {
  const bin = opts.binaryPath ?? sidecarBinary();
  if (!opts.spawnFn && !fs.existsSync(bin)) {
    throw new Error(`sidecar binary not found at ${bin} (set GRIDWELL_SIDECAR)`);
  }
  const port = opts.port ?? (await freePort());

  const onLog = opts.onLog ?? ((l: string) => console.log('[sidecar]', l));
  // No --db, because the server resolves its own database under the Gridwell
  // home (GRIDWELL_HOME, inherited from this process's env, else ~/.gridwell)
  // and mints a missing server.yaml itself.
  //
  // --bind-default gives the ephemeral loopback port only when server.yaml
  // declares no web.bind of its own. A declared one wins, so one server serves
  // both this window and a phone browser on a stable origin.
  //
  // noServer runs `gridwell status`, which starts nothing and only re-emits a
  // running server's banner. The server owns the lock, discovery and home
  // resolution; this process never learns what a home is.
  const args = opts.noServer
    ? ['status']
    : [
        'serve',
        '--bind-default', `127.0.0.1:${port}`,
        // --static only when overridden. The binary embeds the web client
        // (web/embed.go); the dev tree and e2e harness pin their checkout
        // through GRIDWELL_STATIC.
        ...staticArgs(opts.staticPath ?? envStaticDir()),
      ];
  const child = opts.spawnFn
    ? opts.spawnFn(bin, args)
    : spawn(bin, args, { stdio: ['ignore', 'pipe', 'pipe'] });

  const stop = () => {
    if (!child.killed) child.kill('SIGTERM');
  };

  // The server prints its diagnostics to stdout or stderr before exiting.
  // Keeping the tail lets the boot failure dialog say why rather than only an
  // exit code.
  const lastLines: string[] = [];

  return await new Promise<Sidecar>((resolve, reject) => {
    let settled = false;
    const silenceMs = opts.silenceMs ?? 10_000;
    // One timer, re-armed on every line, so the deadline is always silenceMs
    // from the last thing the sidecar said.
    const arm = () =>
      setTimeout(() => {
        if (settled) return;
        settled = true;
        stop();
        reject(new Error(`sidecar went silent for ${silenceMs}ms without reporting ready`));
      }, silenceMs);
    let timer = arm();

    const handleLine = (line: string) => {
      onLog(line);
      if (settled) return;
      clearTimeout(timer);
      timer = arm();
      lastLines.push(line);
      if (lastLines.length > 8) lastLines.shift();
      if (opts.noServer && /^gridwell: not serving\b/.test(line)) {
        settled = true;
        clearTimeout(timer);
        reject(
          new Error(
            'no server is running (--no-server given): start one with `gridwell serve`, then relaunch',
          ),
        );
        return;
      }
      const served = parseServingLine(line);
      if (served) {
        settled = true;
        clearTimeout(timer);
        resolve({
          origin: windowOrigin(served),
          auth: served.auth,
          external: !!served.external,
          child,
          stop: served.external ? () => {} : stop,
        });
      }
    };

    attachLineReader(child.stdout, handleLine);
    attachLineReader(child.stderr, handleLine);

    // A spawn failure emits 'error' on the child instead of 'exit', when the
    // binary is not executable or the fs.existsSync check above raced a
    // removal. Without this listener boot hangs until the silence timer fires
    // with a generic message instead of the real cause.
    child.once('error', (err) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      reject(err);
    });

    child.once('exit', (code, signal) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      const tail = lastLines.length ? `\n${lastLines.join('\n')}` : '';
      reject(new Error(`sidecar exited before ready (code=${code} signal=${signal})${tail}`));
    });
  });
}

// staticArgs maps a static override to serve flags. None means the server
// serves its embedded web client, which is the packaged default.
function staticArgs(dir: string | undefined): string[] {
  return dir ? ['--static', dir] : [];
}

// envStaticDir is the dev and e2e override only. The packaged app passes
// nothing and the embedded client serves.
function envStaticDir(): string | undefined {
  return staticDir() ?? undefined;
}

// attachLineReader wires a stream to the line splitter.
function attachLineReader(stream: NodeJS.ReadableStream | null, cb: (line: string) => void): void {
  if (!stream) return;
  const splitter = makeLineSplitter(cb);
  stream.setEncoding('utf8');
  stream.on('data', (chunk: string) => splitter.push(chunk));
  stream.on('end', () => splitter.flush());
}
