import { describe, it, expect } from 'vitest';
import { serviceTemplates, namedVolumeSource, addServiceToCompose } from '../serviceTemplates.js';
import { appTemplates } from '../appTemplates.js';

// Named volumes referenced by any service, as a sorted list.
function referencedVolumes(compose) {
  const names = new Set();
  for (const svc of Object.values(compose.services || {})) {
    for (const v of svc.volumes || []) {
      const n = namedVolumeSource(v);
      if (n) names.add(n);
    }
  }
  return [...names].sort();
}

describe('namedVolumeSource', () => {
  it('returns the volume name for short syntax named volumes', () => {
    expect(namedVolumeSource('pgdata:/var/lib/postgresql/data')).toBe('pgdata');
    expect(namedVolumeSource('my_vol.1:/data:ro')).toBe('my_vol.1');
  });

  it('returns null for bind mounts and anonymous volumes', () => {
    expect(namedVolumeSource('/etc/localtime:/etc/localtime:ro')).toBeNull();
    expect(namedVolumeSource('./data:/data')).toBeNull();
    expect(namedVolumeSource('~/data:/data')).toBeNull();
    expect(namedVolumeSource('${DATA_DIR}:/data')).toBeNull();
    expect(namedVolumeSource('/data')).toBeNull();
  });

  it('handles long syntax', () => {
    expect(namedVolumeSource({ type: 'volume', source: 'data', target: '/data' })).toBe('data');
    expect(namedVolumeSource({ type: 'bind', source: '/srv', target: '/data' })).toBeNull();
    expect(namedVolumeSource({ type: 'volume', target: '/data' })).toBeNull();
  });
});

describe('addServiceToCompose', () => {
  it.each(serviceTemplates.map((t) => [t.id, t]))(
    '%s: declares every named volume its service references',
    (_id, t) => {
      const compose = { services: {} };
      const name = addServiceToCompose(compose, t);
      expect(name).toBe(t.id.replace(/-/g, '_'));
      expect(compose.services[name].image).toBe(t.config.image);
      expect(Object.keys(compose.volumes || {}).sort()).toEqual(referencedVolumes(compose));
    },
  );

  it('each template added twice gets separate volumes for the second copy', () => {
    for (const t of serviceTemplates) {
      const compose = { services: {} };
      addServiceToCompose(compose, t);
      const second = addServiceToCompose(compose, t);
      const first = referencedVolumes({ services: { a: compose.services[t.id.replace(/-/g, '_')] } });
      const mine = referencedVolumes({ services: { a: compose.services[second] } });
      expect(mine.filter((v) => first.includes(v)), t.id).toEqual([]);
      expect(Object.keys(compose.volumes || {}).sort(), t.id).toEqual(referencedVolumes(compose));
    }
  });

  it('suffixes clashing volumes like service names and keeps existing entries', () => {
    const pg = serviceTemplates.find((t) => t.id === 'postgresql');
    const compose = {
      services: { other: { image: 'x', volumes: ['pgdata2:/x'] } },
      volumes: { pgdata: { driver: 'local' } },
    };
    const name = addServiceToCompose(compose, pg);
    expect(name).toBe('postgresql');
    expect(compose.services.postgresql.volumes).toEqual(['pgdata3:/var/lib/postgresql/data']);
    expect(compose.volumes).toEqual({ pgdata: { driver: 'local' }, pgdata3: {} });
  });

  it('does not declare bind mounts', () => {
    const gitea = serviceTemplates.find((t) => t.id === 'gitea');
    const compose = { services: {} };
    addServiceToCompose(compose, gitea);
    expect(compose.volumes).toEqual({ giteadata: {} });
    expect(compose.services.gitea.volumes).toEqual(gitea.config.volumes);
  });

  it('rewrites long syntax sources when renaming', () => {
    const tpl = { id: 'svc', config: { image: 'x:1', volumes: [{ type: 'volume', source: 'data', target: '/d' }] } };
    const compose = { services: {}, volumes: { data: {} } };
    addServiceToCompose(compose, tpl);
    expect(compose.services.svc.volumes).toEqual([{ type: 'volume', source: 'data2', target: '/d' }]);
    expect(compose.volumes).toEqual({ data: {}, data2: {} });
  });

  it('does not mutate the template config', () => {
    const pg = serviceTemplates.find((t) => t.id === 'postgresql');
    const before = JSON.stringify(pg.config);
    const compose = { services: {}, volumes: { pgdata: {} } };
    addServiceToCompose(compose, pg);
    expect(JSON.stringify(pg.config)).toBe(before);
  });

  it('adds a blank service without template or for the blank id', () => {
    const compose = {};
    expect(addServiceToCompose(compose, null)).toBe('service');
    expect(addServiceToCompose(compose, { id: 'blank' })).toBe('service2');
    expect(compose.services.service).toEqual({ image: '', restart: 'unless-stopped' });
    expect(compose.volumes).toBeUndefined();
  });
});

