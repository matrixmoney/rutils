import { test, expect } from '@playwright/test';

// ================================================================
// Backend API вҖ” Formatter Persona (JSON)
//
// Four groups:
//   Group 1: Happy path вҖ” messy input becomes canonical.
//   Group 2: Fixed-point вҖ” canonical input round-trips unchanged.
//   Group 3: The "never 4xx on broken input" contract.
//   Group 4: Real 4xx cases.
// ================================================================

const API_URL = 'http://localhost:8080';

test.describe('Backend API Formatter вҖ” JSON Persona', () => {

  // ================================================================
  // GROUP 1: Happy path вҖ” formatting
  // ================================================================

  test('Persona: messy JSON comes back formatted', async ({ request }) => {
    await test.step('Why we use this test: the core use case вҖ” a single-line JSON object should come back indented.', async () => {});

    const messy = '{"name":"Alice","age":30,"tags":["a","b"]}';

    let response;
    await test.step('What we use: POST /format with the messy JSON and describe=true.', async () => {
      response = await request.post(`${API_URL}/format`, {
        multipart: {
          file: { name: 'data.json', mimeType: 'application/json', buffer: Buffer.from(messy) },
          describe: 'true',
        },
      });
    });

    await test.step('What we expected: 200 OK.', async () => {
      expect(response.status()).toBe(200);
    });

    await test.step('What we get: format=json, changed=true, indented output.', async () => {
      const json = await response.json();
      expect(json.format).toBe('json');
      expect(json.changed).toBe(true);
      const out = json.output as string;
      expect(out).toContain('\n  "name"');
      expect(out.startsWith('{')).toBe(true);
    });
  });

  test('Persona: canonical JSON comes back unchanged', async ({ request }) => {
    await test.step('Why we use this test: if the file is already formatted, Changed=false and output is byte-identical.', async () => {});

    const canonical = '{\n  "name": "Alice",\n  "age": 30\n}\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'data.json', mimeType: 'application/json', buffer: Buffer.from(canonical) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.format).toBe('json');
    expect(json.changed).toBe(false);
    expect(json.output).toBe(canonical);
  });

  test('Persona: key order preserved', async ({ request }) => {
    await test.step('Why we use this test: json.Indent preserves key order вҖ” alphabetical sorting would lose the user\'s intent.', async () => {});

    const input = '{"z":1,"a":2,"m":3}';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'data.json', mimeType: 'application/json', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    const out = json.output as string;
    const zIdx = out.indexOf('"z"');
    const aIdx = out.indexOf('"a"');
    const mIdx = out.indexOf('"m"');
    expect(zIdx).toBeGreaterThan(-1);
    expect(aIdx).toBeGreaterThan(zIdx);
    expect(mIdx).toBeGreaterThan(aIdx);
  });

  test('Persona: number formatting preserved (1.0, 1e10)', async ({ request }) => {
    await test.step('Why we use this test: json.Indent preserves number formatting byte-for-byte вҖ” 1.0 stays 1.0, 1e10 stays 1e10.', async () => {});

    const input = '{"x":1.0,"y":1e10}';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'data.json', mimeType: 'application/json', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.output).toContain('1.0');
    expect(json.output).toContain('1e10');
  });

  test('Persona: string escaping preserved', async ({ request }) => {
    await test.step('Why we use this test: json.Indent preserves escape sequences byte-for-byte вҖ” \\u00e9 stays \\u00e9.', async () => {});

    const input = '{"s":"caf\\u00e9"}';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'data.json', mimeType: 'application/json', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.output).toContain('\\u00e9');
  });

  test('Persona: BOM-prefixed JSON works', async ({ request }) => {
    await test.step('Why we use this test: Windows editors save JSON with a UTF-8 BOM. The formatter should strip it.', async () => {});

    const bom = '\uFEFF{"a":1}';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'bom.json', mimeType: 'application/json', buffer: Buffer.from(bom, 'utf8') },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.applied).toContain('bom-stripped');
    expect(json.output.startsWith('\uFEFF')).toBe(false);
  });

  // ================================================================
  // GROUP 2: Best-effort fixes
  // ================================================================

  test('Persona: trailing comma stripped', async ({ request }) => {
    await test.step('Why we use this test: trailing commas are the most common JSON defect.', async () => {});

    const input = '{"name":"Alice","age":30,}';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'broken.json', mimeType: 'application/json', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.changed).toBe(true);
    expect(json.applied).toContain('trailing-comma');
    // Output must be valid JSON.
    expect(() => JSON.parse(json.output)).not.toThrow();
  });

  test('Persona: comments stripped', async ({ request }) => {
    await test.step('Why we use this test: JS-style comments are common in JSON files edited by hand.', async () => {});

    const input = '{\n  // leading comment\n  "name": "Alice",\n  "age": 30\n}';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'broken.json', mimeType: 'application/json', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.applied).toContain('comments-stripped');
    expect(() => JSON.parse(json.output)).not.toThrow();
  });

  test('Persona: single quotes converted', async ({ request }) => {
    await test.step('Why we use this test: JS developers frequently write single-quoted JSON.', async () => {});

    const input = "{'name':'Alice','age':30}";

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'broken.json', mimeType: 'application/json', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.applied).toContain('single-quotes');
    expect(() => JSON.parse(json.output)).not.toThrow();
  });

  test('Persona: unquoted keys converted', async ({ request }) => {
    await test.step('Why we use this test: `{name: "Alice"}` is common in JS but invalid JSON.', async () => {});

    const input = '{name:"Alice",age:30}';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'broken.json', mimeType: 'application/json', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.applied).toContain('unquoted-keys');
    expect(() => JSON.parse(json.output)).not.toThrow();
  });

  test('Persona: completely broken JSON returns 200 with warning', async ({ request }) => {
    await test.step('Why we use this test: even unparseable content must not surface as a 4xx. The user gets best-effort output plus a warning.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'broken.json', mimeType: 'application/json', buffer: Buffer.from('this is definitely not json at all\nand neither is this\n') },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.changed).toBe(true);
    expect(typeof json.output).toBe('string');
    expect(Array.isArray(json.warnings)).toBe(true);
    expect(json.warnings.length).toBeGreaterThan(0);
  });

  // ================================================================
  // GROUP 3: Headers path
  // ================================================================

  test('Persona: /format emits X-Format-* headers for JSON', async ({ request }) => {
    await test.step('Why we use this test: the raw-bytes path (no describe) uses the headers to convey the report.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'data.json', mimeType: 'application/json', buffer: Buffer.from('{"a":1}') },
      },
    });

    expect(response.status()).toBe(200);
    const headers = response.headers();
    expect(headers['x-format-format']).toBe('json');
    expect(headers['x-format-changed']).toBe('true');
    expect(headers['x-format-applied']).toContain('json;changed=true');
    expect(headers['access-control-expose-headers']).toContain('X-Format-Applied');
  });

  // ================================================================
  // GROUP 4: Real 4xx cases
  // ================================================================

  test('Persona: missing file field is rejected with 400', async ({ request }) => {
    await test.step('Why we use this test: a request with no file cannot be formatted.', async () => {});

    const response = await request.post(`${API_URL}/format`, { multipart: { describe: 'true' } });
    expect(response.status()).toBe(400);
    expect(await response.text()).toContain('file is required');
  });

  test('Persona: empty file is rejected with 400', async ({ request }) => {
    await test.step('Why we use this test: zero bytes is meaningless.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'empty.json', mimeType: 'application/json', buffer: Buffer.from('') },
      },
    });
    expect(response.status()).toBe(400);
    expect(await response.text()).toContain('input is empty');
  });

  test('Persona: non-JSON garbage is rejected with 415', async ({ request }) => {
    await test.step('Why we use this test: if the extension is not .json and the content does not look like JSON, respond 415.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'garbage.bin', mimeType: 'application/octet-stream', buffer: Buffer.from('this is not json and not any recognised format') },
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