// @vitest-environment jsdom
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect } from 'vitest';
import * as matchers from '@testing-library/jest-dom/matchers';
import { ProviderButton } from '../../src/renderer/components/ProviderButton';

expect.extend(matchers);

describe('ProviderButton', () => {
  it('renders provider label', () => {
    render(
      <ProviderButton
        provider="github"
        icon="terminal"
        label="GitHub"
        keyHint="G"
        onClick={() => {}}
      />
    );
    expect(screen.getByText('GitHub')).toBeInTheDocument();
  });

  it('renders keyboard hint', () => {
    render(
      <ProviderButton
        provider="github"
        icon="terminal"
        label="GitHub"
        keyHint="G"
        onClick={() => {}}
      />
    );
    expect(screen.getByText('G')).toBeInTheDocument();
  });

  it('calls onClick when clicked', async () => {
    const handleClick = vi.fn();
    render(
      <ProviderButton
        provider="github"
        icon="terminal"
        label="GitHub"
        keyHint="G"
        onClick={handleClick}
      />
    );
    await userEvent.click(screen.getByRole('button'));
    expect(handleClick).toHaveBeenCalled();
  });

  it('has correct styling for dark outline', () => {
    const { container } = render(
      <ProviderButton
        provider="github"
        icon="terminal"
        label="GitHub"
        keyHint="G"
        onClick={() => {}}
      />
    );
    const button = container.querySelector('button');
    expect(button).toHaveClass('h-14');
    expect(button).toHaveClass('border');
  });
});
