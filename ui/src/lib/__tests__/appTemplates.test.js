import { describe, it, expect, vi } from 'vitest';
import yaml from 'js-yaml';
import {
  categories,
  appTemplates,
  applyVars,
  generateSecret,
  validateVars,
  suggestName,
  applyAccessMode,
  countEndpoints,
  computeQuickTestDomain,
  isValidHost,
} from '../appTemplates.js';

function singleEndpointCompose() {
  return {
    services: {
      web: {
        image: 'nginx',
        labels: {
          'simpledeploy.endpoints.0.domain': '{{domain}}',
          'simpledeploy.endpoints.0.port': '80',
          'simpledeploy.endpoints.0.tls': 'letsencrypt',
          'simpledeploy.alert.cpu': '80',
        },
      },
    },
  };
}

function multiEndpointCompose() {
  return {
    services: {
      a: {
        image: 'img-a',
        labels: {
          'simpledeploy.endpoints.0.domain': '{{d1}}',
          'simpledeploy.endpoints.0.port': '8080',
          'simpledeploy.endpoints.0.tls': 'letsencrypt',
        },
      },
      b: {
        image: 'img-b',
        labels: {
          'simpledeploy.endpoints.0.domain': '{{d2}}',
          'simpledeploy.endpoints.0.port': '9000',
          'simpledeploy.endpoints.0.tls': 'letsencrypt',
          'simpledeploy.endpoints.1.domain': '{{d3}}',
          'simpledeploy.endpoints.1.port': '9001',
          'simpledeploy.endpoints.1.tls': 'letsencrypt',
        },
      },
    },
  };
}

describe('countEndpoints', () => {
  it('counts single endpoint', () => {
    expect(countEndpoints(singleEndpointCompose())).toBe(1);
  });
  it('counts multi endpoints across services', () => {
    expect(countEndpoints(multiEndpointCompose())).toBe(3);
  });
  it('returns 0 for missing services', () => {
    expect(countEndpoints({})).toBe(0);
  });
});

describe('applyAccessMode', () => {
  it('custom preserves tls=letsencrypt', () => {
    const c = singleEndpointCompose();
    const out = applyAccessMode(c, 'custom');
    expect(out.services.web.labels['simpledeploy.endpoints.0.tls']).toBe('letsencrypt');
  });

  it('quick-test rewrites all tls labels to local', () => {
    const c = multiEndpointCompose();
    const out = applyAccessMode(c, 'quick-test');
    expect(out.services.a.labels['simpledeploy.endpoints.0.tls']).toBe('local');
    expect(out.services.b.labels['simpledeploy.endpoints.0.tls']).toBe('local');
    expect(out.services.b.labels['simpledeploy.endpoints.1.tls']).toBe('local');
    expect(c.services.a.labels['simpledeploy.endpoints.0.tls']).toBe('letsencrypt');
  });

  it('port-only strips endpoint labels and adds ports', () => {
    const c = singleEndpointCompose();
    const out = applyAccessMode(c, 'port-only');
    const labels = out.services.web.labels;
    for (const k of Object.keys(labels)) {
      expect(k.startsWith('simpledeploy.endpoints.')).toBe(false);
    }
    expect(labels['simpledeploy.alert.cpu']).toBe('80');
    expect(out.services.web.ports).toEqual(['0:80']);
  });
});

describe('computeQuickTestDomain', () => {
  it('formats slug.host.sslip.io', () => {
    expect(computeQuickTestDomain('my-app', '1.2.3.4')).toBe('my-app.1.2.3.4.sslip.io');
  });
});

describe('isValidHost', () => {
  it('accepts IPv4', () => {
    expect(isValidHost('192.168.1.1')).toBe(true);
    expect(isValidHost('0.0.0.0')).toBe(true);
  });
  it('accepts hostnames', () => {
    expect(isValidHost('example.com')).toBe(true);
    expect(isValidHost('my-server')).toBe(true);
  });
  it('rejects invalid', () => {
    expect(isValidHost('')).toBe(false);
    expect(isValidHost('.example.com')).toBe(false);
    expect(isValidHost('example.com.')).toBe(false);
    expect(isValidHost('bad host')).toBe(false);
    expect(isValidHost('999.999.999.999')).toBe(false);
  });
});

