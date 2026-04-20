// App-level deploy templates for SimpleDeploy.
// Each template is a curated multi-service compose stack with sensible
// defaults, resource limits, healthchecks, TLS endpoints, backups, and
// alerts. Strings in the `compose` tree may contain `{{var}}` tokens that
// are substituted at render time from the user-supplied variable values.

// --------------------------- helpers ---------------------------------

const CHARSETS = {
  base58: '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz',
  hex: '0123456789abcdef',
  alphanum: 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789',
};

// Cryptographically random secret; rejection-samples to avoid modulo bias.
// Throws if crypto.getRandomValues is unavailable (never falls back to Math.random).
export function generateSecret(length = 24, charset = 'base58') {
  const alphabet = CHARSETS[charset];
  if (!alphabet) throw new Error(`Unknown charset: ${charset}`);
  const g =
    (typeof globalThis !== 'undefined' && globalThis.crypto && globalThis.crypto.getRandomValues)
      ? globalThis.crypto
      : null;
  if (!g) throw new Error('Secure random generator unavailable');

  const n = alphabet.length;
  if (n > 256) throw new Error('Charset too large for byte sampling');
  // Largest multiple of n that fits in an unsigned byte; reject above to
  // eliminate modulo bias.
  const max = Math.floor(256 / n) * n;

  let out = '';
  const buf = new Uint8Array(Math.max(length * 2, 16));
  while (out.length < length) {
    g.getRandomValues(buf);
    for (let i = 0; i < buf.length && out.length < length; i++) {
      const b = buf[i];
      if (b < max) out += alphabet[b % n];
    }
  }
  return out;
}

// Walk object/array tree, replacing {{key}} tokens in strings. Unknown tokens
// are left intact. Non-string leaves are passed through unchanged.
export function applyVars(node, vars) {
  if (node == null) return node;
  if (typeof node === 'string') {
    return node.replace(/\{\{\s*([a-zA-Z0-9_]+)\s*\}\}/g, (match, key) => {
      if (Object.prototype.hasOwnProperty.call(vars, key) && vars[key] != null) {
        return String(vars[key]);
      }
      return match;
    });
  }
  if (Array.isArray(node)) return node.map((item) => applyVars(item, vars));
  if (typeof node === 'object') {
    const out = {};
    for (const [k, v] of Object.entries(node)) {
      out[k] = applyVars(v, vars);
    }
    return out;
  }
  return node;
}

// --------------------------- access modes -----------------------------

export const ACCESS_MODES = ['quick-test', 'custom', 'custom-local', 'port-only'];
export const DEFAULT_ACCESS_MODE = 'quick-test';

// Count endpoint labels across all services.
export function countEndpoints(compose) {
  if (!compose?.services) return 0;
  let n = 0;
  for (const svc of Object.values(compose.services)) {
    for (const k of Object.keys(svc?.labels || {})) {
      if (/^simpledeploy\.endpoints\.\d+\.domain$/.test(k)) n++;
    }
  }
  return n;
}

// Validates hostname or IPv4 dotted quad.
export function isValidHost(s) {
  if (!s || typeof s !== 'string') return false;
  if (s.length < 1 || s.length > 253) return false;
  if (s.startsWith('.') || s.endsWith('.') || s.startsWith('-') || s.endsWith('-')) return false;
  // IPv4
  const ipv4 = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;
  const m = s.match(ipv4);
  if (m) {
    return m.slice(1).every((p) => {
      const n = Number(p);
      return n >= 0 && n <= 255 && String(n) === p;
    });
  }
  // Hostname
  return /^[a-zA-Z0-9.-]+$/.test(s);
}

// sslip.io resolves names like <anything>.<ip>.sslip.io to <ip>. Hostnames
// (e.g. "localhost") are not supported reliably, so Quick test requires IPv4.
export function isValidIPv4(s) {
  if (!s || typeof s !== 'string') return false;
  const m = s.match(/^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/);
  if (!m) return false;
  return m.slice(1).every((p) => {
    const n = Number(p);
    return n >= 0 && n <= 255 && String(n) === p;
  });
}

export function computeQuickTestDomain(slug, host) {
  return `${slug}.${host}.sslip.io`;
}

// Transform compose according to access mode.
// opts = { host, slug } (only used by quick-test indirectly; port-only uses labels).
export function applyAccessMode(compose, mode, _opts) {
  if (mode === 'custom' || !mode) return compose;
  const next = JSON.parse(JSON.stringify(compose));
  if (mode === 'quick-test' || mode === 'custom-local') {
    for (const svc of Object.values(next.services || {})) {
      if (!svc.labels) continue;
      for (const k of Object.keys(svc.labels)) {
        if (/^simpledeploy\.endpoints\.\d+\.tls$/.test(k)) {
          svc.labels[k] = 'local';
        }
      }
    }
    return next;
  }
  if (mode === 'port-only') {
    for (const svc of Object.values(next.services || {})) {
      if (!svc.labels) continue;
      // Find the port from endpoint 0 before stripping.
      let port = null;
      for (const [k, v] of Object.entries(svc.labels)) {
        if (k === 'simpledeploy.endpoints.0.port') port = v;
      }
      // Strip endpoint labels.
      const hadEndpoint = Object.keys(svc.labels).some((k) => /^simpledeploy\.endpoints\.\d+\./.test(k));
      for (const k of Object.keys(svc.labels)) {
        if (/^simpledeploy\.endpoints\.\d+\./.test(k)) delete svc.labels[k];
      }
      if (hadEndpoint && port) {
        if (!Array.isArray(svc.ports)) svc.ports = [];
        svc.ports.push(`0:${port}`);
      }
    }
    return next;
  }
  return next;
}

