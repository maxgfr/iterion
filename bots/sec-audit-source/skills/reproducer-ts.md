---
name: reproducer-ts
description: |
  How to write a focused TS/JS regression test per finding_type —
  one that FAILS on unpatched code and PASSES after the fix.
  Loaded by `[[patch]]`'s author phase when language=ts|js and the
  project has a `vitest` / `jest` / `mocha` suite. HTTP cases use
  `supertest`.
---

# reproducer-ts — failing-then-passing tests per finding_type

Each recipe writes ONE test that:

- Fails on **unpatched** code.
- Passes after the fix.
- Lives next to the unit under test (`*.test.ts`, `*.spec.ts`, or
  `__tests__/`).
- Runs in isolation: a single `vitest run -t '<name>'` or
  `jest -t '<name>'` reproduces it.

Use the framework already in the project (`package.json` →
`scripts.test`). Don't introduce new deps. If `supertest` isn't
present, hand-roll with `node:http` + `fetch`.

## General shape

```ts
import { describe, it, expect } from 'vitest'; // or 'jest'

describe('reproduce <finding_type>: <short>', () => {
  it('rejects the malicious input', async () => {
    // arrange: minimal fixture
    // act:     hit the sink with attacker input
    // assert:  the bad behavior must NOT happen
  });
});
```

Avoid sleeps, real network calls, real DBs. Use `vi.mock` / `jest.mock`,
`tmpdir()`, and `supertest(app)`.

## Recipes per finding_type

### injection (SQL / NoSQL / command / template)

Use a query mocker (`vitest-mock-extended`, `prisma-mock`, or a
plain spy). Assertion: the SQL string MUST be parameterized; the
attacker payload appears only in the args array.

```ts
it('parameterizes order lookup', async () => {
  const exec = vi.fn().mockResolvedValue([{ id: 1 }]);
  const db = { query: exec };
  await lookupOrder(db, "1 OR 1=1 --");
  expect(exec).toHaveBeenCalledWith(
    'SELECT * FROM orders WHERE id = $1',
    ["1 OR 1=1 --"],
  );
});
```

For shell injection: spy on `execFile` / `spawn` and assert argv
items, NOT a `exec("sh -c ...")` string.

### xss

Use `supertest` + a JSDOM-friendly assertion: response body must
HTML-escape the payload.

```ts
import request from 'supertest';
import { app } from '../src/app';

it('escapes user-controlled name', async () => {
  const res = await request(app).get('/u/42');
  expect(res.status).toBe(200);
  expect(res.text).not.toContain('<script>');
  expect(res.text).toContain('&lt;script&gt;');
});
```

For React: assert that `dangerouslySetInnerHTML` is absent from
the rendered tree (parse with `@testing-library/react` + a regex
on outerHTML).

### ssrf

Mock the outbound HTTP layer with `nock` or a fetch stub. Assertion:
the call to internal IPs MUST be rejected by the allowlist before
the network layer is touched.

```ts
it('blocks internal targets', async () => {
  const fetchSpy = vi.fn();
  vi.stubGlobal('fetch', fetchSpy);
  await expect(proxyFetch('http://169.254.169.254/latest/meta-data'))
    .rejects.toThrow(/ssrf/i);
  expect(fetchSpy).not.toHaveBeenCalled();
});
```

### auth

Hit a protected route without a token; assert 401.

```ts
it('requires bearer token on admin routes', async () => {
  const res = await request(app).get('/admin/users');
  expect(res.status).toBe(401);
});
```

For JWT: assert that a token with `none` alg is rejected, and a
token signed with the wrong secret is rejected.

### authz / idor

Two tenants, one route, two tokens.

```ts
it('tenant A cannot read tenant B orders', async () => {
  await seedTenants(db);
  const res = await request(app)
    .get('/orders/B-1')
    .set('Authorization', `Bearer ${tokenFor('A')}`);
  expect(res.status).not.toBe(200);
});
```

### crypto — note → hard-stop

Crypto findings do NOT get auto-patched (see `[[crypto-handling]]`).
For humans writing the fix, test shapes:

- Constant-time compare: assert source uses `crypto.timingSafeEqual`
  via AST grep (`ts-morph`), not via timing.
- RNG: assert `crypto.randomBytes` / `crypto.getRandomValues`, not
  `Math.random()`.
- TLS: assert `minVersion: 'TLSv1.2'` (Node `tls.createServer`
  options).

Do NOT land an auto-generated crypto fix.

### secrets

Same caveat as Go — primarily a CI-time `gitleaks` exit-code
assertion. If you write a unit test:

```ts
it('build artifact has no AWS key prefix', () => {
  const bin = fs.readFileSync('./dist/server.js', 'utf8');
  expect(bin).not.toMatch(/AKIA[0-9A-Z]{16}/);
});
```

### deserialization

For `JSON.parse` of user input fed to a schema-less object: assert
the parsed object goes through a validator (`zod`, `valibot`,
`class-validator`) BEFORE reaching the sink.

```ts
it('rejects prototype-pollution payload', () => {
  const payload = '{"__proto__":{"admin":true}}';
  const parsed = parseOrder(payload); // your validator
  expect((parsed as any).admin).toBeUndefined();
  // and: a fresh object isn't polluted
  expect(({} as any).admin).toBeUndefined();
});
```

For `yaml.load`: assert `yaml.parse` (safe) is used, not
`yaml.load` with the default schema.

### path-trav

Use `os.tmpdir()` + `path.join`. Assertion: the helper rejects
escapes.

```ts
it('rejects ../ in download path', async () => {
  const base = await mkdtemp(path.join(os.tmpdir(), 'rep-'));
  await expect(safeOpen(base, '../../etc/passwd'))
    .rejects.toThrow(/escape/i);
});
```

### redirect

`supertest` recorder; assert the `Location` header is on the
allowlist.

```ts
it('does not honor off-domain next=', async () => {
  const res = await request(app).get('/login?next=https://evil.example/');
  const loc = res.headers.location ?? '';
  expect(loc.startsWith('https://evil.example')).toBe(false);
});
```

### config

A unit test on the parsed config:

```ts
it('TLS minVersion >= TLSv1.2', () => {
  const cfg = loadServerTLSConfig();
  expect(cfg.minVersion).toBe('TLSv1.2');
});
```

For Next.js: assert `next.config.js` middleware matches the
expected matcher pattern.

### other

Pick the closest pattern. The bar is still "fails-on-unpatched,
passes-after".

## Reproducer hygiene

- One `it()` per finding. Don't bundle.
- Name `reproduce <finding_type>: <short>` so `vitest -t` /
  `jest -t` targets it cleanly.
- Don't import production code's CLI entry; test the route, the
  handler, or the validator directly.
- Use `vi.useFakeTimers()` / `jest.useFakeTimers()` for any
  time-sensitive code; never `setTimeout` waits.
- TS strict on: types in the test file must match the production
  types — no `as any` to silence the compiler around the sink.

## See also

- `[[patch]]` — invokes this skill in step 6 of the author brief.
- `[[reproducer-go]]` — same recipes for Go.
- `[[crypto-handling]]` — why crypto findings don't get
  auto-patched.
- `[[finding-taxonomy]]` — the twelve categories.
