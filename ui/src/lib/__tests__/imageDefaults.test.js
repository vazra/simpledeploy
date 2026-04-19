import { describe, it, expect } from 'vitest';
import { getImageDefaults, getHealthcheckSuggestion } from '../imageDefaults.js';

describe('getImageDefaults', () => {
  it('returns null for missing image name', () => {
    expect(getImageDefaults('')).toBeNull();
    expect(getImageDefaults(null)).toBeNull();
    expect(getImageDefaults(undefined)).toBeNull();
  });

  it('returns null for an unknown image', () => {
    expect(getImageDefaults('gcr.io/something/weird:1.2')).toBeNull();
  });

  it('matches postgres and supplies healthcheck + volumes', () => {
    const d = getImageDefaults('postgres:16-alpine');
    expect(d).not.toBeNull();
    expect(d.volumes).toEqual(['pgdata:/var/lib/postgresql/data']);
    expect(d.healthcheck.test[1]).toBe('pg_isready -U postgres');
    expect(d.environment.POSTGRES_PASSWORD).toBeDefined();
  });

  it('matches mariadb before mysql (both contain "maria" but base check is substring)', () => {
    const d = getImageDefaults('mariadb:11');
    expect(d).not.toBeNull();
    expect(d.environment.MARIADB_ROOT_PASSWORD).toBeDefined();
    expect(d.environment.MYSQL_ROOT_PASSWORD).toBeUndefined();
  });

  it('matches mysql', () => {
    const d = getImageDefaults('mysql:8');
    expect(d.environment.MYSQL_ROOT_PASSWORD).toBeDefined();
  });

  it('matches redis', () => {
    const d = getImageDefaults('redis:7-alpine');
    expect(d.command).toBe('redis-server --appendonly yes');
  });

  it('matches mongo with the mongo-style healthcheck', () => {
    const d = getImageDefaults('mongo:7');
    expect(d.volumes).toEqual(['mongodata:/data/db']);
    expect(d.healthcheck.test[1]).toMatch(/mongosh/);
  });

  it('matches rabbitmq and exposes management port', () => {
    const d = getImageDefaults('rabbitmq:3-management');
    expect(d.ports).toContain('15672:15672');
  });

  it('matches nginx with port mapping', () => {
    const d = getImageDefaults('nginx:latest');
    expect(d.ports).toEqual(['80:80']);
  });

  it('is case-insensitive on image base', () => {
    const d = getImageDefaults('Nginx:alpine');
    expect(d).not.toBeNull();
  });

  it('matches registry-prefixed images too (substring on base)', () => {
    expect(getImageDefaults('library/postgres:16')).not.toBeNull();
    expect(getImageDefaults('ghcr.io/vendor/redis:latest')).not.toBeNull();
  });
});

describe('getHealthcheckSuggestion', () => {
  it('returns null for missing input', () => {
    expect(getHealthcheckSuggestion('')).toBeNull();
    expect(getHealthcheckSuggestion(null)).toBeNull();
  });

  it('returns null for an unknown image', () => {
    expect(getHealthcheckSuggestion('alpine')).toBeNull();
  });

  it('gives a command for each supported image family', () => {
    expect(getHealthcheckSuggestion('postgres:16')).toMatch(/pg_isready/);
    expect(getHealthcheckSuggestion('mysql:8')).toMatch(/mysqladmin/);
    expect(getHealthcheckSuggestion('mariadb:11')).toMatch(/healthcheck\.sh/);
    expect(getHealthcheckSuggestion('redis:7')).toMatch(/redis-cli/);
    expect(getHealthcheckSuggestion('mongo:7')).toMatch(/mongosh/);
    expect(getHealthcheckSuggestion('rabbitmq:3')).toMatch(/rabbitmq-diagnostics/);
  });
});
