import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return { api: makeApiMock() };
});

import { api } from '../../lib/api.js';
import BackupWizard from '../BackupWizard.svelte';

const S3_KEYS = ['access_key', 'bucket', 'endpoint', 'prefix', 'region', 'secret_key'];

beforeEach(() => {
  vi.clearAllMocks();
  api.detectStrategies.mockResolvedValue({
    data: { strategies: [{ strategy_type: 'volume', label: 'Files & Volumes', available: true }] },
    error: null,
    status: 200,
  });
});

async function clickNext(utils) {
  const next = utils.getByRole('button', { name: 'Next' });
  await waitFor(() => expect(next).not.toBeDisabled());
  await fireEvent.click(next);
}

// Open the wizard, pick the detected strategy and choose S3.
async function openS3Step(props = {}) {
  const utils = render(BackupWizard, { open: true, slug: 'foo', ...props });
  await clickNext(utils);
  await fireEvent.click(utils.getByText('S3-Compatible Storage'));
  return utils;
}

async function fillS3(utils) {
  const set = (placeholder, value) => fireEvent.input(utils.getByPlaceholderText(placeholder), { target: { value } });
  await set('https://s3.amazonaws.com', 'https://minio.example.com');
  await set('my-backups', 'my-bucket');
  await set('backups/', 'nightly');
  await set('AKIAIOSFODNN7EXAMPLE', 'AKIDTEST');
  await set('••••••••', 'shh');
  await set('us-east-1', 'eu-west-1');
}

const expectedS3 = {
  endpoint: 'https://minio.example.com',
  bucket: 'my-bucket',
  prefix: 'nightly',
  access_key: 'AKIDTEST',
  secret_key: 'shh',
  region: 'eu-west-1',
};

describe('BackupWizard', () => {
  it('is hidden when closed', () => {
    const { queryByRole } = render(BackupWizard, { open: false, slug: 'foo' });
    expect(queryByRole('dialog')).toBeNull();
  });

  it('renders step controls when open', () => {
    const { container } = render(BackupWizard, { open: true, slug: 'foo' });
    expect(container.textContent.length).toBeGreaterThan(0);
  });

  describe('Test S3 connection', () => {
    async function testConnection(response) {
      api.testS3.mockResolvedValueOnce(response);
      const utils = await openS3Step();
      await fillS3(utils);
      await fireEvent.click(utils.getByRole('button', { name: 'Test Connection' }));
      return utils;
    }

    it('sends the snake_case S3 fields the server reads', async () => {
      await testConnection({ data: { ok: true }, error: null, status: 200 });
      await waitFor(() => expect(api.testS3).toHaveBeenCalledTimes(1));
      const sent = api.testS3.mock.calls[0][0];
      expect(Object.keys(sent).sort()).toEqual(S3_KEYS);
      expect(sent).toEqual(expectedS3);
    });

    it('shows success when the server reports ok: true', async () => {
      const utils = await testConnection({ data: { ok: true }, error: null, status: 200 });
      expect(await utils.findByText('Connection successful')).toHaveClass('text-success');
    });

    it('shows the server error when a 200 response says ok: false', async () => {
      const utils = await testConnection({
        data: { ok: false, error: 's3 put: api error InvalidAccessKeyId: bad key' },
        error: null,
        status: 200,
      });
      const msg = await utils.findByText('Connection failed: s3 put: api error InvalidAccessKeyId: bad key');
      expect(msg).toHaveClass('text-danger');
      expect(utils.queryByText('Connection successful')).toBeNull();
    });

    it('does not report success for a 200 response without ok: true', async () => {
      const utils = await testConnection({ data: null, error: null, status: 200 });
      expect(await utils.findByText(/^Connection failed\./)).toHaveClass('text-danger');
      expect(utils.queryByText('Connection successful')).toBeNull();
    });

    it('shows 4xx errors as before', async () => {
      const utils = await testConnection({
        data: null,
        error: 'S3 endpoint "minio" must be a full http:// or https:// address',
        status: 400,
      });
      const msg = await utils.findByText('S3 endpoint "minio" must be a full http:// or https:// address');
      expect(msg).toHaveClass('text-danger');
      expect(utils.queryByText('Connection successful')).toBeNull();
    });
  });

  describe('saving an S3 config', () => {
    async function finish(utils, buttonName) {
      for (let i = 0; i < 4; i++) await clickNext(utils); // steps 2 -> 6
      await fireEvent.click(utils.getByRole('button', { name: buttonName }));
    }

    it('stores target_config_json with snake_case keys', async () => {
      const utils = await openS3Step();
      await fillS3(utils);
      await finish(utils, 'Create Backup');
      await waitFor(() => expect(api.createBackupConfig).toHaveBeenCalledTimes(1));
      const [slug, cfg] = api.createBackupConfig.mock.calls[0];
      expect(slug).toBe('foo');
      expect(cfg.target).toBe('s3');
      const stored = JSON.parse(cfg.target_config_json);
      expect(Object.keys(stored).sort()).toEqual(S3_KEYS);
      expect(stored).toEqual(expectedS3);
    });

    it('loads a stored config that uses the legacy key names and saves snake_case', async () => {
      const editConfig = {
        id: 5,
        strategy: 'volume',
        target: 's3',
        schedule_cron: '0 2 * * *',
        retention_mode: 'count',
        retention_count: 3,
        target_config_json: JSON.stringify({
          Endpoint: 'https://minio.example.com',
          Bucket: 'old-bucket',
          Prefix: 'old',
          AccessKey: 'AKIDOLD',
          SecretKey: 'oldsecret',
          Region: 'eu-west-1',
        }),
      };
      const utils = render(BackupWizard, { open: true, slug: 'foo', editConfig });
      await clickNext(utils);
      expect(utils.getByPlaceholderText('my-backups')).toHaveValue('old-bucket');
      expect(utils.getByPlaceholderText('AKIAIOSFODNN7EXAMPLE')).toHaveValue('AKIDOLD');
      expect(utils.getByPlaceholderText('••••••••')).toHaveValue('oldsecret');
      await finish(utils, 'Save Changes');
      await waitFor(() => expect(api.updateBackupConfig).toHaveBeenCalledTimes(1));
      const [id, cfg] = api.updateBackupConfig.mock.calls[0];
      expect(id).toBe(5);
      expect(JSON.parse(cfg.target_config_json)).toEqual({
        endpoint: 'https://minio.example.com',
        bucket: 'old-bucket',
        prefix: 'old',
        access_key: 'AKIDOLD',
        secret_key: 'oldsecret',
        region: 'eu-west-1',
      });
    });
  });
});