describe('applyVars', () => {
  it('substitutes top-level string tokens', () => {
    expect(applyVars('hello {{name}}', { name: 'world' })).toBe('hello world');
  });

  it('substitutes inside nested objects', () => {
    expect(applyVars({ a: { b: '{{x}}' } }, { x: 1 })).toEqual({ a: { b: '1' } });
  });

  it('substitutes inside arrays', () => {
    expect(applyVars(['{{a}}', '{{b}}'], { a: 1, b: 2 })).toEqual(['1', '2']);
  });

  it('leaves unknown tokens intact', () => {
    expect(applyVars('{{foo}}', {})).toBe('{{foo}}');
  });

  it('does not substitute object keys (keys preserved literally)', () => {
    const input = { '{{k}}': 1 };
    const out = applyVars(input, { k: 'x' });
    expect(out).toEqual({ '{{k}}': 1 });
    expect(Object.keys(out)).toEqual(['{{k}}']);
  });

  it('returns numbers unchanged', () => {
    expect(applyVars(42, { a: 1 })).toBe(42);
  });

  it('returns booleans unchanged', () => {
    expect(applyVars(true, {})).toBe(true);
    expect(applyVars(false, {})).toBe(false);
  });

  it('returns null unchanged', () => {
    expect(applyVars(null, {})).toBe(null);
  });

  it('does not mutate the input object', () => {
    const input = { a: { b: '{{x}}', c: ['{{y}}'] } };
    const snapshot = JSON.parse(JSON.stringify(input));
    applyVars(input, { x: 'X', y: 'Y' });
    expect(input).toEqual(snapshot);
  });

  it('handles multiple tokens in one string', () => {
    expect(applyVars('{{a}}-{{b}}', { a: 1, b: 2 })).toBe('1-2');
  });

  it('tolerates whitespace inside tokens', () => {
    expect(applyVars('{{ name }}', { name: 'val' })).toBe('val');
  });
});

describe('generateSecret', () => {
  it('defaults to length 24', () => {
    expect(generateSecret()).toHaveLength(24);
  });

  it('returns the requested length', () => {
    expect(generateSecret(10)).toHaveLength(10);
    expect(generateSecret(64)).toHaveLength(64);
  });

  it('uses only base58 characters with base58 charset', () => {
    const base58 = '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz';
    const out = generateSecret(500, 'base58');
    for (const ch of out) {
      expect(base58.includes(ch)).toBe(true);
    }
  });

  it('uses only hex characters with hex charset', () => {
    const out = generateSecret(500, 'hex');
    expect(/^[0-9a-f]+$/.test(out)).toBe(true);
  });

  it('uses only alphanum characters with alphanum charset', () => {
    const out = generateSecret(500, 'alphanum');
    expect(/^[A-Za-z0-9]+$/.test(out)).toBe(true);
  });

  it('throws when globalThis.crypto is missing', () => {
    const spy = vi.spyOn(globalThis, 'crypto', 'get').mockReturnValue(undefined);
    try {
      expect(() => generateSecret(16)).toThrow(/secure random/i);
    } finally {
      spy.mockRestore();
    }
  });

  it('throws when the requested charset is unknown', () => {
    expect(() => generateSecret(16, 'nope')).toThrow(/unknown charset/i);
  });

  it('produces mostly distinct values across 1000 calls', () => {
    const seen = new Set();
    for (let i = 0; i < 1000; i++) {
      seen.add(generateSecret(16));
    }
    expect(seen.size).toBeGreaterThanOrEqual(900);
  });
});

describe('validateVars', () => {
  it('returns empty object when all required fields are filled', () => {
    const vars = [
      { key: 'name', label: 'Name', required: true, type: 'text' },
      { key: 'email', label: 'Email', required: true, type: 'email' },
    ];
    const errors = validateVars(vars, { name: 'foo', email: 'a@b.co' });
    expect(errors).toEqual({});
  });

  it('reports error when a required field is missing', () => {
    const vars = [{ key: 'name', label: 'Name', required: true, type: 'text' }];
    const errors = validateVars(vars, {});
    expect(errors.name).toMatch(/required/i);
  });

  it('reports error on invalid email format', () => {
    const vars = [{ key: 'email', label: 'Email', required: true, type: 'email' }];
    const errors = validateVars(vars, { email: 'not-an-email' });
    expect(errors.email).toMatch(/email/i);
  });

  it('reports error on invalid domain format', () => {
    const vars = [{ key: 'dom', label: 'Domain', required: true, type: 'domain' }];
    const errors = validateVars(vars, { dom: 'not a domain!!' });
    expect(errors.dom).toMatch(/domain/i);
  });

  it('reports error when pattern fails', () => {
    const vars = [
      {
        key: 'user',
        label: 'User',
        required: true,
        type: 'text',
        pattern: '^[a-z]{3,}$',
        patternMessage: 'lowercase only',
      },
    ];
    const errors = validateVars(vars, { user: 'ABC' });
    expect(errors.user).toBe('lowercase only');
  });

  it('optional empty field produces no error', () => {
    const vars = [{ key: 'nick', label: 'Nick', required: false, type: 'text' }];
    const errors = validateVars(vars, {});
    expect(errors).toEqual({});
  });
});

