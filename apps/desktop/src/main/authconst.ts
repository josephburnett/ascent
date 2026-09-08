// The auth-cookie facts internal/server/auth.go owns, copied for the one place
// Electron pre-sets the cookie. authconst.test.ts pins the copies.
export const AUTH_COOKIE_NAME = 'gridwell_auth';
export const AUTH_COOKIE_MAX_AGE_S = 400 * 24 * 60 * 60;
