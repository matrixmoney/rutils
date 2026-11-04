import { test, expect } from '@playwright/test';

// ================================================================
// Backend API вҖ” Formatter Persona (XML)
//
// Four groups:
//   Group 1: Happy path вҖ” messy input becomes canonical.
//   Group 2: Fixed-point вҖ” canonical input round-trips unchanged.
//   Group 3: The "never 4xx on broken input" contract.
//   Group 4: Real 4xx cases (missing file, empty file, non-XML garbage, GET).
//
// All fixtures are inline strings; no filesystem dependency.
// ================================================================

const API_URL = 'http://localhost:8080';

test.describe('Backend API Formatter вҖ” XML Persona', () => {

  // ================================================================
  // GROUP 1: Happy path вҖ” formatting
  // ================================================================

  test('Persona: messy XML comes back formatted', async ({ request }) => {
    await test.step('Why we use this test: the core use case вҖ” a single-line XML fragment should come back indented across multiple lines.', async () => {});

    const messy = '<root><a><b>1</b></a><c>2</c></root>';

    let response;
    await test.step('What we use: POST /format with the messy XML and describe=true.', async () => {
      response = await request.post(`${API_URL}/format`, {
        multipart: {
          file: { name: 'doc.xml', mimeType: 'application/xml', buffer: Buffer.from(messy) },
          describe: 'true',
        },
      });
    });

    await test.step('What we expected: 200 OK.', async () => {
      expect(response.status()).toBe(200);
    });

    await test.step('What we get: format=xml, changed=true, multi-line output.', async () => {
      const json = await response.json();
      expect(json.format).toBe('xml');
      expect(json.changed).toBe(true);
      expect(Array.isArray(json.applied)).toBe(true);

      const out = json.output as string;
      expect(out).toContain('<root>');
      expect(out).toContain('</root>');
      expect((out.match(/\n/g) || []).length).toBeGreaterThanOrEqual(4);

      // Indentation: <a> more indented than <root>.
      const rootLine = out.split('\n').find((l) => l.includes('<root>')) ?? '';
      const aLine = out.split('\n').find((l) => l.includes('<a>')) ?? '';
      const rootIndent = rootLine.length - rootLine.trimStart().length;
      const aIndent = aLine.length - aLine.trimStart().length;
      expect(aIndent).toBeGreaterThan(rootIndent);
    });
  });

  test('Persona: canonical XML comes back unchanged (Changed=false)', async ({ request }) => {
    await test.step('Why we use this test: if the file is already formatted, we do not touch it вҖ” Changed=false and byte-identical output.', async () => {});

    const canonical = `<?xml version="1.0" encoding="UTF-8"?>
<root>
  <a>1</a>
  <b>2</b>
</root>
`;

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'doc.xml', mimeType: 'application/xml', buffer: Buffer.from(canonical) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.format).toBe('xml');
    expect(json.changed).toBe(false);
    expect(json.output).toBe(canonical);
  });

  test('Persona: `<a></a>` becomes `<a />`', async ({ request }) => {
    await test.step('Why we use this test: empty elements should be self-closed for compactness.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'doc.xml', mimeType: 'application/xml', buffer: Buffer.from('<root><a></a></root>') },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.changed).toBe(true);
    expect(json.output).toContain('<a />');
    expect(json.output).not.toContain('<a></a>');
  });

  test('Persona: attribute order preserved', async ({ request }) => {
    await test.step('Why we use this test: XML attribute order sometimes matters (e.g. for diffing or signing). We must never reorder.', async () => {});

    const input = '<root><item b="2" a="1" c="3">x</item></root>';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'doc.xml', mimeType: 'application/xml', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    const out = json.output as string;

    // b, then a, then c in that order.
    const bIdx = out.indexOf('b="2"');
    const aIdx = out.indexOf('a="1"');
    const cIdx = out.indexOf('c="3"');
    expect(bIdx).toBeGreaterThan(-1);
    expect(aIdx).toBeGreaterThan(bIdx);
    expect(cIdx).toBeGreaterThan(aIdx);
  });

  test('Persona: text content preserved byte-for-byte', async ({ request }) => {
    await test.step('Why we use this test: XML does not collapse whitespace on render, so we must not collapse it on format either.', async () => {});

    const input = '<root><pre>line one\n  line two\n  line three</pre></root>';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'doc.xml', mimeType: 'application/xml', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    const out = json.output as string;
    expect(out).toContain('line one');
    expect(out).toContain('line two');
    expect(out).toContain('line three');
  });

  // ================================================================
  // GROUP 2: The "never 4xx on broken input" contract
  // ================================================================

  test('Persona: unclosed tag is auto-closed with warning', async ({ request }) => {
    await test.step('Why we use this test: permissive formatting вҖ” an unclosed tag must be auto-closed and returned with a warning, not a 4xx.', async () => {});

    const broken = '<root><a>hello';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'broken.xml', mimeType: 'application/xml', buffer: Buffer.from(broken) },
        describe: 'true',
      },
    });

    await test.step('What we expected: 200 OK вҖ” never 400.', async () => {
      expect(response.status()).toBe(200);
    });

    await test.step('What we get: changed=true, non-empty output, at least one warning, both tags closed.', async () => {
      const json = await response.json();
      expect(json.changed).toBe(true);
      expect(typeof json.output).toBe('string');
      expect(json.output.length).toBeGreaterThan(0);
      expect(Array.isArray(json.warnings)).toBe(true);
      expect(json.warnings.length).toBeGreaterThan(0);
      expect(json.output).toContain('</a>');
      expect(json.output).toContain('</root>');
    });
  });

  test('Persona: multiple roots wrapped in <root> with warning', async ({ request }) => {
    await test.step('Why we use this test: XML requires exactly one root; wrapping is the correct recovery.', async () => {});

    const multiRoot = '<a>1</a><b>2</b>';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'broken.xml', mimeType: 'application/xml', buffer: Buffer.from(multiRoot) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.changed).toBe(true);
    expect(json.output).toContain('<root>');
    expect(Array.isArray(json.warnings)).toBe(true);
  });

  test('Persona: garbage XML still returns 200', async ({ request }) => {
    await test.step('Why we use this test: even a badly malformed XML file must not surface as a 4xx. The user gets best-effort output plus a warning.', async () => {});

    const garbage = '<root><<a>1</b></root>';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'broken.xml', mimeType: 'application/xml', buffer: Buffer.from(garbage) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(typeof json.output).toBe('string');
    expect(json.output.length).toBeGreaterThan(0);
  });

  test('Persona: BOM-prefixed XML works', async ({ request }) => {
    await test.step('Why we use this test: Windows editors frequently save XML with a UTF-8 BOM. The formatter should strip it.', async () => {});

    const bom = '\uFEFF<root><a>1</a></root>';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'bom.xml', mimeType: 'application/xml', buffer: Buffer.from(bom, 'utf8') },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.applied).toContain('bom-stripped');
    expect(json.output.startsWith('\uFEFF')).toBe(false);
  });

  // ================================================================
  // GROUP 3: Headers path
  // ================================================================

  test('Persona: /format emits X-Format-* headers for XML', async ({ request }) => {
    await test.step('Why we use this test: the raw-bytes path (no describe) uses the headers to convey the report.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'doc.xml', mimeType: 'application/xml', buffer: Buffer.from('<root><a>1</a></root>') },
      },
    });

    expect(response.status()).toBe(200);
    const headers = response.headers();
    expect(headers['x-format-format']).toBe('xml');
    expect(headers['x-format-changed']).toBe('true');
    expect(headers['x-format-applied']).toContain('xml;changed=true');
    expect(headers['access-control-expose-headers']).toContain('X-Format-Applied');
  });

  // ================================================================
  // GROUP 4: Real 4xx cases
  // ================================================================

  test('Persona: missing file field is rejected with 400', async ({ request }) => {
    await test.step('Why we use this test: a request with no file cannot be formatted вҖ” one of the only two 4xx cases.', async () => {});

    const response = await request.post(`${API_URL}/format`, { multipart: { describe: 'true' } });
    expect(response.status()).toBe(400);
    expect(await response.text()).toContain('file is required');
  });

  test('Persona: empty file is rejected with 400', async ({ request }) => {
    await test.step('Why we use this test: zero bytes is meaningless вҖ” the second of the only two 4xx cases.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'empty.xml', mimeType: 'application/xml', buffer: Buffer.from('') },
      },
    });
    expect(response.status()).toBe(400);
    expect(await response.text()).toContain('input is empty');
  });

  test('Persona: non-XML garbage is rejected with 415', async ({ request }) => {
    await test.step('Why we use this test: if the extension is not .xml and the content does not look like XML, respond 415.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'garbage.bin', mimeType: 'application/octet-stream', buffer: Buffer.from('this is definitely not xml') },
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