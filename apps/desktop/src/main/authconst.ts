// The auth-cookie facts the server owns in internal/server/auth.go, copied
// for the one place Electron pre-sets the cookie (index.ts).
// authconst.test.ts pins the copies to the Go source.
export const AUTH_COOKIE_NAME = 'gridwell_auth';
export const AUTH_COOKIE_MAX_AGE_S = 400 * 24 * 60 * 60;
