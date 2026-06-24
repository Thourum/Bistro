/**
 * rehype-sanitize schema for Markdown rendering.
 *
 * Rules:
 * - Only https:// href protocols allowed (matches ExternalLink policy)
 * - img / picture / source tags and their attributes are removed entirely
 *   (images are blocked; the img component override renders null as defence-in-depth)
 * - script is in the strip list so it is removed before sanitization
 * - details + summary are allowed; details.open is the only extra attribute
 */

import type { Schema } from 'hast-util-sanitize';
import { defaultSchema } from 'rehype-sanitize';

// Build a clean copy of the default attributes without any image-related keys.
const { img: _img, ...attributesWithoutImg } =
  defaultSchema.attributes as Record<string, unknown>;

// Protocols: keep only https for hrefs; remove src/longDesc entirely.
const {
  src: _src,
  longDesc: _longDesc,
  href: _href,
  ...otherProtocols
} = (defaultSchema.protocols ?? {}) as Record<string, unknown>;

// Tag names: exclude img / picture / source.
const cleanTagNames = (defaultSchema.tagNames ?? []).filter(
  (t) => t !== 'img' && t !== 'picture' && t !== 'source',
);

export const markdownSchema: Schema = {
  ...defaultSchema,
  tagNames: [...cleanTagNames, 'details', 'summary'],
  strip: [...(defaultSchema.strip ?? []), 'script'],
  attributes: {
    ...attributesWithoutImg,
    details: ['open'],
  },
  protocols: {
    ...otherProtocols,
    href: ['https'],
  },
};
