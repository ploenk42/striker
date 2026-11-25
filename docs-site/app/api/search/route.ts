import { createFromSource } from 'fumadocs-core/search/server';
import { source } from '@/lib/source';

// Static export: the index is written at build time and searched in the browser.
export const revalidate = false;

export const { staticGET: GET } = createFromSource(source, {
  language: 'english',
});
