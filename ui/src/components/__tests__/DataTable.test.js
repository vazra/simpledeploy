import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import DataTable from '../DataTable.svelte';

describe('DataTable', () => {
  it('shows empty message when rows is empty', () => {
    const { getByText, queryByRole } = render(DataTable, {
      columns: [{ key: 'a', label: 'A' }],
      rows: [],
    });
    expect(getByText('No data.')).toBeInTheDocument();
    expect(queryByRole('table')).toBeNull();
  });

  it('respects a custom empty message', () => {
    const { getByText } = render(DataTable, {
      columns: [{ key: 'a', label: 'A' }],
      rows: [],
      emptyMessage: 'Nothing to see.',
    });
    expect(getByText('Nothing to see.')).toBeInTheDocument();
  });

  it('renders headers and cell values', () => {
    const { getByText, getAllByRole } = render(DataTable, {
      columns: [
        { key: 'name', label: 'Name' },
        { key: 'role', label: 'Role' },
      ],
      rows: [
        { name: 'Ada', role: 'admin' },
        { name: 'Bob', role: 'viewer' },
      ],
    });
    expect(getByText('Name')).toBeInTheDocument();
    expect(getByText('Role')).toBeInTheDocument();
    expect(getByText('Ada')).toBeInTheDocument();
    expect(getByText('viewer')).toBeInTheDocument();
    expect(getAllByRole('row').length).toBe(3); // header + 2
  });

  it('renders missing values as empty string, not undefined', () => {
    const { queryByText } = render(DataTable, {
      columns: [{ key: 'missing', label: 'Missing' }],
      rows: [{}],
    });
    expect(queryByText('undefined')).toBeNull();
  });

  it('uses column.render when provided', () => {
    const { getByText } = render(DataTable, {
      columns: [{ key: 'n', label: 'N', render: (row) => `#${row.n}` }],
      rows: [{ n: 7 }],
    });
    expect(getByText('#7')).toBeInTheDocument();
  });
});