describe('serviceTemplates integrity', () => {
  it('every template has required fields', () => {
    for (const t of serviceTemplates) {
      expect(t.id, `missing id`).toBeTruthy();
      expect(t.name, `${t.id}: missing name`).toBeTruthy();
      expect(t.icon, `${t.id}: missing icon`).toBeTruthy();
      expect(t.description, `${t.id}: missing description`).toBeTruthy();
      expect(t.config, `${t.id}: missing config`).toBeTruthy();
      expect(typeof t.config).toBe('object');
    }
  });

  it('template ids are unique', () => {
    const ids = serviceTemplates.map((t) => t.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it('every template config has an image with a non-empty tag', () => {
    for (const t of serviceTemplates) {
      const img = t.config.image;
      expect(img, `${t.id}: missing image`).toBeTruthy();
      expect(img.includes(':'), `${t.id}: image missing tag: ${img}`).toBe(true);
      const tag = img.split(':').pop();
      expect(tag, `${t.id}: empty tag`).toBeTruthy();
    }
  });

  it('every template has a restart policy', () => {
    for (const t of serviceTemplates) {
      expect(t.config.restart, `${t.id}: missing restart policy`).toBeTruthy();
    }
  });

  it('when healthcheck is present it has test, interval, and retries', () => {
    for (const t of serviceTemplates) {
      const hc = t.config.healthcheck;
      if (!hc) continue;
      expect(Array.isArray(hc.test) || typeof hc.test === 'string', `${t.id}: invalid healthcheck.test`).toBe(true);
      expect(hc.interval, `${t.id}: missing healthcheck.interval`).toBeTruthy();
      expect(hc.retries, `${t.id}: missing healthcheck.retries`).toBeTruthy();
    }
  });

  it('environment values are never plain objects (must be scalar or string)', () => {
    for (const t of serviceTemplates) {
      const env = t.config.environment || {};
      for (const [k, v] of Object.entries(env)) {
        expect(
          typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean',
          `${t.id}.${k}: env value must be scalar, got ${typeof v}`,
        ).toBe(true);
      }
    }
  });

  it('volume entries use "name:/path" form', () => {
    for (const t of serviceTemplates) {
      for (const vol of t.config.volumes || []) {
        expect(typeof vol).toBe('string');
        expect(vol.includes(':/'), `${t.id}: volume "${vol}" missing ":/"`).toBe(true);
      }
    }
  });
});

describe('minio service template', () => {
  // Regression: minio/minio no longer exists on Docker Hub (and quay.io/minio
  // requires login), so the service template now uses pgsty/silo.
  it('does not use the removed official MinIO images', () => {
    for (const t of serviceTemplates) {
      const img = t.config.image;
      expect(/^(docker\.io\/)?minio\/(minio|mc)([:@]|$)/.test(img), `${t.id}: ${img}`).toBe(false);
      expect(img.startsWith('quay.io/minio/'), `${t.id}: ${img}`).toBe(false);
    }
  });

  it('pins the same pgsty/silo release as the MinIO app template', () => {
    const svc = serviceTemplates.find((t) => t.id === 'minio');
    const app = appTemplates.find((t) => t.id === 'minio');
    expect(svc.config.image).toMatch(/^pgsty\/silo:RELEASE\./);
    expect(svc.config.image).toBe(app.compose.services.minio.image);
    expect(svc.config.command).toBe('server /data --console-address ":9001"');
  });
});
