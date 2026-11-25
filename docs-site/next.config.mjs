import { createMDX } from 'fumadocs-mdx/next';
import { basePath } from './lib/base-path.mjs';

/** @type {import('next').NextConfig} */
const config = {
  reactStrictMode: true,
  output: 'export',
  basePath,
  trailingSlash: true,
  images: { unoptimized: true },
  // `next dev` would otherwise write an AGENTS.md into docs-site/.
  agentRules: false,
};

const withMDX = createMDX();

export default withMDX(config);
