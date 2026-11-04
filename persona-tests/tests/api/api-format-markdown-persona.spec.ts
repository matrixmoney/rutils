import { test, expect } from '@playwright/test';

// ================================================================
// Backend API вҖ” Formatter Persona (Markdown)
//
// Four groups:
//   Group 1: Happy path вҖ” messy Markdown becomes canonical.
//   Group 2: Fixed-point вҖ” canonical Markdown round-trips unchanged.
//   Group 3: The "never 4xx on broken input" contract.
//   Group 4: Real 4xx cases.
// ================================================================

const API_URL = 'http://localhost:8080';

test.describe('Backend API Formatter вҖ” Markdown Persona', () => {

  // ================================================================
  // GROUP 1: Happy path вҖ” formatting
  // ================================================================

  test('Persona: messy Markdown comes back formatted', async ({ request }) => {
    await test.step('Why we use this test: `#Heading` (no space) is not a heading in any Markdown spec; the formatter must fix it.', async () => {});

    const messy = '#Title\n\n\n\n##Section  \n\nSome text.\n';

    let response;
    await test.step('What we use: POST /format with the messy Markdown and describe=true.', async () => {
      response = await request.post(`${API_URL}/format`, {
        multipart: {
          file: { name: 'README.md', mimeType: 'text/markdown', buffer: Buffer.from(messy) },
          describe: 'true',
        },
      });
    });

    await test.step('What we expected: 200 OK.', async () => {
      expect(response.status()).toBe(200);
    });

    await test.step('What we get: format=md, changed=true, `# Title` and `## Section`.', async () => {
      const json = await response.json();
      expect(json.format).toBe('md');
      expect(json.changed).toBe(true);
      expect(Array.isArray(json.applied)).toBe(true);

      const out = json.output as string;
      expect(out).toContain('# Title');
      expect(out).toContain('## Section');
      expect(out).not.toContain('#Title');
    });
  });

  test('Persona: canonical Markdown comes back unchanged (Changed=false)', async ({ request }) => {
    await test.step('Why we use this test: if the file is already formatted, Changed=false and output is byte-identical.', async () => {});

    const canonical = '# Title\n\n## Section\n\nSome text.\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'README.md', mimeType: 'text/markdown', buffer: Buffer.from(canonical) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.format).toBe('md');
    expect(json.changed).toBe(false);
    expect(json.output).toBe(canonical);
  });

  test('Persona: closing hash sequence stripped', async ({ request }) => {
    await test.step('Why we use this test: `# Heading ###` should become `# Heading` вҖ” the closing hashes are cosmetic.', async () => {});

    const input = '# Heading ###\n\nSome text.\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'README.md', mimeType: 'text/markdown', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.output).toContain('# Heading');
    expect(json.output).not.toContain('###');
  });

  test('Persona: fenced code block preserved verbatim', async ({ request }) => {
    await test.step('Why we use this test: whitespace inside a fenced code block is code вҖ” it must not be touched.', async () => {});

    const input = '# Title\n\n```python\ndef foo():\n    x = 1\n\n    return   x + 2\n```\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'README.md', mimeType: 'text/markdown', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    const out = json.output as string;
    expect(out).toContain('    x = 1');
    expect(out).toContain('    return   x + 2');
    expect(out).toContain('```python');
  });

  test('Persona: runs of blank lines collapsed', async ({ request }) => {
    await test.step('Why we use this test: multiple consecutive blank lines are noise; one is the paragraph separator.', async () => {});

    const input = '# Title\n\n\n\n\nParagraph.\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'README.md', mimeType: 'text/markdown', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    const out = json.output as string;
    expect(out).not.toMatch(/\n\n\n/);
    expect(out).toContain('# Title\n\nParagraph.');
  });

  test('Persona: trailing whitespace stripped', async ({ request }) => {
    await test.step('Why we use this test: trailing whitespace is invisible and confusing; the formatter strips it.', async () => {});

    const input = '# Title   \n\nParagraph with trailing spaces   \n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'README.md', mimeType: 'text/markdown', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    const out = json.output as string;
    expect(out).not.toContain('# Title   ');
    expect(out).not.toContain('trailing spaces   ');
    expect(out).toContain('# Title\n');
  });

  test('Persona: BOM-prefixed Markdown works', async ({ request }) => {
    await test.step('Why we use this test: some editors save Markdown with a UTF-8 BOM.', async () => {});

    const bom = '\uFEFF# Title\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'bom.md', mimeType: 'text/markdown', buffer: Buffer.from(bom, 'utf8') },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.applied).toContain('bom-stripped');
    expect(json.output.startsWith('\uFEFF')).toBe(false);
  });

  // ================================================================
  // GROUP 2: The "never 4xx on broken input" contract
  // ================================================================

  test('Persona: unclosed code fence preserved verbatim, warning emitted', async ({ request }) => {
    await test.step('Why we use this test: an unclosed fence is a common Markdown defect. The content must be preserved and a warning emitted, not a 4xx.', async () => {});

    const input = '# Title\n\n```python\nx = 1\ny = 2\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'broken.md', mimeType: 'text/markdown', buffer: Buffer.from(input) },
        describe: 'true',
      },
    });

    await test.step('What we expected: 200 OK вҖ” never 400.', async () => {
      expect(response.status()).toBe(200);
    });

    await test.step('What we get: content preserved, warning present.', async () => {
      const json = await response.json();
      expect(typeof json.output).toBe('string');
      expect(json.output).toContain('```python');
      expect(json.output).toContain('x = 1');
      expect(json.output).toContain('y = 2');
      expect(Array.isArray(json.warnings)).toBe(true);
      expect(json.warnings.length).toBeGreaterThan(0);
    });
  });

  test('Persona: garbage text still returns 200', async ({ request }) => {
    await test.step('Why we use this test: even a file with no Markdown syntax must not surface as a 4xx.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'broken.md', mimeType: 'text/markdown', buffer: Buffer.from('this is not markdown at all\njust plain text\n') },
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

  test('Persona: /format emits X-Format-* headers for Markdown', async ({ request }) => {
    await test.step('Why we use this test: the raw-bytes path (no describe) uses the headers to convey the report.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'README.md', mimeType: 'text/markdown', buffer: Buffer.from('#Title\n') },
      },
    });

    expect(response.status()).toBe(200);
    const headers = response.headers();
    expect(headers['x-format-format']).toBe('md');
    expect(headers['x-format-changed']).toBe('true');
    expect(headers['x-format-applied']).toContain('md;changed=true');
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
        file: { name: 'empty.md', mimeType: 'text/markdown', buffer: Buffer.from('') },
      },
    });
    expect(response.status()).toBe(400);
    expect(await response.text()).toContain('input is empty');
  });

  test('Persona: non-Markdown garbage is rejected with 415', async ({ request }) => {
    await test.step('Why we use this test: if the extension is not .md and the content does not look like Markdown, respond 415.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: { name: 'garbage.bin', mimeType: 'application/octet-stream', buffer: Buffer.from('this is definitely not markdown and not any recognised format') },
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