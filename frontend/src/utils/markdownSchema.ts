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

import type { Options as RehypeSanitizeOptions } from 'rehype-sanitize';
import { defaultSchema } from 'rehype-sanitize';

type MarkdownSchema = NonNullable<RehypeSanitizeOptions>;
// Remove image-related fields without changing the default schema.
const attributesWithoutImg = Object.fromEntries(
  Object.entries(defaultSchema.attributes ?? {}).filter(
    ([tag]) => tag !== 'img',
  ),
);
const otherProtocols = Object.fromEntries(
  Object.entries(defaultSchema.protocols ?? {}).filter(
    ([attribute]) => !['src', 'longDesc', 'href'].includes(attribute),
  ),
);

// Tag names: exclude img / picture / source.
const cleanTagNames = (defaultSchema.tagNames ?? []).filter(
  (t) => t !== 'img' && t !== 'picture' && t !== 'source',
);

export const markdownSchema: MarkdownSchema = {
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
