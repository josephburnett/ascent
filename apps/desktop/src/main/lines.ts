// Reading the sidecar's stdout. Kept in its own module, with no Electron
// import, so `node --test` can exercise it without booting Electron.

// ServingAddr is the address announced by the serve banner. host is the raw web
// listener host and may be a wildcard ("0.0.0.0", "::" or ""), since Go
// announces a wildcard bind as the dual-stack address "[::]:<port>". auth is
// the web auth token the server prints, so this window authenticates without
// prompting.
interface ServingAddr {
  host: string;
  port: number;
  auth?: string;
  // external means another process holds this home's serve lock (one serve per
  // home; internal/cli/servelock.go). The app connects to that server and must
  // never treat its own exited probe child as the server.
  external?: boolean;
}

// parseServingLine extracts the bound address from the serve banner, or null
// for any other line. The banner is the boot contract with `gridwell serve`;
// servingBanner in internal/cli/serve.go owns its shape:
//   "gridwell: serving on <host>:<port> (static=... plugins=N auth=<hex> federation=<socket path>)"
// The address is the listener's actual bound one, because server.yaml
// `web.bind` may override the sidecar's --bind-default so a phone and this
// window share one origin. federation= is not read: the desktop app reaches
// everything it needs over the web door, and a node may serve no connection
// door at all.
//
// "already serving on" is the same banner re-emitted by a serve that found the
// home's lock held. The address and auth belong to the running holder.
export function parseServingLine(line: string): ServingAddr | null {
  const m = /^gridwell: (already )?serving on (\S+) /.exec(line);
  if (!m) return null;
  const external = !!m[1];
  const addr = m[2];
  const i = addr.lastIndexOf(':');
  if (i < 0) return null;
  const port = Number(addr.slice(i + 1));
  if (!Number.isInteger(port) || port <= 0 || port > 65535) return null;
  let host = addr.slice(0, i);
  if (host.startsWith('[') && host.endsWith(']')) host = host.slice(1, -1); // net.JoinHostPort IPv6 form
  const auth = /\bauth=([0-9a-f]{64})\b/.exec(line)?.[1];
  const out: ServingAddr = { host, port };
  if (auth) out.auth = auth;
  if (external) out.external = true;
  return out;
}

// windowOrigin maps the announced address to the origin the local Electron
// window loads. A wildcard host is reachable locally as loopback; a concrete
// host, such as a Tailscale IP, is kept so the desktop window and a phone
// browser share one origin.
export function windowOrigin(a: ServingAddr): string {
  return `http://${reachableHost(a)}:${a.port}`;
}

function reachableHost(a: ServingAddr): string {
  const wildcard = a.host === '' || a.host === '0.0.0.0' || a.host === '::';
  return wildcard ? '127.0.0.1' : a.host.includes(':') ? `[${a.host}]` : a.host;
}

// makeLineSplitter takes raw stream chunks and calls `cb` once per complete
// newline-delimited line, buffering partial lines across chunk boundaries.
// `.flush()` emits a trailing unterminated line at stream end.
export function makeLineSplitter(cb: (line: string) => void): {
  push: (chunk: string) => void;
  flush: () => void;
} {
  let buf = '';
  return {
    push(chunk: string) {
      buf += chunk;
      let idx: number;
      while ((idx = buf.indexOf('\n')) >= 0) {
        const line = buf.slice(0, idx).replace(/\r$/, '');
        buf = buf.slice(idx + 1);
        cb(line);
      }
    },
    flush() {
      if (buf.length > 0) {
        cb(buf);
        buf = '';
      }
    },
  };
}
