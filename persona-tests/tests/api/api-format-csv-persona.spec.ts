import { test, expect } from '@playwright/test';

// ================================================================
// Backend API вҖ” Formatter Persona (CSV)
//
// Four groups:
//   Group 1: Happy path вҖ” messy CSV becomes canonical.
//   Group 2: Fixed-point вҖ” canonical CSV round-trips unchanged.
//   Group 3: The "never 4xx on broken input" contract.
//   Group 4: Real 4xx cases.
// ================================================================

const API_URL = 'http://localhost:8080';

test.describe('Backend API Formatter вҖ” CSV Persona', () => {

  // ================================================================
  // GROUP 1: Happy path вҖ” formatting
  // ================================================================

  test('Persona: messy CSV comes back canonicalised', async ({ request }) => {
    await test.step('Why we use this test: CRLF endings and trailing whitespace must be normalised.', async () => {});

    const messy = 'id,name,age\r\n1, Alice ,30\r\n2, Bob ,25\r\n';

    let response;
    await test.step('What we use: POST /format with the messy CSV and describe=true.', async () => {
      response = await request.post(`${API_URL}/format`, {
        multipart: {
          file: { name: 'data.csv', mimeType: 'text/csv', buffer: Buffer.from(messy) },
          describe: 'true',
        },
      });
    });

    await test.step('What we expected: 200 OK.', async () => {
      expect(response.status()).toBe(200);
    });

    await test.step('What we get: format=csv, changed=true, LF endings, no trailing spaces.', async () => {
      const json = await response.json();
      expect(json.format).toBe('csv');
      expect(json.changed).toBe(true);
      const out = json.output as string;
      expect(out).not.toContain('\r');
      expect(out).not.toContain('Alice ,');
      expect(out.endsWith('\n')).toBe(true);
    });
  });

  test('Persona: canonical CSV comes back unchanged', async ({ request }) => {
    await test.step('Why we use this test: if the file is already formatted, Changed=false and output is byte-identical.', async () => {});

    const canonical = 'id,name,age\n1,Alice,30\n2,Bob,25\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'data.csv', mimeType: 'text/csv', buffer: Buffer.from(canonical) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.format).toBe('csv');
    expect(json.changed).toBe(false);
    expect(json.output).toBe(canonical);
  });

  test('Persona: semicolon delimiter preserved', async ({ request }) => {
    await test.step('Why we use this test: European exports often use semicolons. We must preserve the dominant delimiter.', async () => {});

    const input = 'a;b;c\n1;2;3\n4;5;6\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'data.csv', mimeType: 'text/csv', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.output).toContain('1;2;3');
  });

  test('Persona: tab delimiter preserved', async ({ request }) => {
    await test.step('Why we use this test: TSV-style exports must keep tabs.', async () => {});

    const input = 'a\tb\tc\n1\t2\t3\n4\t5\t6\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'data.tsv', mimeType: 'text/tab-separated-values', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.output).toContain('1\t2\t3');
  });

  test('Persona: quoted field with comma preserved', async ({ request }) => {
    await test.step('Why we use this test: fields with commas inside quotes must remain quoted and re-parseable.', async () => {});

    const input = 'id,name\n1,"Doe, John"\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'data.csv', mimeType: 'text/csv', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.output).toContain('Doe, John');
    expect(json.output).toContain('"Doe, John"');
  });

  test('Persona: BOM-prefixed CSV works', async ({ request }) => {
    await test.step('Why we use this test: Excel exports with BOM are common and must not break header detection.', async () => {});

    const bom = '\uFEFFid,name\n1,Alice\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'bom.csv', mimeType: 'text/csv', buffer: Buffer.from(bom, 'utf8') },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.applied).toContain('bom-stripped');
    expect(json.output.startsWith('id')).toBe(true);
  });

  // ================================================================
  // GROUP 2: Ragged rows вҖ” the "never 4xx" contract
  // ================================================================

  test('Persona: short rows padded to header width', async ({ request }) => {
    await test.step('Why we use this test: short rows are the most common CSV defect; they must be padded, not rejected.', async () => {});

    const input = 'id,name,age\n1,Alice\n2,Bob,25\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'data.csv', mimeType: 'text/csv', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.changed).toBe(true);
    expect(json.applied).toContain('rows-padded');
    expect(Array.isArray(json.warnings)).toBe(true);
  });

  test('Persona: long rows truncated to header width', async ({ request }) => {
    await test.step('Why we use this test: long rows must be truncated with warning, not rejected.', async () => {});

    const input = 'id,name\n1,Alice,extra1,extra2\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'data.csv', mimeType: 'text/csv', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.changed).toBe(true);
    expect(json.applied).toContain('rows-truncated');
    expect(json.warnings.some((w: string) => w.includes('truncated'))).toBe(true);
  });

  test('Persona: completely unparseable content still returns 200', async ({ request }) => {
    await test.step('Why we use this test: even content with no delimiters must return a best-effort output with a warning.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'data.csv', mimeType: 'text/csv', buffer: Buffer.from('just plain text\nwith no delimiters\n') },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(typeof json.output).toBe('string');
    expect(json.output.length).toBeGreaterThan(0);
  });

  // ================================================================
  // GROUP 3: Headers path
  // ================================================================

  test('Persona: /format emits X-Format-* headers for CSV', async ({ request }) => {
    await test.step('Why we use this test: the raw-bytes path (no describe) uses the headers to convey the report.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'data.csv', mimeType: 'text/csv', buffer: Buffer.from('id,name\n1,Alice\n') },
      },
    });

    expect(response.status()).toBe(200);
    const headers = response.headers();
    expect(headers['x-format-format']).toBe('csv');
    expect(headers['x-format-changed']).toBe('false');
    expect(headers['x-format-applied']).toContain('csv;changed=false');
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
        file: { name: 'empty.csv', mimeType: 'text/csv', buffer: Buffer.from('') },
      },
    });
    expect(response.status()).toBe(400);
    expect(await response.text()).toContain('input is empty');
  });

  test('Persona: non-CSV garbage is rejected with 415', async ({ request }) => {
    await test.step('Why we use this test: if the extension is not .csv and the content does not look like CSV, respond 415.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'garbage.bin', mimeType: 'application/octet-stream', buffer: Buffer.from('this is not csv and not any recognised format') },
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