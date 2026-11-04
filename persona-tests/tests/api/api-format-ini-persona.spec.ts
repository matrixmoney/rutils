import { test, expect } from '@playwright/test';

// ================================================================
// Backend API — Formatter Persona (INI)
//
// This spec exercises the new POST /format endpoint for the INI
// formatter. It has four groups:
//
//   Group 1: Happy path — messy input becomes canonical.
//   Group 2: Fixed-point — canonical input round-trips unchanged.
//   Group 3: The "never 4xx on broken input" contract.
//   Group 4: Real 4xx cases (missing file, non-INI garbage, GET).
//
// All fixtures are inline strings; no filesystem dependency.
// ================================================================

const API_URL = 'http://localhost:8080';

// ================================================================
// GROUP 1: Happy path — formatting
// ================================================================

test.describe('Backend API Formatter — INI Persona', () => {

  test('Persona: messy INI comes back formatted', async ({ request }) => {
    await test.step('Why we use this test: the core use case — a file with inconsistent spacing and colon separators should come back canonical.', async () => {});

    const messy = `[server]
host:localhost
port : 8080


[database]
name=mydb
user : admin
`;

    let response;
    await test.step('What we use: POST /format with the messy INI and describe=true.', async () => {
      response = await request.post(`${API_URL}/format`, {
        multipart: {
          file: {
            name: 'config.ini',
            mimeType: 'text/plain',
            buffer: Buffer.from(messy),
          },
          describe: 'true',
        },
      });
    });

    await test.step('What we expected: 200 OK.', async () => {
      expect(response.status()).toBe(200);
    });

    await test.step('What we get: format=ini, changed=true, and every key uses the canonical `key = value` form.', async () => {
      const json = await response.json();
      expect(json.format).toBe('ini');
      expect(json.changed).toBe(true);
      expect(Array.isArray(json.applied)).toBe(true);

      const out = json.output as string;
      expect(out).toContain('host = localhost');
      expect(out).toContain('port = 8080');
      expect(out).toContain('name = mydb');
      expect(out).toContain('user = admin');
    });
  });

  test('Persona: canonical INI comes back unchanged (Changed=false)', async ({ request }) => {
    await test.step('Why we use this test: if the file is already formatted, we do not touch it — Changed=false and byte-identical output.', async () => {});

    const canonical = `[server]
host = localhost
port = 8080

[database]
name = mydb
`;

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'config.ini',
          mimeType: 'text/plain',
          buffer: Buffer.from(canonical),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.format).toBe('ini');
    expect(json.changed).toBe(false);
    expect(json.output).toBe(canonical);
  });

  test('Persona: `key : value` becomes `key = value`', async ({ request }) => {
    await test.step('Why we use this test: colon separators are common in INI files written by hand; they must be canonicalised to `=`.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'config.ini',
          mimeType: 'text/plain',
          buffer: Buffer.from('[s]\nkey : value\n'),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.changed).toBe(true);
    expect(json.output).toContain('key = value');
    expect(json.output).not.toContain('key : value');
  });

  test('Persona: comments preserved (`;` and `#`)', async ({ request }) => {
    await test.step('Why we use this test: the two INI comment styles are user preference, not a formatting choice — we must not convert between them.', async () => {});

    const input = `; semicolon comment
# hash comment
[s]
key = value
`;

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'config.ini',
          mimeType: 'text/plain',
          buffer: Buffer.from(input),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.output).toContain('; semicolon comment');
    expect(json.output).toContain('# hash comment');
  });

  test('Persona: runs of blank lines collapse to a single blank', async ({ request }) => {
    await test.step('Why we use this test: multiple consecutive blank lines are noise; one blank line is enough separation.', async () => {});

    const input = `[a]
key = value



[b]
key2 = value2
`;

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'config.ini',
          mimeType: 'text/plain',
          buffer: Buffer.from(input),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.changed).toBe(true);
    // The output must NOT contain two or more consecutive blank lines.
    expect(json.output).not.toMatch(/\n\n\n/);
    // But it should still contain a single blank line between sections.
    expect(json.output).toContain('\n\n[b]');
  });

  test('Persona: key and section order preserved', async ({ request }) => {
    await test.step('Why we use this test: INI sometimes depends on ordering (e.g. override files). We must never sort.', async () => {});

    const input = `[zebra]
zzz = 1
aaa = 2

[apple]
yyy = 3
bbb = 4
`;

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'config.ini',
          mimeType: 'text/plain',
          buffer: Buffer.from(input),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    const out = json.output as string;

    // Sections appear in the original order.
    const zebraIdx = out.indexOf('[zebra]');
    const appleIdx = out.indexOf('[apple]');
    expect(zebraIdx).toBeGreaterThan(-1);
    expect(appleIdx).toBeGreaterThan(zebraIdx);

    // Keys appear in the original order within their section.
    const zzzIdx = out.indexOf('zzz = 1');
    const aaaIdx = out.indexOf('aaa = 2');
    expect(zzzIdx).toBeGreaterThan(-1);
    expect(aaaIdx).toBeGreaterThan(zzzIdx);
  });

  // ================================================================
  // GROUP 2: The "never 4xx on broken input" contract
  // ================================================================

  test('Persona: garbage line preserved with warning, still 200', async ({ request }) => {
    await test.step('Why we use this test: this is the whole point of permissive formatting. A line that cannot be parsed must NOT cause a 4xx — the user gets it back with a warning.', async () => {});

    const broken = `[s]
key = value
this is not INI syntax
another = key
`;

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'broken.ini',
          mimeType: 'text/plain',
          buffer: Buffer.from(broken),
        },
        describe: 'true',
      },
    });

    await test.step('What we expected: 200 OK — never 400.', async () => {
      expect(response.status()).toBe(200);
    });

    await test.step('What we get: changed=true, non-empty output, at least one warning, and the garbage line is preserved.', async () => {
      const json = await response.json();
      expect(json.changed).toBe(true);
      expect(typeof json.output).toBe('string');
      expect(json.output.length).toBeGreaterThan(0);
      expect(Array.isArray(json.warnings)).toBe(true);
      expect(json.warnings.length).toBeGreaterThan(0);
      expect(json.output).toContain('this is not INI syntax');
    });
  });

  test('Persona: BOM-prefixed INI works', async ({ request }) => {
    await test.step('Why we use this test: many Windows editors save INI files with a UTF-8 BOM. The formatter should strip it (recorded as applied) and continue.', async () => {});

    const bom = '\uFEFF[server]\nhost = localhost\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'bom.ini',
          mimeType: 'text/plain',
          buffer: Buffer.from(bom, 'utf8'),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.applied).toContain('bom-stripped');
    expect(json.output.startsWith('\uFEFF')).toBe(false);
    expect(json.output).toContain('[server]');
  });

  // ================================================================
  // GROUP 3: Headers path
  // ================================================================

  test('Persona: /format emits the X-Format-* headers for INI', async ({ request }) => {
    await test.step('Why we use this test: the raw-bytes path (no describe) relies on the headers to convey the report.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'config.ini',
          mimeType: 'text/plain',
          buffer: Buffer.from('[s]\nkey : value\n'),
        },
        // describe intentionally omitted → raw bytes response
      },
    });

    expect(response.status()).toBe(200);

    const headers = response.headers();
    expect(headers['x-format-format']).toBe('ini');
    expect(headers['x-format-changed']).toBe('true');
    expect(headers['x-format-applied']).toContain('ini;changed=true');
    expect(headers['access-control-expose-headers']).toContain('X-Format-Applied');
  });

  // ================================================================
  // GROUP 4: Real 4xx cases (only the ones the spec allows)
  // ================================================================

  test('Persona: missing file field is rejected with 400', async ({ request }) => {
    await test.step('Why we use this test: a request with no file cannot be formatted — this is one of the only two 4xx cases.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        describe: 'true',
      },
    });

    expect(response.status()).toBe(400);
    expect(await response.text()).toContain('file is required');
  });

  test('Persona: non-INI garbage is rejected with 415', async ({ request }) => {
    await test.step('Why we use this test: the formatter cannot identify the format. If the extension is not .ini and the content does not look like INI, respond 415.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'garbage.bin',
          mimeType: 'application/octet-stream',
          buffer: Buffer.from('this is not ini and not any recognised format'),
        },
      },
    });

    expect(response.status()).toBe(415);
    expect((await response.text()).toLowerCase()).toContain('not supported');
  });

  test('Persona: wrong HTTP method is rejected with 405', async ({ request }) => {
    await test.step('Why we use this test: /format is POST-only.', async () => {});

    const response = await request.get(`${API_URL}/format`);
    expect(response.status()).toBe(405);
    expect(await response.text()).toContain('Only POST method is allowed');
  });
});