// Per-template reachability probe specs for `templates-deploy-all.spec.js`.
// After the wizard deploys the template, we hit the SimpleDeploy proxy with
// this path + Host header (from `hostVar`) and assert the service is
// actually serving HTTP.
//
// Recorded by running each template through `e2e/tools/probe-template.sh`
// (raw `docker compose up`) and observing the default out-of-box behavior.
//
// Shape:
//   { probes: [{ hostVar, path, statusMin, statusMax, bodyIncludes?, timeoutMs? }] }
//   { probe: null, reason: '...' }  =>  skip HTTP probe entirely; template is
//     broken-by-design without external config (BYO-code scaffolds, auth
//     config files, OAuth credentials, etc.). Wizard-deploy is still
//     asserted via the status pill.
//
// `hostVar` is the key of the template variable that holds the domain to
// route by. Defaults to 'domain' if omitted. A probe without bodyIncludes
// also needs a non-empty body (or a Location on a redirect), since Caddy
// answers hosts without a route with an empty 200.

export const templateProbes = {
  'nginx-static': {
    // Docker populates the new, empty web-content volume from the image on
    // first mount (also when compose created it beforehand), so nginx serves
    // its welcome page.
    probes: [
      { path: '/', statusMin: 200, statusMax: 200, bodyIncludes: 'Welcome to nginx' },
    ],
  },

  // Bring-your-own-code scaffolds: app volumes ship empty by design; nothing
  // serves HTTP until the user drops in source. Still asserted to deploy.
  'node-api-postgres': { probe: null, reason: 'BYO scaffold: app-src volume empty until user drops in code' },
  'go-rest-api':       { probe: null, reason: 'BYO scaffold: app-bin volume empty until user drops in binary' },
  'redis-worker':      { probe: null, reason: 'BYO scaffold: app-src volume empty until user drops in code' },

  'gitea-postgres': {
    probes: [
      { path: '/', statusMin: 200, statusMax: 200, bodyIncludes: 'Installation' },
    ],
  },
  'code-server': {
    probes: [
      { path: '/', statusMin: 302, statusMax: 302 },
    ],
  },
  'uptime-kuma': {
    probes: [
      { path: '/', statusMin: 200, statusMax: 399 },
    ],
  },
  'mailpit': {
    // smtp_domain is SMTP (port 1025), not HTTP — do not probe it.
    probes: [
      { hostVar: 'domain', path: '/', statusMin: 200, statusMax: 200, bodyIncludes: 'Mailpit' },
    ],
  },
  'minio': {
    probes: [
      // pgsty/silo (MinIO fork) serves the console as "SILO Console".
      { hostVar: 'console_domain', path: '/', statusMin: 200, statusMax: 200, bodyIncludes: 'SILO Console' },
    ],
  },
  'meilisearch': {
    probes: [
      { path: '/', statusMin: 200, statusMax: 200, bodyIncludes: 'Meilisearch is running' },
    ],
  },
  'adminer': {
    probes: [
      { path: '/', statusMin: 200, statusMax: 200, bodyIncludes: 'Adminer' },
    ],
  },
  'pgadmin': {
    // pgAdmin returns 302 to /login when unauthed.
    probes: [
      { path: '/', statusMin: 200, statusMax: 302, timeoutMs: 120_000 },
    ],
  },
  'n8n-postgres': {
    // n8n redirects / to /setup on first boot.
    probes: [
      { path: '/', statusMin: 200, statusMax: 399, timeoutMs: 180_000 },
    ],
  },
  'vaultwarden': {
    probes: [
      { path: '/', statusMin: 200, statusMax: 200, bodyIncludes: 'Vaultwarden' },
    ],
  },
  'umami-postgres': {
    // Umami redirects / to /login with an empty body on first load.
    probes: [
      { path: '/', statusMin: 200, statusMax: 399, timeoutMs: 180_000 },
    ],
  },

  'authelia':      { probe: null, reason: 'Advanced: requires configuration.yml + users_database.yml in authelia-config volume before boot' },

  'docker-registry': {
    probes: [
      { hostVar: 'ui_domain', path: '/', statusMin: 200, statusMax: 200, bodyIncludes: 'Docker Registry UI' },
    ],
  },
  'poste-io': {
    probes: [
      { path: '/', statusMin: 200, statusMax: 399, timeoutMs: 120_000 },
    ],
  },


  'webhook-tester': {
    probes: [
      { path: '/', statusMin: 200, statusMax: 200, bodyIncludes: 'WebHook Tester' },
    ],
  },
};
