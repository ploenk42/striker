import { remarkStructureDefaultOptions } from 'fumadocs-core/mdx-plugins';
import { defineConfig, defineDocs } from 'fumadocs-mdx/config';

export const docs = defineDocs({
  dir: 'content/docs',
});

export default defineConfig({
  mdxOptions: {
    remarkStructureOptions: {
      // Keep diagram sources out of the search index: a <Mermaid chart="…" /> element would
      // otherwise be indexed as its raw source text.
      types: (node) =>
        remarkStructureDefaultOptions.types.includes(node.type) &&
        !(node.type === 'mdxJsxFlowElement' && node.name === 'Mermaid'),
    },
  },
});
