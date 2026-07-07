import { listenBaseUrl } from './listen-url';

const VERIFY_PROMPT = '[kotro-verify] What is a semantic cache in one sentence?';

type CacheLabel = 'hit' | 'miss' | 'bypass' | 'error';

export type VerifyResult = {
  first: CacheLabel;
  second: CacheLabel;
  ok: boolean;
  detail: string;
};

async function postOnce(url: string): Promise<CacheLabel> {
  try {
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        model: 'gpt-4o',
        stream: true,
        messages: [{ role: 'user', content: VERIFY_PROMPT }],
      }),
    });
    if (!res.ok) {
      const body = await res.text().catch(() => '');
      throw new Error(`HTTP ${res.status}${body ? `: ${body.slice(0, 120)}` : ''}`);
    }

    const cache = res.headers.get('x-kotro-cache')?.toLowerCase();
    await res.text();

    if (cache === 'hit') {
      return 'hit';
    }
    if (cache === 'bypass') {
      return 'bypass';
    }
    return 'miss';
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    throw new Error(msg);
  }
}

export async function verifyCache(listenAddr: string): Promise<VerifyResult> {
  const url = `${listenBaseUrl(listenAddr)}/v1/chat/completions`;

  let first: CacheLabel;
  try {
    first = await postOnce(url);
  } catch (err) {
    const detail = err instanceof Error ? err.message : String(err);
    return {
      first: 'error',
      second: 'error',
      ok: false,
      detail: `Request 1 failed: ${detail}`,
    };
  }

  let second: CacheLabel;
  try {
    second = await postOnce(url);
  } catch (err) {
    const detail = err instanceof Error ? err.message : String(err);
    return {
      first,
      second: 'error',
      ok: false,
      detail: `Request 1 was ${first.toUpperCase()}, but request 2 failed: ${detail}`,
    };
  }

  if (first === 'bypass' || second === 'bypass') {
    return {
      first,
      second,
      ok: false,
      detail:
        'Cache is bypassed (check kotrolabs.enableCache and use stream:true). ' +
        `Got ${first.toUpperCase()} then ${second.toUpperCase()}.`,
    };
  }

  if (second === 'hit') {
    return {
      first,
      second,
      ok: true,
      detail: `Cache works: ${first.toUpperCase()} on first request, HIT on second.`,
    };
  }

  return {
    first,
    second,
    ok: false,
    detail: `Expected HIT on second request, got ${second.toUpperCase()} (first was ${first.toUpperCase()}).`,
  };
}