// Returns {} if valid; else {key: 'error msg'} per failing field.
export function validateVars(variables, values) {
  const errors = {};
  for (const v of variables) {
    const raw = values[v.key];
    const has = raw != null && String(raw).length > 0;
    if (v.required && !has) {
      errors[v.key] = `${v.label || v.key} is required`;
      continue;
    }
    if (!has) continue;
    const str = String(raw);
    if (v.type === 'email') {
      if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(str)) {
        errors[v.key] = 'Enter a valid email address';
        continue;
      }
    }
    if (v.type === 'domain') {
      if (!/^(?=.{1,253}$)(?!-)[A-Za-z0-9-]{1,63}(?<!-)(\.(?!-)[A-Za-z0-9-]{1,63}(?<!-))*$/.test(str)) {
        errors[v.key] = 'Enter a valid domain';
        continue;
      }
    }
    if (v.type === 'number') {
      if (!/^-?\d+(\.\d+)?$/.test(str)) {
        errors[v.key] = 'Enter a number';
        continue;
      }
    }
    if (v.type === 'enum') {
      const allowed = (v.options || []).map((o) => o.value);
      if (!allowed.includes(str)) {
        errors[v.key] = 'Choose a valid option';
        continue;
      }
    }
    if (v.pattern) {
      try {
        const re = new RegExp(v.pattern);
        if (!re.test(str)) {
          errors[v.key] = v.patternMessage || `${v.label || v.key} has invalid format`;
          continue;
        }
      } catch (_e) {
        // ignore malformed patterns
      }
    }
  }
  return errors;
}

// Returns base if unused, otherwise base-2, base-3, ... based on existing names array.
export function suggestName(base, existingNames) {
  const set = new Set((existingNames || []).map((n) => String(n)));
  if (!set.has(base)) return base;
  let i = 2;
  while (set.has(`${base}-${i}`)) i++;
  return `${base}-${i}`;
}

// --------------------------- categories ------------------------------

export const categories = [
  { id: 'web',           label: 'Web' },
  { id: 'dev-tools',     label: 'Dev Tools' },
  { id: 'databases',     label: 'Databases' },
  { id: 'storage',       label: 'Storage' },
  { id: 'productivity',  label: 'Productivity' },
  { id: 'observability', label: 'Observability' },
  { id: 'auth',          label: 'Auth' },
  { id: 'mail',          label: 'Mail' },
  { id: 'ci',            label: 'CI/CD' },
];

// --------------------------- shared fragments -------------------------

const HC = {
  pg: {
    test: ['CMD-SHELL', 'pg_isready -U postgres'],
    interval: '10s',
    timeout: '5s',
    retries: 5,
    start_period: '30s',
  },
  redis: {
    test: ['CMD-SHELL', 'redis-cli ping'],
    interval: '10s',
    timeout: '5s',
    retries: 5,
    start_period: '10s',
  },
};

function limits(cpus, memory) {
  return { resources: { limits: { cpus, memory } } };
}

const domainVar = {
  key: 'domain',
  label: 'Domain',
  type: 'domain',
  required: true,
  placeholder: 'app.example.com',
  help: 'Public domain name pointing at this server.',
};

// --------------------------- templates --------------------------------

