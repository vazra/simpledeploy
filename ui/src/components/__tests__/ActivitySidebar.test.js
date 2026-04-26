import { describe, test, expect, vi, beforeEach } from 'vitest'
import { render, fireEvent, waitFor } from '@testing-library/svelte'

const apiMock = vi.hoisted(() => ({ listRecentActivity: vi.fn() }))

vi.mock('../../lib/api.js', () => ({ api: apiMock }))

import ActivitySidebar from '../ActivitySidebar.svelte'

const entry = (id, summary = `entry ${id}`) => ({
  id,
  category: 'compose',
  action: 'changed',
  summary,
  actor_name: 'Ameen',
  actor_source: 'ui',
  app_slug: 'app1',
  created_at: new Date().toISOString(),
  sync_status: 'pending',
})

describe('ActivitySidebar', () => {
  beforeEach(() => {
    apiMock.listRecentActivity.mockReset()
    Object.defineProperty(document, 'visibilityState', { value: 'visible', writable: true })
  })

  test('renders toggle button always', async () => {
    apiMock.listRecentActivity.mockResolvedValue({ data: { entries: [] } })
    const { getByTestId } = render(ActivitySidebar)
    expect(getByTestId('activity-sidebar-toggle')).toBeTruthy()
  })

  test('toggle opens and closes sidebar', async () => {
    apiMock.listRecentActivity.mockResolvedValue({ data: { entries: [] } })
    const { getByTestId } = render(ActivitySidebar)
    const toggle = getByTestId('activity-sidebar-toggle')
    const sidebar = getByTestId('activity-sidebar')

    expect(sidebar.classList.contains('translate-x-full')).toBe(true)
    await fireEvent.click(toggle)
    expect(sidebar.classList.contains('translate-x-0')).toBe(true)
    await fireEvent.click(toggle)
    expect(sidebar.classList.contains('translate-x-full')).toBe(true)
  })

  test('renders entries when fetched', async () => {
    apiMock.listRecentActivity.mockResolvedValue({ data: { entries: [entry(1, 'first event')] } })
    const { getByTestId, container } = render(ActivitySidebar)
    await waitFor(() => expect(apiMock.listRecentActivity).toHaveBeenCalled())
    await fireEvent.click(getByTestId('activity-sidebar-toggle'))
    await waitFor(() => expect(container.textContent).toContain('first event'))
  })

  test('shows unseen count badge when new entries arrive after first poll', async () => {
    apiMock.listRecentActivity
      .mockResolvedValueOnce({ data: { entries: [entry(5)] } })
      .mockResolvedValueOnce({ data: { entries: [entry(7), entry(6), entry(5)] } })

    const { getByTestId } = render(ActivitySidebar)
    await waitFor(() => expect(apiMock.listRecentActivity).toHaveBeenCalledTimes(1))
    // Trigger second poll via visibility event
    document.dispatchEvent(new Event('visibilitychange'))
    await waitFor(() => expect(apiMock.listRecentActivity).toHaveBeenCalledTimes(2))

    // Sidebar should auto-open on new activity
    const sidebar = getByTestId('activity-sidebar')
    await waitFor(() => expect(sidebar.classList.contains('translate-x-0')).toBe(true))
  })
})
