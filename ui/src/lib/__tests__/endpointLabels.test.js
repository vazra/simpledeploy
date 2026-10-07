import { describe, it, expect } from 'vitest';
import { parseEndpointLabels, writeEndpointLabels } from '../endpointLabels.js';

describe('endpointLabels', () => {
  it('parses protocol and path with defaults', () => {
    const services = {
      co: {
        labels: {
          'simpledeploy.endpoints.0.domain': 'co.example.com',
          'simpledeploy.endpoints.0.port': '50051',
          'simpledeploy.endpoints.0.protocol': 'grpc',
          'simpledeploy.endpoints.1.domain': 'co.example.com',
          'simpledeploy.endpoints.1.port': '8000',
          'simpledeploy.endpoints.1.path': '/ws*',
          'other.label': 'x',
        },
      },
    };
    const eps = parseEndpointLabels(services);
    expect(eps).toEqual([
      { domain: 'co.example.com', port: '50051', tls: 'letsencrypt', service: 'co', protocol: 'grpc' },
      { domain: 'co.example.com', port: '8000', tls: 'letsencrypt', service: 'co', path: '/ws*' },
    ]);
  });

  it('rewrites labels, re-indexes, keeps protocol/path and drops stale ones', () => {
    const services = {
      co: {
        labels: {
          'simpledeploy.endpoints.0.domain': 'old.example.com',
          'simpledeploy.endpoints.0.protocol': 'grpc',
          'simpledeploy.endpoints.1.domain': 'co.example.com',
          'simpledeploy.endpoints.1.path': '/ws*',
          'keep.me': 'yes',
        },
      },
    };
    const eps = parseEndpointLabels(services);
    eps.splice(0, 1); // remove the grpc endpoint
    writeEndpointLabels(services, eps);
    expect(services.co.labels).toEqual({
      'keep.me': 'yes',
      'simpledeploy.endpoints.0.domain': 'co.example.com',
      'simpledeploy.endpoints.0.tls': 'letsencrypt',
      'simpledeploy.endpoints.0.path': '/ws*',
    });
  });

  it('creates labels map on services without one and skips unknown services', () => {
    const services = { web: { image: 'nginx' } };
    writeEndpointLabels(services, [
      { domain: 'a.example.com', port: '80', tls: 'letsencrypt', service: 'web', protocol: 'h2c' },
      { domain: 'b.example.com', port: '80', tls: 'letsencrypt', service: 'ghost' },
    ]);
    expect(services.web.labels).toEqual({
      'simpledeploy.endpoints.0.domain': 'a.example.com',
      'simpledeploy.endpoints.0.port': '80',
      'simpledeploy.endpoints.0.tls': 'letsencrypt',
      'simpledeploy.endpoints.0.protocol': 'h2c',
    });
    expect(services.ghost).toBeUndefined();
  });
});