describe('suggestName', () => {
  it('returns base when unused', () => {
    expect(suggestName('foo', [])).toBe('foo');
  });

  it('returns base-2 when base is taken', () => {
    expect(suggestName('foo', ['foo'])).toBe('foo-2');
  });

  it('returns base-3 when base and base-2 are taken', () => {
    expect(suggestName('foo', ['foo', 'foo-2'])).toBe('foo-3');
  });

  it('returns base when only unrelated names exist', () => {
    expect(suggestName('foo', ['bar'])).toBe('foo');
  });
});

describe('appTemplates integrity', () => {
  const categoryIds = new Set(categories.map((c) => c.id));

  it('has the expected number of templates', () => {
    expect(appTemplates.length).toBe(20);
  });

  it('every template has required fields', () => {
    for (const t of appTemplates) {
      expect(t.id, `missing id`).toBeTruthy();
      expect(t.name, `${t.id}: missing name`).toBeTruthy();
      expect(t.icon, `${t.id}: missing icon`).toBeTruthy();
      expect(t.category, `${t.id}: missing category`).toBeTruthy();
      expect(t.description, `${t.id}: missing description`).toBeTruthy();
      expect(t.compose, `${t.id}: missing compose`).toBeTruthy();
      expect(typeof t.compose).toBe('object');
    }
  });

  it('template ids are unique', () => {
    const ids = appTemplates.map((t) => t.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it('every template category references a valid category entry', () => {
    for (const t of appTemplates) {
      expect(categoryIds.has(t.category), `${t.id}: unknown category ${t.category}`).toBe(true);
    }
  });

  it('every template has at least one simpledeploy.endpoints.0.domain label bound to a declared variable', () => {
    const tokenRe = /^\{\{\s*([a-zA-Z0-9_]+)\s*\}\}$/;
    for (const t of appTemplates) {
      const services = (t.compose && t.compose.services) || {};
      const varKeys = new Set((t.variables || []).map(v => v.key));
      let found = 0;
      for (const svcName of Object.keys(services)) {
        const labels = services[svcName].labels || {};
        const v = labels['simpledeploy.endpoints.0.domain'];
        if (v === undefined) continue;
        found++;
        const m = tokenRe.exec(v);
        expect(m, `${t.id}/${svcName}: endpoints.0.domain is not a {{var}} token: ${v}`).not.toBeNull();
        expect(varKeys.has(m[1]), `${t.id}/${svcName}: {{${m[1]}}} not declared in variables[]`).toBe(true);
      }
      expect(found, `${t.id}: expected at least one endpoints.0.domain label`).toBeGreaterThanOrEqual(1);
    }
  });

  it('every {{token}} in compose has a matching variables[] entry', () => {
    const tokenRe = /\{\{\s*([a-zA-Z0-9_]+)\s*\}\}/g;
    const collect = (node, acc) => {
      if (node == null) return;
      if (typeof node === 'string') {
        let m;
        while ((m = tokenRe.exec(node)) !== null) acc.add(m[1]);
        tokenRe.lastIndex = 0;
        return;
      }
      if (Array.isArray(node)) {
        for (const x of node) collect(x, acc);
        return;
      }
      if (typeof node === 'object') {
        for (const v of Object.values(node)) collect(v, acc);
      }
    };
    for (const t of appTemplates) {
      const tokens = new Set();
      collect(t.compose, tokens);
      const declared = new Set((t.variables || []).map((v) => v.key));
      for (const tok of tokens) {
        expect(
          declared.has(tok),
          `${t.id}: compose references {{${tok}}} but variables[] has no entry for it`
        ).toBe(true);
      }
    }
  });

  it('applyVars + yaml round-trip succeeds for every template', () => {
    for (const t of appTemplates) {
      const synthetic = {};
      for (const v of t.variables || []) {
        if (v.type === 'domain') synthetic[v.key] = 'example.com';
        else if (v.type === 'email') synthetic[v.key] = 'a@b.co';
        else if (v.type === 'secret') synthetic[v.key] = generateSecret(32);
        else synthetic[v.key] = v.default || 'admin';
      }
      const rendered = applyVars(t.compose, synthetic);
      let dumped;
      expect(() => {
        dumped = yaml.dump(rendered);
      }, `${t.id}: yaml.dump threw`).not.toThrow();
      expect(() => {
        yaml.load(dumped);
      }, `${t.id}: yaml.load threw`).not.toThrow();
    }
  });

  it('advanced:true is set on exactly authelia and poste-io', () => {
    const advanced = appTemplates.filter((t) => t.advanced === true).map((t) => t.id).sort();
    expect(advanced).toEqual(['authelia', 'poste-io']);
  });
});
