// @vitest-environment jsdom
import { render, screen } from '@testing-library/react';
import { expect } from 'vitest';
import * as matchers from '@testing-library/jest-dom/matchers';
import { AuthLayout } from '../../src/renderer/layouts/AuthLayout';

expect.extend(matchers);
import { MainLayout } from '../../src/renderer/layouts/MainLayout';
import { SettingsLayout } from '../../src/renderer/layouts/SettingsLayout';

// ---------------------------------------------------------------------------
// AuthLayout
// ---------------------------------------------------------------------------

describe('AuthLayout', () => {
  it('renders children centered on screen', () => {
    render(
      <AuthLayout>
        <div data-testid="child">Test Content</div>
      </AuthLayout>
    );
    expect(screen.getByTestId('child')).toBeInTheDocument();
  });

  it('uses correct background color', () => {
    const { container } = render(
      <AuthLayout>
        <div>Test</div>
      </AuthLayout>
    );
    const layoutDiv = container.querySelector('[data-testid="auth-layout"]');
    expect(layoutDiv).toHaveClass('bg-surface');
  });

  it('renders card with correct styling', () => {
    const { container } = render(
      <AuthLayout>
        <div>Test</div>
      </AuthLayout>
    );
    const card = container.querySelector('[data-testid="auth-card"]');
    expect(card).toHaveClass('bg-surface-container-lowest');
    expect(card).toHaveClass('border');
  });
});

// ---------------------------------------------------------------------------
// MainLayout
// ---------------------------------------------------------------------------

describe('MainLayout', () => {
  it('renders children in content area', () => {
    render(
      <MainLayout>
        <div data-testid="main-content">Content</div>
      </MainLayout>
    );
    expect(screen.getByTestId('main-content')).toBeInTheDocument();
  });

  it('renders toolbar in header when provided', () => {
    render(
      <MainLayout toolbar={<button>Test Button</button>}>
        <div>Content</div>
      </MainLayout>
    );
    expect(screen.getByRole('button', { name: /Test Button/i })).toBeInTheDocument();
  });

  it('uses correct background color', () => {
    const { container } = render(
      <MainLayout>
        <div>Content</div>
      </MainLayout>
    );
    const layout = container.querySelector('[data-testid="main-layout"]');
    expect(layout).toHaveClass('bg-surface-container-lowest');
  });
});

// ---------------------------------------------------------------------------
// SettingsLayout
// ---------------------------------------------------------------------------

describe('SettingsLayout', () => {
  it('renders title and description', () => {
    render(
      <SettingsLayout title="Settings" description="Configure Bistro">
        <div>Content</div>
      </SettingsLayout>
    );
    expect(screen.getByText('Settings')).toBeInTheDocument();
    expect(screen.getByText('Configure Bistro')).toBeInTheDocument();
  });

  it('renders children below header', () => {
    render(
      <SettingsLayout title="Test" description="Desc">
        <div data-testid="settings-content">Content</div>
      </SettingsLayout>
    );
    expect(screen.getByTestId('settings-content')).toBeInTheDocument();
  });

  it('uses correct styling on header', () => {
    const { container } = render(
      <SettingsLayout title="Test" description="Desc">
        <div>Content</div>
      </SettingsLayout>
    );
    const header = container.querySelector('[data-testid="settings-header"]');
    expect(header).toHaveClass('border-b');
  });
});