export const appTemplates = [
  // 1. nginx-static
  {
    id: 'nginx-static',
    name: 'Nginx Static Site',
    icon: '🌐',
    category: 'web',
    description: 'Serve static HTML/CSS/JS with Nginx.',
    tags: ['static', 'website', 'html', 'nginx'],
    nameSuggestion: 'static-site',
    advanced: false,
    variables: [domainVar],
    compose: {
      services: {
        web: {
          image: 'nginx:1.27-alpine',
          restart: 'unless-stopped',
          volumes: ['web-content:/usr/share/nginx/html:ro'],
          deploy: limits('0.25', '128M'),
          healthcheck: {
            test: ['CMD', 'wget', '-qO-', 'http://localhost/'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '10s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '80',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.alert.cpu': '80',
            'simpledeploy.alert.memory': '85',
            'simpledeploy.backup.strategy': 'volume',
            'simpledeploy.backup.schedule': '0 3 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '7',
          },
        },
      },
      volumes: { 'web-content': {} },
    },
    notes: [
      'Upload files into the `web-content` volume after first deploy.',
    ],
  },

  // 2. node-api-postgres
  {
    id: 'node-api-postgres',
    name: 'Node API + Postgres',
    icon: '🟢',
    category: 'dev-tools',
    description: 'Node.js API with a Postgres database.',
    tags: ['node', 'api', 'postgres', 'javascript'],
    nameSuggestion: 'node-api',
    advanced: false,
    variables: [
      domainVar,
      {
        key: 'db_password',
        label: 'Postgres password',
        type: 'secret',
        required: true,
        hidden: true,
        generate: { length: 32, charset: 'base58' },
      },
    ],
    compose: {
      services: {
        api: {
          image: 'node:20-alpine',
          restart: 'unless-stopped',
          working_dir: '/app',
          command: 'node server.js',
          volumes: ['app-src:/app'],
          environment: {
            NODE_ENV: 'production',
            PORT: '3000',
            DATABASE_URL: 'postgres://app:{{db_password}}@db:5432/app',
          },
          depends_on: { db: { condition: 'service_healthy' } },
          deploy: limits('0.5', '256M'),
          healthcheck: {
            test: ['CMD', 'wget', '-qO-', 'http://localhost:3000/health'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '30s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '3000',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.alert.cpu': '80',
            'simpledeploy.alert.memory': '85',
          },
        },
        db: {
          image: 'postgres:16-alpine',
          restart: 'unless-stopped',
          environment: {
            POSTGRES_USER: 'app',
            POSTGRES_PASSWORD: '{{db_password}}',
            POSTGRES_DB: 'app',
          },
          volumes: ['pgdata:/var/lib/postgresql/data'],
          deploy: limits('0.5', '512M'),
          healthcheck: HC.pg,
          labels: {
            'simpledeploy.backup.strategy': 'postgres',
            'simpledeploy.backup.schedule': '0 2 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '7',
            'simpledeploy.alert.cpu': '80',
            'simpledeploy.alert.memory': '85',
          },
        },
      },
      volumes: { 'app-src': {}, pgdata: {} },
    },
    notes: [
      'Drop your API source into the `app-src` volume (or replace with a build step).',
      'The API should expose `/health` for the healthcheck to pass.',
    ],
  },

  // 3. go-rest-api
  {
    id: 'go-rest-api',
    name: 'Go REST API',
    icon: '🐹',
    category: 'dev-tools',
    description: 'Go HTTP service behind the proxy with rate limiting.',
    tags: ['go', 'golang', 'api', 'rest'],
    nameSuggestion: 'go-api',
    advanced: false,
    variables: [domainVar],
    compose: {
      services: {
        api: {
          image: 'golang:1.23-alpine',
          restart: 'unless-stopped',
          working_dir: '/app',
          command: './server',
          volumes: ['app-bin:/app'],
          environment: {
            PORT: '8080',
          },
          deploy: limits('0.5', '128M'),
          healthcheck: {
            test: ['CMD', 'wget', '-qO-', 'http://localhost:8080/health'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '20s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '8080',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.ratelimit.requests': '60',
            'simpledeploy.ratelimit.window': '1m',
            'simpledeploy.ratelimit.by': 'ip',
            'simpledeploy.ratelimit.burst': '20',
            'simpledeploy.alert.cpu': '80',
            'simpledeploy.alert.memory': '85',
          },
        },
      },
      volumes: { 'app-bin': {} },
    },
    notes: [
      'Build your Go binary as `server` and place it in the `app-bin` volume, or switch this to a custom image build.',
    ],
  },

  // 4. redis-worker
  {
    id: 'redis-worker',
    name: 'API + Worker + Redis',
    icon: '🔴',
    category: 'dev-tools',
    description: 'Node API with a background worker and Redis queue.',
    tags: ['node', 'redis', 'worker', 'queue'],
    nameSuggestion: 'worker-app',
    advanced: false,
    variables: [domainVar],
    compose: {
      services: {
        api: {
          image: 'node:20-alpine',
          restart: 'unless-stopped',
          working_dir: '/app',
          command: 'node server.js',
          volumes: ['app-src:/app'],
          environment: {
            NODE_ENV: 'production',
            PORT: '3000',
            REDIS_URL: 'redis://redis:6379',
          },
          depends_on: { redis: { condition: 'service_healthy' } },
          deploy: limits('0.5', '256M'),
          healthcheck: {
            test: ['CMD', 'wget', '-qO-', 'http://localhost:3000/health'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '30s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '3000',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.alert.cpu': '80',
            'simpledeploy.alert.memory': '85',
          },
        },
        worker: {
          image: 'node:20-alpine',
          restart: 'unless-stopped',
          working_dir: '/app',
          command: 'node worker.js',
          volumes: ['app-src:/app'],
          environment: {
            NODE_ENV: 'production',
            REDIS_URL: 'redis://redis:6379',
          },
          depends_on: { redis: { condition: 'service_healthy' } },
          deploy: limits('0.5', '256M'),
          labels: {
            'simpledeploy.alert.cpu': '80',
            'simpledeploy.alert.memory': '85',
          },
        },
        redis: {
          image: 'redis:7-alpine',
          restart: 'unless-stopped',
          command: 'redis-server --appendonly yes',
          volumes: ['redisdata:/data'],
          deploy: limits('0.25', '128M'),
          healthcheck: HC.redis,
          labels: {
            'simpledeploy.backup.strategy': 'volume',
            'simpledeploy.backup.schedule': '0 2 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '7',
          },
        },
      },
      volumes: { 'app-src': {}, redisdata: {} },
    },
    notes: [
      'Drop your Node source with `server.js` and `worker.js` into the `app-src` volume.',
    ],
  },

  // 5. gitea-postgres
  {
    id: 'gitea-postgres',
    name: 'Gitea (Git hosting)',
    icon: '🍵',
    category: 'dev-tools',
    description: 'Self-hosted Git service backed by Postgres.',
    tags: ['git', 'gitea', 'source control', 'postgres'],
    nameSuggestion: 'gitea',
    advanced: false,
    variables: [
      domainVar,
      {
        key: 'admin_user',
        label: 'Admin username',
        type: 'text',
        required: false,
        default: 'gitea-admin',
        pattern: '^[a-z0-9_-]{3,32}$',
        help: '3-32 chars, lowercase letters, digits, `_` or `-`.',
      },
      { key: 'admin_email', label: 'Admin email', type: 'email', required: false },
      {
        key: 'admin_password',
        label: 'Admin password',
        type: 'secret',
        required: false,
        generate: { length: 24, charset: 'base58' },
      },
      {
        key: 'db_password',
        label: 'Database password',
        type: 'secret',
        required: true,
        hidden: true,
        generate: { length: 32, charset: 'base58' },
      },
    ],
    compose: {
      services: {
        gitea: {
          image: 'gitea/gitea:1.26',
          restart: 'unless-stopped',
          environment: {
            USER_UID: '1000',
            USER_GID: '1000',
            GITEA__database__DB_TYPE: 'postgres',
            GITEA__database__HOST: 'db:5432',
            GITEA__database__NAME: 'gitea',
            GITEA__database__USER: 'gitea',
            GITEA__database__PASSWD: '{{db_password}}',
            GITEA__server__ROOT_URL: 'https://{{domain}}/',
            GITEA__server__DOMAIN: '{{domain}}',
          },
          volumes: ['gitea-data:/data'],
          depends_on: { db: { condition: 'service_healthy' } },
          deploy: limits('1.0', '512M'),
          healthcheck: {
            test: ['CMD', 'wget', '-qO-', 'http://localhost:3000/'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '60s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '3000',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.backup.strategy': 'volume',
            'simpledeploy.backup.schedule': '0 3 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '14',
            'simpledeploy.alert.cpu': '85',
            'simpledeploy.alert.memory': '85',
          },
        },
        db: {
          image: 'postgres:16-alpine',
          restart: 'unless-stopped',
          environment: {
            POSTGRES_USER: 'gitea',
            POSTGRES_PASSWORD: '{{db_password}}',
            POSTGRES_DB: 'gitea',
          },
          volumes: ['pgdata:/var/lib/postgresql/data'],
          deploy: limits('0.5', '512M'),
          healthcheck: HC.pg,
          labels: {
            'simpledeploy.backup.strategy': 'postgres',
            'simpledeploy.backup.schedule': '0 2 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '14',
          },
        },
      },
      volumes: { 'gitea-data': {}, pgdata: {} },
    },
    notes: [
      'Visit the domain after deploy and complete the install form using the admin username/email/password you chose.',
      'Use these credentials when completing the Gitea web installer on first visit.',
    ],
  },

  // 6. code-server
  {
    id: 'code-server',
    name: 'VS Code (code-server)',
    icon: '💻',
    category: 'dev-tools',
    description: 'Browser-based VS Code IDE.',
    tags: ['vscode', 'editor', 'code-server', 'ide'],
    nameSuggestion: 'code',
    advanced: false,
    variables: [
      domainVar,
      {
        key: 'password',
        label: 'Login password',
        type: 'secret',
        required: true,
        generate: { length: 24, charset: 'base58' },
      },
      {
        key: 'access_cidr',
        label: 'Allowed IP range (CIDR)',
        type: 'text',
        required: true,
        default: '0.0.0.0/0',
        help: 'Restrict access to a network range. Use `10.0.0.0/8` for internal-only.',
      },
    ],
    compose: {
      services: {
        code: {
          image: 'codercom/code-server:4.116.0',
          restart: 'unless-stopped',
          user: '1000:1000',
          environment: {
            PASSWORD: '{{password}}',
            DOCKER_USER: 'coder',
          },
          // Mount only the project dir; .config lives inside the container to avoid
          // root-owned named-volume init blocking code-server's first-run mkdir.
          volumes: ['code-project:/home/coder/project'],
          deploy: limits('1.0', '1024M'),
          healthcheck: {
            test: ['CMD', 'wget', '-qO-', 'http://localhost:8080/'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '30s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '8080',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.access.allow': '{{access_cidr}}',
            'simpledeploy.backup.strategy': 'volume',
            'simpledeploy.backup.schedule': '0 3 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '7',
            'simpledeploy.alert.cpu': '90',
            'simpledeploy.alert.memory': '90',
          },
        },
      },
      volumes: { 'code-project': {} },
    },
    notes: [
      'The default `0.0.0.0/0` CIDR allows anyone. Narrow it to your IP range for sensitive projects.',
    ],
  },

  // 7. uptime-kuma
  {
    id: 'uptime-kuma',
    name: 'Uptime Kuma',
    icon: '📈',
    category: 'observability',
    description: 'Self-hosted uptime monitoring with nice dashboards.',
    tags: ['monitoring', 'uptime', 'status'],
    nameSuggestion: 'uptime',
    advanced: false,
    variables: [domainVar],
    compose: {
      services: {
        kuma: {
          image: 'louislam/uptime-kuma:1',
          restart: 'unless-stopped',
          volumes: ['kuma-data:/app/data'],
          deploy: limits('0.25', '128M'),
          healthcheck: {
            test: ['CMD', 'node', '/app/extra/healthcheck.js'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '60s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '3001',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.backup.strategy': 'volume',
            'simpledeploy.backup.schedule': '0 3 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '14',
            'simpledeploy.alert.cpu': '80',
            'simpledeploy.alert.memory': '85',
          },
        },
      },
      volumes: { 'kuma-data': {} },
    },
    notes: [
      'On first load, create the admin account in the web UI.',
    ],
  },

  // 8. mailpit
  {
    id: 'mailpit',
    name: 'Mailpit',
    icon: '📮',
    category: 'dev-tools',
    description: 'Dev-only SMTP catcher with a friendly web UI.',
    tags: ['smtp', 'email', 'dev', 'testing'],
    nameSuggestion: 'mailpit',
    advanced: false,
    variables: [
      domainVar,
      {
        key: 'smtp_domain',
        label: 'SMTP hostname',
        type: 'domain',
        required: true,
        placeholder: 'smtp.example.com',
        help: 'Public SMTP hostname apps use to connect on port 1025. Must be a valid domain pointing at this server.',
      },
    ],
    compose: {
      services: {
        mailpit: {
          image: 'axllent/mailpit:v1.29',
          restart: 'unless-stopped',
          environment: {
            MP_SMTP_AUTH_ACCEPT_ANY: '1',
            MP_SMTP_AUTH_ALLOW_INSECURE: '1',
          },
          deploy: limits('0.25', '64M'),
          healthcheck: {
            test: ['CMD', '/mailpit', 'readyz'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '10s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '8025',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.endpoints.1.domain': '{{smtp_domain}}',
            'simpledeploy.endpoints.1.port': '1025',
            'simpledeploy.endpoints.1.tls': 'off',
            'simpledeploy.alert.cpu': '80',
            'simpledeploy.alert.memory': '85',
          },
        },
      },
    },
    notes: [
      'Mailpit is for testing only. Do not point production apps at it.',
    ],
  },

  // 9. minio
  {
    id: 'minio',
    name: 'MinIO (S3 storage)',
    icon: '🪣',
    category: 'storage',
    description: 'S3-compatible object storage with a web console.',
    tags: ['s3', 'storage', 'minio', 'objects'],
    nameSuggestion: 'minio',
    advanced: false,
    variables: [
      { ...domainVar, label: 'API domain', help: 'Domain for the S3 API (e.g. s3.example.com).' },
      {
        key: 'console_domain',
        label: 'Console domain',
        type: 'domain',
        required: true,
        placeholder: 'minio.example.com',
        help: 'Domain for the MinIO web console.',
      },
      { key: 'root_user', label: 'Root user', type: 'text', required: true, default: 'admin' },
      {
        key: 'root_password',
        label: 'Root password',
        type: 'secret',
        required: true,
        generate: { length: 32, charset: 'base58' },
      },
    ],
    compose: {
      services: {
        minio: {
          image: 'minio/minio:RELEASE.2025-09-07T16-13-09Z',
          restart: 'unless-stopped',
          command: 'server /data --console-address :9001',
          environment: {
            MINIO_ROOT_USER: '{{root_user}}',
            MINIO_ROOT_PASSWORD: '{{root_password}}',
            MINIO_BROWSER_REDIRECT_URL: 'https://{{console_domain}}',
            MINIO_SERVER_URL: 'https://{{domain}}',
          },
          volumes: ['minio-data:/data'],
          deploy: limits('1.0', '512M'),
          healthcheck: {
            test: ['CMD-SHELL', 'curl -fsS http://localhost:9000/minio/health/live || exit 1'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '20s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '9000',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.endpoints.1.domain': '{{console_domain}}',
            'simpledeploy.endpoints.1.port': '9001',
            'simpledeploy.endpoints.1.tls': 'letsencrypt',
            'simpledeploy.backup.strategy': 'volume',
            'simpledeploy.backup.schedule': '0 2 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '7',
            'simpledeploy.alert.cpu': '85',
            'simpledeploy.alert.memory': '85',
          },
        },
      },
      volumes: { 'minio-data': {} },
    },
    notes: [
      'Create access keys in the console after first login.',
    ],
  },

  // 10. meilisearch
  {
    id: 'meilisearch',
    name: 'Meilisearch',
    icon: '🔎',
    category: 'storage',
    description: 'Lightning-fast full-text search engine.',
    tags: ['search', 'meilisearch', 'index'],
    nameSuggestion: 'meilisearch',
    advanced: false,
    variables: [
      domainVar,
      {
        key: 'master_key',
        label: 'Master key',
        type: 'secret',
        required: true,
        generate: { length: 32, charset: 'base58' },
      },
    ],
    compose: {
      services: {
        meili: {
          image: 'getmeili/meilisearch:v1.42',
          restart: 'unless-stopped',
          environment: {
            MEILI_MASTER_KEY: '{{master_key}}',
            MEILI_ENV: 'production',
          },
          volumes: ['meili-data:/meili_data'],
          deploy: limits('0.5', '256M'),
          healthcheck: {
            test: ['CMD', 'wget', '-qO-', 'http://localhost:7700/health'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '20s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '7700',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.backup.strategy': 'volume',
            'simpledeploy.backup.schedule': '0 2 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '7',
            'simpledeploy.alert.cpu': '85',
            'simpledeploy.alert.memory': '85',
          },
        },
      },
      volumes: { 'meili-data': {} },
    },
  },

  // 11. adminer
  {
    id: 'adminer',
    name: 'Adminer',
    icon: '🗄️',
    category: 'databases',
    description: 'Lightweight web UI for managing databases.',
    tags: ['database', 'admin', 'adminer', 'sql'],
    nameSuggestion: 'adminer',
    advanced: false,
    variables: [
      domainVar,
      {
        key: 'access_cidr',
        label: 'Allowed IP range (CIDR)',
        type: 'text',
        required: true,
        default: '0.0.0.0/0',
        help: 'Restrict who can reach Adminer. Use `10.0.0.0/8` for internal-only.',
      },
    ],
    compose: {
      services: {
        adminer: {
          image: 'adminer:5.4',
          restart: 'unless-stopped',
          deploy: limits('0.25', '64M'),
          healthcheck: {
            test: ['CMD', 'wget', '-qO-', 'http://localhost:8080/'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '10s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '8080',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.access.allow': '{{access_cidr}}',
            'simpledeploy.alert.cpu': '80',
            'simpledeploy.alert.memory': '85',
          },
        },
      },
    },
    notes: [
      'Adminer is a powerful tool. Lock it down with `access_cidr` or a VPN.',
    ],
  },

  // 12. pgadmin
  {
    id: 'pgadmin',
    name: 'pgAdmin',
    icon: '🐘',
    category: 'databases',
    description: 'Full-featured web UI for managing Postgres.',
    tags: ['postgres', 'pgadmin', 'database', 'admin'],
    nameSuggestion: 'pgadmin',
    advanced: false,
    variables: [
      domainVar,
      { key: 'admin_email', label: 'Admin email', type: 'email', required: true },
      {
        key: 'admin_password',
        label: 'Admin password',
        type: 'secret',
        required: true,
        generate: { length: 24, charset: 'base58' },
      },
    ],
    compose: {
      services: {
        pgadmin: {
          image: 'dpage/pgadmin4:9',
          restart: 'unless-stopped',
          environment: {
            PGADMIN_DEFAULT_EMAIL: '{{admin_email}}',
            PGADMIN_DEFAULT_PASSWORD: '{{admin_password}}',
            PGADMIN_LISTEN_PORT: '80',
          },
          volumes: ['pgadmin-data:/var/lib/pgadmin'],
          deploy: limits('0.5', '256M'),
          healthcheck: {
            test: ['CMD', 'wget', '-qO-', 'http://localhost:80/misc/ping'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '30s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '80',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.backup.strategy': 'volume',
            'simpledeploy.backup.schedule': '0 3 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '7',
            'simpledeploy.alert.cpu': '80',
            'simpledeploy.alert.memory': '85',
          },
        },
      },
      volumes: { 'pgadmin-data': {} },
    },
  },

  // 13. n8n-postgres
  {
    id: 'n8n-postgres',
    name: 'n8n Workflow Automation',
    icon: '🤖',
    category: 'productivity',
    description: 'Visual workflow automation, backed by Postgres.',
    tags: ['n8n', 'automation', 'workflow', 'postgres'],
    nameSuggestion: 'n8n',
    advanced: false,
    variables: [
      domainVar,
      {
        key: 'admin_user',
        label: 'Admin username',
        type: 'text',
        required: false,
        default: 'admin',
        pattern: '^[a-zA-Z0-9_-]{3,32}$',
      },
      {
        key: 'admin_password',
        label: 'Admin password',
        type: 'secret',
        required: false,
        generate: { length: 24, charset: 'base58' },
      },
      {
        key: 'db_password',
        label: 'Database password',
        type: 'secret',
        required: true,
        hidden: true,
        generate: { length: 32, charset: 'base58' },
      },
    ],
    compose: {
      services: {
        n8n: {
          image: 'n8nio/n8n:2.16.1',
          restart: 'unless-stopped',
          environment: {
            DB_TYPE: 'postgresdb',
            DB_POSTGRESDB_HOST: 'db',
            DB_POSTGRESDB_PORT: '5432',
            DB_POSTGRESDB_DATABASE: 'n8n',
            DB_POSTGRESDB_USER: 'n8n',
            DB_POSTGRESDB_PASSWORD: '{{db_password}}',
            N8N_HOST: '{{domain}}',
            N8N_PORT: '5678',
            N8N_PROTOCOL: 'https',
            WEBHOOK_URL: 'https://{{domain}}/',
          },
          volumes: ['n8n-data:/home/node/.n8n'],
          depends_on: { db: { condition: 'service_healthy' } },
          deploy: limits('1.0', '512M'),
          healthcheck: {
            test: ['CMD', 'wget', '-qO-', 'http://localhost:5678/healthz'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '60s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '5678',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.backup.strategy': 'volume',
            'simpledeploy.backup.schedule': '0 3 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '14',
            'simpledeploy.alert.cpu': '85',
            'simpledeploy.alert.memory': '85',
          },
        },
        db: {
          image: 'postgres:16-alpine',
          restart: 'unless-stopped',
          environment: {
            POSTGRES_USER: 'n8n',
            POSTGRES_PASSWORD: '{{db_password}}',
            POSTGRES_DB: 'n8n',
          },
          volumes: ['pgdata:/var/lib/postgresql/data'],
          deploy: limits('0.5', '512M'),
          healthcheck: HC.pg,
          labels: {
            'simpledeploy.backup.strategy': 'postgres',
            'simpledeploy.backup.schedule': '0 2 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '14',
          },
        },
      },
      volumes: { 'n8n-data': {}, pgdata: {} },
    },
    notes: [
      'After deploy, open the app and complete the n8n owner-account setup wizard with the admin credentials above.',
    ],
  },

  // 14. vaultwarden
  {
    id: 'vaultwarden',
    name: 'Vaultwarden (Bitwarden)',
    icon: '🔐',
    category: 'auth',
    description: 'Self-hosted Bitwarden-compatible password manager.',
    tags: ['password', 'vaultwarden', 'bitwarden', 'secrets'],
    nameSuggestion: 'vault',
    advanced: false,
    variables: [
      domainVar,
      {
        key: 'admin_token',
        label: 'Admin panel token',
        type: 'secret',
        required: true,
        generate: { length: 48, charset: 'base58' },
        help: 'Used to access the /admin panel. Keep this safe.',
      },
    ],
    compose: {
      services: {
        vaultwarden: {
          image: 'vaultwarden/server:1.35.7',
          restart: 'unless-stopped',
          environment: {
            DOMAIN: 'https://{{domain}}',
            ADMIN_TOKEN: '{{admin_token}}',
            SIGNUPS_ALLOWED: 'false',
            ROCKET_PORT: '80',
          },
          volumes: ['vaultwarden-data:/data'],
          deploy: limits('0.5', '256M'),
          healthcheck: {
            test: ['CMD', 'wget', '-qO-', 'http://localhost:80/alive'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '30s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '80',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.ratelimit.requests': '10',
            'simpledeploy.ratelimit.window': '1m',
            'simpledeploy.ratelimit.by': 'ip',
            'simpledeploy.ratelimit.burst': '5',
            'simpledeploy.backup.strategy': 'volume',
            'simpledeploy.backup.schedule': '0 2 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '30',
            'simpledeploy.alert.cpu': '80',
            'simpledeploy.alert.memory': '85',
          },
        },
      },
      volumes: { 'vaultwarden-data': {} },
    },
    notes: [
      'Sign-ups are disabled by default. Temporarily set SIGNUPS_ALLOWED=true to create your first account, then turn it back off.',
      'Your vault is only as safe as your backups. Keep them off-server.',
    ],
  },

  // 15. umami-postgres
  {
    id: 'umami-postgres',
    name: 'Umami Analytics',
    icon: '📊',
    category: 'observability',
    description: 'Privacy-friendly web analytics, backed by Postgres.',
    tags: ['analytics', 'umami', 'privacy', 'postgres'],
    nameSuggestion: 'umami',
    advanced: false,
    variables: [
      domainVar,
      {
        key: 'hash_salt',
        label: 'Hash salt',
        type: 'secret',
        required: true,
        hidden: true,
        generate: { length: 32, charset: 'hex' },
      },
      {
        key: 'db_password',
        label: 'Database password',
        type: 'secret',
        required: true,
        hidden: true,
        generate: { length: 32, charset: 'base58' },
      },
    ],
    compose: {
      services: {
        umami: {
          image: 'ghcr.io/umami-software/umami:postgresql-v2.20.2',
          restart: 'unless-stopped',
          environment: {
            DATABASE_URL: 'postgresql://umami:{{db_password}}@db:5432/umami',
            DATABASE_TYPE: 'postgresql',
            APP_SECRET: '{{hash_salt}}',
          },
          depends_on: { db: { condition: 'service_healthy' } },
          deploy: limits('0.5', '256M'),
          healthcheck: {
            test: ['CMD', 'wget', '-qO-', 'http://localhost:3000/api/heartbeat'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '60s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '3000',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.alert.cpu': '80',
            'simpledeploy.alert.memory': '85',
          },
        },
        db: {
          image: 'postgres:16-alpine',
          restart: 'unless-stopped',
          environment: {
            POSTGRES_USER: 'umami',
            POSTGRES_PASSWORD: '{{db_password}}',
            POSTGRES_DB: 'umami',
          },
          volumes: ['pgdata:/var/lib/postgresql/data'],
          deploy: limits('0.5', '512M'),
          healthcheck: HC.pg,
          labels: {
            'simpledeploy.backup.strategy': 'postgres',
            'simpledeploy.backup.schedule': '0 2 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '14',
          },
        },
      },
      volumes: { pgdata: {} },
    },
    notes: [
      'Default login is `admin` / `umami` -- change it immediately after first login.',
    ],
  },

  // 16. authelia
  {
    id: 'authelia',
    name: 'Authelia SSO',
    icon: '🛡️',
    category: 'auth',
    description: 'Single sign-on and 2FA portal. Requires config file.',
    tags: ['auth', 'sso', '2fa', 'authelia'],
    nameSuggestion: 'authelia',
    advanced: true,
    variables: [
      domainVar,
      { key: 'admin_email', label: 'Admin email', type: 'email', required: true },
      {
        key: 'jwt_secret',
        label: 'JWT secret',
        type: 'secret',
        required: true,
        hidden: true,
        generate: { length: 64, charset: 'base58' },
      },
      {
        key: 'session_secret',
        label: 'Session secret',
        type: 'secret',
        required: true,
        hidden: true,
        generate: { length: 64, charset: 'base58' },
      },
      {
        key: 'storage_encryption_key',
        label: 'Storage encryption key',
        type: 'secret',
        required: true,
        hidden: true,
        generate: { length: 64, charset: 'base58' },
      },
    ],
    compose: {
      services: {
        authelia: {
          image: 'authelia/authelia:4.39',
          restart: 'unless-stopped',
          environment: {
            AUTHELIA_JWT_SECRET: '{{jwt_secret}}',
            AUTHELIA_SESSION_SECRET: '{{session_secret}}',
            AUTHELIA_STORAGE_ENCRYPTION_KEY: '{{storage_encryption_key}}',
            AUTHELIA_NOTIFIER_SMTP_SENDER: '{{admin_email}}',
          },
          volumes: ['authelia-config:/config'],
          depends_on: { redis: { condition: 'service_healthy' } },
          deploy: limits('0.5', '128M'),
          healthcheck: {
            test: ['CMD', '/app/authelia', 'healthcheck'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '30s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '9091',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.backup.strategy': 'volume',
            'simpledeploy.backup.schedule': '0 2 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '14',
            'simpledeploy.alert.cpu': '80',
            'simpledeploy.alert.memory': '85',
          },
        },
        redis: {
          image: 'redis:7-alpine',
          restart: 'unless-stopped',
          volumes: ['authelia-redis:/data'],
          deploy: limits('0.25', '128M'),
          healthcheck: HC.redis,
        },
      },
      volumes: { 'authelia-config': {}, 'authelia-redis': {} },
    },
    notes: [
      'Authelia needs a `configuration.yml` in the `authelia-config` volume before it will start. Copy one from the official examples.',
      'Define users in `users_database.yml` or configure an LDAP backend.',
      'This is an advanced template. Expect to do post-deploy config work.',
    ],
  },

  // 17. docker-registry
  {
    id: 'docker-registry',
    name: 'Docker Registry + UI',
    icon: '🐳',
    category: 'storage',
    description: 'Private Docker image registry with a browser UI.',
    tags: ['docker', 'registry', 'images'],
    nameSuggestion: 'registry',
    advanced: false,
    variables: [
      { ...domainVar, label: 'Registry API domain', placeholder: 'registry.example.com' },
      {
        key: 'ui_domain',
        label: 'Registry UI domain',
        type: 'domain',
        required: true,
        placeholder: 'registry-ui.example.com',
      },
    ],
    compose: {
      services: {
        registry: {
          image: 'registry:2.8.3',
          restart: 'unless-stopped',
          environment: {
            REGISTRY_STORAGE_DELETE_ENABLED: 'true',
          },
          volumes: ['registry-data:/var/lib/registry'],
          deploy: limits('0.5', '256M'),
          healthcheck: {
            test: ['CMD', '/bin/registry', '--version'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '15s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '5000',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.backup.strategy': 'volume',
            'simpledeploy.backup.schedule': '0 2 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '7',
            'simpledeploy.alert.cpu': '80',
            'simpledeploy.alert.memory': '85',
          },
        },
        ui: {
          image: 'joxit/docker-registry-ui:2.5',
          restart: 'unless-stopped',
          environment: {
            REGISTRY_TITLE: 'Docker Registry',
            NGINX_PROXY_PASS_URL: 'http://registry:5000',
            DELETE_IMAGES: 'true',
            SINGLE_REGISTRY: 'true',
          },
          depends_on: { registry: { condition: 'service_healthy' } },
          deploy: limits('0.25', '128M'),
          healthcheck: {
            test: ['CMD-SHELL', 'wget -qO- http://localhost/ >/dev/null || exit 1'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '15s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{ui_domain}}',
            'simpledeploy.endpoints.0.port': '80',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
          },
        },
      },
      volumes: { 'registry-data': {} },
    },
    notes: [
      'This registry is open by default. Add basic auth (REGISTRY_AUTH=htpasswd) if internet-accessible.',
      'Log in from clients with: `docker login {{domain}}`.',
    ],
  },

  // 18. poste-io
  {
    id: 'poste-io',
    name: 'Poste.io Mail Server',
    icon: '✉️',
    category: 'mail',
    description: 'All-in-one mail server (SMTP/IMAP/POP3 + webmail).',
    tags: ['email', 'mail', 'smtp', 'imap', 'poste'],
    nameSuggestion: 'mail',
    advanced: true,
    variables: [
      domainVar,
      { key: 'admin_email', label: 'Admin email', type: 'email', required: false },
      {
        key: 'admin_password',
        label: 'Admin password',
        type: 'secret',
        required: false,
        generate: { length: 24, charset: 'base58' },
      },
    ],
    compose: {
      services: {
        poste: {
          image: 'analogic/poste.io:2',
          restart: 'unless-stopped',
          hostname: '{{domain}}',
          environment: {
            HTTPS: 'OFF',
            DISABLE_CLAMAV: 'TRUE',
            VIRTUAL_HOST: '{{domain}}',
          },
          ports: [
            '25:25',
            '110:110',
            '143:143',
            '465:465',
            '587:587',
            '993:993',
            '995:995',
          ],
          volumes: ['poste-data:/data'],
          deploy: limits('1.0', '1024M'),
          healthcheck: {
            test: ['CMD', 'wget', '-qO-', 'http://localhost:80/'],
            interval: '60s',
            timeout: '10s',
            retries: 3,
            start_period: '120s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '80',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.backup.strategy': 'volume',
            'simpledeploy.backup.schedule': '0 2 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '30',
            'simpledeploy.alert.cpu': '85',
            'simpledeploy.alert.memory': '90',
          },
        },
      },
      volumes: { 'poste-data': {} },
    },
    notes: [
      'Running mail is hard. You must control DNS for {{domain}} and configure MX, SPF, DKIM, and DMARC records.',
      'Port 25 is blocked by many cloud providers. Verify outbound SMTP works before depending on this.',
      'Reverse DNS (PTR) on the server IP must match {{domain}}, or most mail will be rejected.',
      'Use these credentials when visiting the admin UI on first load to create the admin account.',
    ],
  },

  // 19. woodpecker-ci
  {
    id: 'woodpecker-ci',
    name: 'Woodpecker CI',
    icon: '🪵',
    category: 'ci',
    description: 'Simple, container-native CI server with agent.',
    tags: ['ci', 'woodpecker', 'pipelines', 'build'],
    nameSuggestion: 'woodpecker',
    advanced: false,
    variables: [
      domainVar,
      {
        key: 'admin_user',
        label: 'Admin username (forge login)',
        type: 'text',
        required: true,
        pattern: '^[a-zA-Z0-9_-]{2,64}$',
        help: 'Your username on the Git forge (e.g. GitHub/Gitea) that becomes the Woodpecker admin.',
      },
      {
        key: 'agent_secret',
        label: 'Agent secret',
        type: 'secret',
        required: true,
        hidden: true,
        generate: { length: 64, charset: 'hex' },
      },
    ],
    compose: {
      services: {
        server: {
          image: 'woodpeckerci/woodpecker-server:v3',
          restart: 'unless-stopped',
          environment: {
            WOODPECKER_OPEN: 'false',
            WOODPECKER_ADMIN: '{{admin_user}}',
            WOODPECKER_HOST: 'https://{{domain}}',
            WOODPECKER_AGENT_SECRET: '{{agent_secret}}',
          },
          volumes: ['woodpecker-server-data:/var/lib/woodpecker'],
          deploy: limits('0.5', '256M'),
          healthcheck: {
            test: ['CMD', 'wget', '-qO-', 'http://localhost:8000/healthz'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '30s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '8000',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.backup.strategy': 'volume',
            'simpledeploy.backup.schedule': '0 3 * * *',
            'simpledeploy.backup.target': 'local',
            'simpledeploy.backup.retention': '14',
            'simpledeploy.alert.cpu': '85',
            'simpledeploy.alert.memory': '85',
          },
        },
        agent: {
          image: 'woodpeckerci/woodpecker-agent:v3',
          restart: 'unless-stopped',
          command: 'agent',
          environment: {
            WOODPECKER_SERVER: 'server:9000',
            WOODPECKER_AGENT_SECRET: '{{agent_secret}}',
            WOODPECKER_MAX_WORKFLOWS: '2',
          },
          volumes: ['/var/run/docker.sock:/var/run/docker.sock'],
          depends_on: { server: { condition: 'service_healthy' } },
          deploy: limits('0.5', '256M'),
          healthcheck: {
            test: ['CMD', '/bin/woodpecker-agent', 'ping'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '20s',
          },
        },
      },
      volumes: { 'woodpecker-server-data': {} },
    },
    notes: [
      'The agent mounts the host Docker socket to run pipelines. Treat pipeline access like shell access.',
      'You must configure a forge (GitHub/Gitea/GitLab) in the server env before first login; see Woodpecker docs.',
    ],
  },

  // 20. webhook-tester
  {
    id: 'webhook-tester',
    name: 'Webhook Tester',
    icon: '🪝',
    category: 'dev-tools',
    description: 'Inspect and debug incoming webhooks in the browser.',
    tags: ['webhook', 'debug', 'testing', 'http'],
    nameSuggestion: 'webhook-tester',
    advanced: false,
    variables: [domainVar],
    compose: {
      services: {
        webhook: {
          image: 'tarampampam/webhook-tester:2.3',
          restart: 'unless-stopped',
          command: 'start --port 8080',
          deploy: limits('0.25', '128M'),
          healthcheck: {
            test: ['CMD', '/bin/app', 'start', 'healthcheck'],
            interval: '30s',
            timeout: '5s',
            retries: 3,
            start_period: '10s',
          },
          labels: {
            'simpledeploy.endpoints.0.domain': '{{domain}}',
            'simpledeploy.endpoints.0.port': '8080',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
            'simpledeploy.alert.cpu': '80',
            'simpledeploy.alert.memory': '85',
          },
        },
      },
    },
    notes: [
      'Data is in-memory by default; captured requests are lost on restart.',
    ],
  },
];
