// @vitest-environment jsdom

import '@testing-library/jest-dom/vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DEFAULT_SETTINGS } from '../../types/settings';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import { initialUIState } from '../store/ui';
import { AddRepoDialog } from './AddRepoDialog';
import ToastContainer from './ToastContainer';

const requestMock = vi.fn();
const sendMock = vi.fn();
const closeMock = vi.fn();

vi.mock('../hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    request: requestMock,
    send: sendMock,
  }),
}));

function renderDialog(): void {
  render(
    <>
      <AddRepoDialog onClose={closeMock} />
      <ToastContainer />
    </>,
  );
}

describe('AddRepoDialog', () => {
  beforeEach(() => {
    requestMock.mockReset();
    sendMock.mockReset();
    closeMock.mockReset();
    settingsStore.setState(() => DEFAULT_SETTINGS);
    appStore.setState(() => initialUIState);
  });

  afterEach(() => {
    settingsStore.setState(() => DEFAULT_SETTINGS);
    appStore.setState(() => initialUIState);
  });

  it('renders the repository URL input', () => {
    renderDialog();

    expect(
      screen.getByRole('textbox', { name: /repository url/i }),
    ).toBeInTheDocument();
  });

  it('closes with a success toast when the repository is found locally', async () => {
    const user = userEvent.setup();
    settingsStore.setState((settings) => ({
      ...settings,
      repositories: {
        ...settings.repositories,
        devFolderPath: '/Users/sam/dev',
      },
    }));
    requestMock.mockResolvedValueOnce({ ok: true, source: 'local' });

    renderDialog();

    await user.type(
      screen.getByRole('textbox', { name: /repository url/i }),
      'https://github.com/acme/api.git',
    );
    await user.click(screen.getByRole('button', { name: /add repository/i }));

    expect(requestMock).toHaveBeenCalledWith('ADD_REPO_BY_URL', {
      url: 'https://github.com/acme/api.git',
      destination: null,
      dev_folder_path: '/Users/sam/dev',
    });
    await waitFor(() => expect(closeMock).toHaveBeenCalledTimes(1));
    expect(screen.getByRole('alert')).toHaveTextContent('Repository added');
  });

  it('shows both destination options when a dev folder is configured and focuses Dev folder', async () => {
    const user = userEvent.setup();
    settingsStore.setState((settings) => ({
      ...settings,
      repositories: {
        ...settings.repositories,
        devFolderPath: '/Users/sam/dev',
      },
    }));
    requestMock.mockResolvedValueOnce({
      ok: false,
      repo_id: 'github:acme/api',
      operation: 'clone',
      code: 'destination_required',
      message: 'server text is ignored',
    });

    renderDialog();

    await user.type(
      screen.getByRole('textbox', { name: /repository url/i }),
      'https://github.com/acme/api.git',
    );
    await user.click(screen.getByRole('button', { name: /add repository/i }));

    expect(
      await screen.findByRole('button', { name: /dev folder/i }),
    ).toHaveFocus();
    expect(
      screen.getByRole('button', { name: /bistro cache/i }),
    ).toBeInTheDocument();

    requestMock.mockResolvedValueOnce({ ok: true, source: 'local' });
    await user.click(screen.getByRole('button', { name: /dev folder/i }));

    expect(requestMock).toHaveBeenNthCalledWith(2, 'ADD_REPO_BY_URL', {
      url: 'https://github.com/acme/api.git',
      destination: 'dev_folder',
      dev_folder_path: '/Users/sam/dev',
    });
  });

  it('shows only the Bistro cache destination when no dev folder is configured', async () => {
    const user = userEvent.setup();
    requestMock.mockResolvedValueOnce({
      ok: false,
      repo_id: 'github:acme/api',
      operation: 'clone',
      code: 'destination_required',
      message: 'server text is ignored',
    });

    renderDialog();

    await user.type(
      screen.getByRole('textbox', { name: /repository url/i }),
      'https://github.com/acme/api.git',
    );
    await user.click(screen.getByRole('button', { name: /add repository/i }));

    expect(
      await screen.findByRole('button', { name: /bistro cache/i }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: /dev folder/i }),
    ).not.toBeInTheDocument();
    expect(screen.getByText(/set one in settings/i)).toBeInTheDocument();
  });

  it.each([
    ['invalid_url', 'Enter a valid Git repository URL.'],
    [
      'destination_conflict',
      'A repository already exists at that destination.',
    ],
    ['auth_failed', 'Authentication failed while adding the repository.'],
  ])('renders a distinct message for %s', async (code, expectedMessage) => {
    const user = userEvent.setup();
    requestMock.mockResolvedValueOnce({
      ok: false,
      repo_id: 'github:acme/api',
      operation: 'clone',
      code,
      message: 'server text is ignored',
    });

    renderDialog();

    await user.type(
      screen.getByRole('textbox', { name: /repository url/i }),
      'https://github.com/acme/api.git',
    );
    await user.click(screen.getByRole('button', { name: /add repository/i }));

    expect(await screen.findByText(expectedMessage)).toBeInTheDocument();
    expect(
      screen.queryByText('server text is ignored'),
    ).not.toBeInTheDocument();
  });

  it('renders a transport error and re-enables submit when the request rejects', async () => {
    const user = userEvent.setup();
    requestMock.mockRejectedValueOnce(new Error('socket timed out'));

    renderDialog();

    await user.type(
      screen.getByRole('textbox', { name: /repository url/i }),
      'https://github.com/acme/api.git',
    );
    await user.click(screen.getByRole('button', { name: /add repository/i }));

    expect(await screen.findByText('socket timed out')).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /add repository/i }),
    ).toBeEnabled();
    expect(closeMock).not.toHaveBeenCalled();
  });

  it('renders an invalid response error when the response payload is malformed', async () => {
    const user = userEvent.setup();
    requestMock.mockResolvedValueOnce({ ok: false, code: 'unexpected_code' });

    renderDialog();

    await user.type(
      screen.getByRole('textbox', { name: /repository url/i }),
      'https://github.com/acme/api.git',
    );
    await user.click(screen.getByRole('button', { name: /add repository/i }));

    expect(
      await screen.findByText('Add repository returned an invalid response.'),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /add repository/i }),
    ).toBeEnabled();
    expect(closeMock).not.toHaveBeenCalled();
  });
});
