import Link from 'next/link';
import { withBasePath } from '@/lib/base-path.mjs';

// The static export has no server to redirect, so the start page forwards with a meta refresh
// (works without JavaScript) and shows a link in case the browser does not follow it.
export default function Home() {
  return (
    <main className="flex flex-1 items-center justify-center p-8">
      <meta httpEquiv="refresh" content={`0; url=${withBasePath('/docs/')}`} />
      <p>
        Go to the <Link href="/docs/" className="underline">Striker documentation</Link>.
      </p>
    </main>
  );
}
