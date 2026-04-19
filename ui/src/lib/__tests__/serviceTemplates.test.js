import { describe, it, expect } from 'vitest';
import { serviceTemplates } from '../serviceTemplates.js';

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
