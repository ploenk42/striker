import './global.css';
import { RootProvider } from 'fumadocs-ui/provider/next';
import type { Metadata } from 'next';
import type { ReactNode } from 'react';
import { withBasePath } from '@/lib/base-path.mjs';

export const metadata: Metadata = {
  title: {
    template: '%s | Striker',
    default: 'Striker',
  },
  description: 'Striker: a fleet of Claude Code agents across repos and machines, run from Herdr.',
};

export default function Layout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className="flex flex-col min-h-screen">
        <RootProvider
          search={{
            // Static export: the browser downloads the index exported by app/api/search.
            // basePath is not added for us, so the path includes it.
            options: { type: 'static', api: withBasePath('/api/search') },
          }}
        >
          {children}
        </RootProvider>
      </body>
    </html>
  );
}
