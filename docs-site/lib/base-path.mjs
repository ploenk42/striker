// The path the site is served under on GitHub Pages (https://<user>.github.io/striker/).
// Plain JavaScript so next.config.mjs can import it too.
// Next adds it to <Link> and its own assets; anything else (the search index URL, a meta refresh,
// image paths in MDX) has to put it in front itself.

/** @type {string} */
export const basePath = '/striker';

/**
 * A site path with the base path in front, e.g. withBasePath('/shots/plan.png').
 * @param {string} path a path starting with '/'
 * @returns {string}
 */
export function withBasePath(path) {
  return `${basePath}${path}`;
}
