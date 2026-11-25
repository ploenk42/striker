'use client';

import { useTheme } from 'next-themes';
import { useEffect, useId, useState } from 'react';

/**
 * Renders a Mermaid diagram in the browser and follows the site's light/dark theme.
 * If the chart cannot be rendered, its source is shown instead.
 * With `wide`, the diagram keeps its natural size and the container scrolls horizontally,
 * so large diagrams stay readable instead of being shrunk to the content width.
 */
export function Mermaid({ chart, wide = false }: { chart: string; wide?: boolean }) {
  const id = useId().replace(/[^a-zA-Z0-9]/g, '');
  const { resolvedTheme } = useTheme();
  const [svg, setSvg] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let cancelled = false;

    async function render() {
      try {
        const { default: mermaid } = await import('mermaid');
        mermaid.initialize({
          startOnLoad: false,
          securityLevel: 'strict',
          theme: resolvedTheme === 'dark' ? 'dark' : 'default',
          flowchart: { useMaxWidth: !wide },
          sequence: { useMaxWidth: !wide },
        });
        const { svg: out } = await mermaid.render(`mermaid-${id}-${resolvedTheme ?? 'light'}`, chart);
        if (!cancelled) {
          setSvg(out);
          setFailed(false);
        }
      } catch (error) {
        console.error('Mermaid render failed', error);
        if (!cancelled) {
          setSvg(null);
          setFailed(true);
        }
      }
    }

    void render();
    return () => {
      cancelled = true;
    };
  }, [chart, id, resolvedTheme, wide]);

  if (failed) {
    return (
      <pre>
        <code>{chart}</code>
      </pre>
    );
  }

  if (!svg) return <div className="my-4 min-h-24" aria-busy="true" />;

  if (wide) {
    return (
      <div className="my-4 overflow-x-auto">
        <div className="mx-auto w-max" dangerouslySetInnerHTML={{ __html: svg }} />
      </div>
    );
  }

  return <div className="my-4 flex justify-center [&_svg]:max-w-full" dangerouslySetInnerHTML={{ __html: svg }} />;
}
