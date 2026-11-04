import { test, expect } from '@playwright/test';

// ================================================================
// Backend API — Formatter Persona (HTML)
//
// This spec exercises the new POST /format endpoint for the HTML
// formatter. It has three groups:
//
//   Group 1: Happy path — the formatter does what the user expects.
//   Group 2: The "never 4xx" contract — broken input comes back 200
//            with a Report, not a 4xx.
//   Group 3: Headers and the raw-bytes path.
//
// All fixtures are inline strings; no filesystem dependency.
// ================================================================

const API_URL = 'http://localhost:8080';

// ================================================================
// GROUP 1: Happy path
// ================================================================

test.describe('Backend API Formatter — HTML Persona', () => {

  test('Persona: formats a single-line HTML fragment', async ({ request }) => {
    await test.step('Why we use this test: the core use case — a messy one-line fragment should come back indented and split across lines.', async () => {});

    const messy = '<div><p>Hello</p><ul><li>One</li><li>Two</li></ul></div>';

    let response;
    await test.step('What we use: POST /format with the messy fragment and describe=true.', async () => {
      response = await request.post(`${API_URL}/format`, {
        multipart: {
          file: {
            name: 'page.html',
            mimeType: 'text/html',
            buffer: Buffer.from(messy),
          },
          describe: 'true',
        },
      });
    });

    await test.step('What we expected: 200 OK.', async () => {
      expect(response.status()).toBe(200);
    });

    await test.step('What we get: format=html, changed=true, applied contains "formatted", and the output is multi-line and indented.', async () => {
      const json = await response.json();
      expect(json.format).toBe('html');
      expect(json.changed).toBe(true);
      expect(Array.isArray(json.applied)).toBe(true);
      expect(json.applied).toContain('formatted');

      const out = json.output as string;
      expect(out).toContain('<div>');
      expect(out).toContain('<p>Hello</p>');
      expect(out).toContain('<ul>');
      expect(out).toContain('<li>One</li>');

      // Multi-line check: the output should have at least 5 newlines
      // (doctype/html/head/body/div are each on their own line).
      expect((out.match(/\n/g) || []).length).toBeGreaterThanOrEqual(5);

      // Indentation check: <p> should be more indented than <div>.
      const divLine = out.split('\n').find((l) => l.includes('<div>')) ?? '';
      const pLine = out.split('\n').find((l) => l.includes('<p>Hello')) ?? '';
      expect(pLine.length).toBeGreaterThan(0);
      expect(divLine.length).toBeGreaterThan(0);
      const divIndent = divLine.length - divLine.trimStart().length;
      const pIndent = pLine.length - pLine.trimStart().length;
      expect(pIndent).toBeGreaterThan(divIndent);
    });
  });

  test('Persona: canonical document comes back unchanged', async ({ request }) => {
    await test.step('Why we use this test: if the file is already pretty, we do not touch it — changed=false, output matches input exactly.', async () => {});

    const canonical =
      '<!DOCTYPE html>\n' +
      '<html>\n' +
      '  <head>\n' +
      '    <title>x</title>\n' +
      '  </head>\n' +
      '  <body>\n' +
      '    <p>Hello</p>\n' +
      '  </body>\n' +
      '</html>\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'page.html',
          mimeType: 'text/html',
          buffer: Buffer.from(canonical),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.format).toBe('html');
    expect(json.changed).toBe(false);
    expect((json.applied ?? []).length).toBe(0);
    expect(json.output).toBe(canonical);
  });

  test('Persona: unquoted attribute values are quoted', async ({ request }) => {
    await test.step('Why we use this test: a common "messy" pattern — <a href=foo> should become <a href="foo">.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'page.html',
          mimeType: 'text/html',
          buffer: Buffer.from('<a href=foo>x</a>'),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.changed).toBe(true);
    expect(json.output).toContain('href="foo"');
  });

  test('Persona: uppercase tags are lowercased', async ({ request }) => {
    await test.step('Why we use this test: <DIV>text</DIV> should become <div>text</div> per HTML5 canonical rules.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'page.html',
          mimeType: 'text/html',
          buffer: Buffer.from('<DIV>text</DIV>'),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.changed).toBe(true);
    expect(json.output).toContain('<div>');
    expect(json.output).toContain('</div>');
  });

  test('Persona: unclosed tags are auto-closed', async ({ request }) => {
    await test.step('Why we use this test: <p>hello (no closing tag) is the classic broken-HTML case. The parser should close it and warn the user.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'page.html',
          mimeType: 'text/html',
          buffer: Buffer.from('<p>hello'),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.changed).toBe(true);
    expect(json.output).toContain('</p>');
    // Expect at least one warning since we had to auto-close.
    expect(Array.isArray(json.warnings)).toBe(true);
    expect(json.warnings.length).toBeGreaterThan(0);
  });

  // ================================================================
  // GROUP 2: The "never 4xx on broken input" contract
  // ================================================================

  test('Persona: badly broken HTML still returns 200, not 400', async ({ request }) => {
    await test.step('Why we use this test: this is the entire point of the new philosophy. A file the HTML5 parser recovers from must NOT surface as a 4xx. The user gets a best-effort output plus a warning.', async () => {});

    // Intentionally garbage: mismatched quotes, unclosed tags, a
    // </span> with no matching open.
    const broken = '<div class=foo bar"><p>Hi</span><ul><li>x</ul>';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'broken.html',
          mimeType: 'text/html',
          buffer: Buffer.from(broken),
        },
        describe: 'true',
      },
    });

    await test.step('What we expected: 200 OK — never 400.', async () => {
      expect(response.status()).toBe(200);
    });

    await test.step('What we get: changed=true, non-empty output, at least one warning.', async () => {
      const json = await response.json();
      expect(json.changed).toBe(true);
      expect(typeof json.output).toBe('string');
      expect(json.output.length).toBeGreaterThan(0);
      expect(Array.isArray(json.warnings)).toBe(true);
      expect(json.warnings.length).toBeGreaterThan(0);
    });
  });

  test('Persona: BOM-prefixed HTML is stripped and formatted', async ({ request }) => {
    await test.step('Why we use this test: many Windows editors save HTML with a UTF-8 BOM. The formatter should strip it (recorded as applied) and continue formatting.', async () => {});

    const bom = '\uFEFF<p>hi</p>';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'bom.html',
          mimeType: 'text/html',
          buffer: Buffer.from(bom, 'utf8'),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.applied).toContain('bom-stripped');
    // Output should no longer start with the BOM byte sequence.
    expect(json.output.startsWith('\uFEFF')).toBe(false);
  });

  // ================================================================
  // GROUP 3: Headers and raw-bytes path
  // ================================================================

  test('Persona: /format emits the X-Format-* headers', async ({ request }) => {
    await test.step('Why we use this test: the raw-bytes path is used by the frontend download button. The headers carry the same info as the JSON envelope.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'page.html',
          mimeType: 'text/html',
          buffer: Buffer.from('<div><p>x</p></div>'),
        },
        // describe intentionally omitted → raw bytes response
      },
    });

    expect(response.status()).toBe(200);

    const headers = response.headers();
    expect(headers['x-format-format']).toBe('html');
    expect(headers['x-format-changed']).toBe('true');
    expect(headers['x-format-applied']).toContain('html;changed=true');
    // CORS must expose them so browser JS can read them.
    expect(headers['access-control-expose-headers']).toContain('X-Format-Applied');
  });

  test('Persona: raw-bytes response is the formatted HTML', async ({ request }) => {
    await test.step('Why we use this test: when describe is absent, the response body must be the formatted HTML, not a JSON envelope. This is what the download button relies on.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'page.html',
          mimeType: 'text/html',
          buffer: Buffer.from('<div><p>x</p></div>'),
        },
      },
    });

    expect(response.status()).toBe(200);
    expect(response.headers()['content-type']).toContain('application/octet-stream');

    const body = await response.text();
    expect(body).toContain('<div>');
    expect(body).toContain('<p>x</p>');

    // Confirm it's NOT a JSON envelope.
    expect(() => JSON.parse(body)).toThrow();
  });

  // ================================================================
  // GROUP 4: Real 4xx cases (only two exist)
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

  test('Persona: empty file is rejected with 400', async ({ request }) => {
    await test.step('Why we use this test: zero bytes is meaningless for formatting — the second of the only two 4xx cases.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'empty.html',
          mimeType: 'text/html',
          buffer: Buffer.from(''),
        },
      },
    });

    expect(response.status()).toBe(400);
    expect(await response.text()).toContain('input is empty');
  });

  test('Persona: non-HTML input is rejected with 415', async ({ request }) => {
    await test.step('Why we use this test: the formatter cannot identify the format. If the extension is not .html/.htm and the content does not look like HTML, respond 415 Unsupported Media Type.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'garbage.bin',
          mimeType: 'application/octet-stream',
          buffer: Buffer.from('this is not html or any recognised format'),
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