import { describe, it, expect } from 'vitest';
import { markdownSchema } from '../../src/renderer/utils/markdownSchema';

describe('markdownSchema', () => {
  describe('details/summary support', () => {
    it('allows details tag', () => {
      expect(markdownSchema.tagNames).toContain('details');
    });

    it('allows summary tag', () => {
      expect(markdownSchema.tagNames).toContain('summary');
    });

    it('allows open attribute on details', () => {
      const detailsAttrs = markdownSchema.attributes?.['details'] ?? [];
      const hasOpen = detailsAttrs.some((attr) =>
        Array.isArray(attr) ? attr[0] === 'open' : attr === 'open'
      );
      expect(hasOpen).toBe(true);
    });
  });

  describe('script stripping', () => {
    it('strips script tags (not in tagNames)', () => {
      const tagNames = markdownSchema.tagNames ?? [];
      expect(tagNames).not.toContain('script');
    });

    it('has script in strip list', () => {
      expect(markdownSchema.strip).toContain('script');
    });
  });

  describe('href protocol narrowing', () => {
    it('allows https in href protocols', () => {
      expect(markdownSchema.protocols?.href).toContain('https');
    });

    it('does not allow http in href protocols', () => {
      expect(markdownSchema.protocols?.href).not.toContain('http');
    });

    it('does not allow mailto in href protocols', () => {
      expect(markdownSchema.protocols?.href).not.toContain('mailto');
    });

    it('href protocols contains only https', () => {
      expect(markdownSchema.protocols?.href).toEqual(['https']);
    });
  });

  describe('image surfaces removed', () => {
    it('does not allow img tag', () => {
      expect(markdownSchema.tagNames).not.toContain('img');
    });

    it('does not allow picture tag', () => {
      expect(markdownSchema.tagNames).not.toContain('picture');
    });

    it('does not allow source tag', () => {
      expect(markdownSchema.tagNames).not.toContain('source');
    });

    it('does not have img attributes defined', () => {
      expect(markdownSchema.attributes).not.toHaveProperty('img');
    });

    it('does not allow src protocol entries', () => {
      expect(markdownSchema.protocols?.src).toBeUndefined();
    });

    it('does not allow longDesc protocol entries', () => {
      expect(markdownSchema.protocols?.longDesc).toBeUndefined();
    });
  });
});
