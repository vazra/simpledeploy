import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  formatBytes,
  formatBytesShort,
  formatTime,
  formatDate,
  timeAgo,
} from '../format.js';

describe('formatBytes', () => {
  it('returns "0 B" for 0, null, undefined', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(null)).toBe('0 B');
    expect(formatBytes(undefined)).toBe('0 B');
  });

  it('formats bytes', () => {
    expect(formatBytes(512)).toBe('512.0 B');
  });

  it('formats kilobytes', () => {
    expect(formatBytes(1024)).toBe('1.0 KB');
    expect(formatBytes(1536)).toBe('1.5 KB');
  });

  it('formats megabytes', () => {
    expect(formatBytes(1024 * 1024)).toBe('1.0 MB');
    expect(formatBytes(5 * 1024 * 1024)).toBe('5.0 MB');
  });

  it('formats gigabytes', () => {
    expect(formatBytes(1024 * 1024 * 1024)).toBe('1.0 GB');
  });

  it('formats terabytes', () => {
    expect(formatBytes(1024 ** 4)).toBe('1.0 TB');
  });
});

describe('formatBytesShort', () => {
  it('returns "0" for 0 or falsy', () => {
    expect(formatBytesShort(0)).toBe('0');
    expect(formatBytesShort(null)).toBe('0');
  });

  it('has no decimal places', () => {
    expect(formatBytesShort(1536)).toBe('2 KB');
    expect(formatBytesShort(1024 * 1024 * 1.9)).toBe('2 MB');
  });
});

describe('formatTime', () => {
  it('returns "" for falsy input', () => {
    expect(formatTime(null)).toBe('');
    expect(formatTime(undefined)).toBe('');
    expect(formatTime(0)).toBe('');
  });

  it('returns HH:MM style string for an epoch ms value', () => {
    const out = formatTime(new Date('2026-01-01T13:45:00Z').toISOString());
    expect(out).toMatch(/^\d{1,2}:\d{2}(\s?(AM|PM))?$/i);
  });
});

describe('formatDate', () => {
  it('returns "" for falsy input', () => {
    expect(formatDate(null)).toBe('');
    expect(formatDate(undefined)).toBe('');
  });

  it('returns a non-empty locale string', () => {
    const out = formatDate('2026-01-01T00:00:00Z');
    expect(typeof out).toBe('string');
    expect(out.length).toBeGreaterThan(0);
  });
});

describe('timeAgo', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-04-18T12:00:00Z'));
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns "-" for falsy input', () => {
    expect(timeAgo(null)).toBe('-');
    expect(timeAgo(undefined)).toBe('-');
  });

  it('returns "just now" for <60s old', () => {
    const ts = new Date('2026-04-18T11:59:30Z').toISOString();
    expect(timeAgo(ts)).toBe('just now');
  });

  it('returns minutes for 1-59m', () => {
    const ts = new Date('2026-04-18T11:45:00Z').toISOString();
    expect(timeAgo(ts)).toBe('15m ago');
  });

  it('returns hours for 1-23h', () => {
    const ts = new Date('2026-04-18T09:00:00Z').toISOString();
    expect(timeAgo(ts)).toBe('3h ago');
  });

  it('returns days past 24h', () => {
    const ts = new Date('2026-04-15T12:00:00Z').toISOString();
    expect(timeAgo(ts)).toBe('3d ago');
  });
});
