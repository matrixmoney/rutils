import { test, expect } from '@playwright/test';

// ================================================================
// Backend API — Formatter Persona (TOML)
//
// TOML formatting is conservative (see toml.go for the design
// rationale). The tests focus on the safe canonicalisations and the
// "never 4xx on messy input" contract.
// ================================================================

const API_URL = 'http://localhost:8080';

// ================================================================
// GROUP 1: Happy path — formatting
// ================================================================

test.describe('Backend API Formatter — TOML Persona', () => {

  test('Persona: messy TOML comes back formatted', async ({ request }) => {
    await test.step('Why we use this test: the core use case — missing space around `=`, inner header whitespace, and blank-line runs should all be canonicalised.', async () => {});

    const messy = '[package]\nname="my-crate"\nversion   ="0.1.0"\n\n\n\n[ deps ]\nserde="1.0"\n';

    let response;
    await test.step('What we use: POST /format with the messy TOML and describe=true.', async () => {
      response = await request.post(`${API_URL}/format`, {
        multipart: {
          file: {
            name: 'Cargo.toml',
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

    await test.step('What we get: format=toml, changed=true, key=value spacing fixed, headers trimmed, blank lines collapsed.', async () => {
      const json = await response.json();
      expect(json.format).toBe('toml');
      expect(json.changed).toBe(true);
      expect(Array.isArray(json.applied)).toBe(true);

      const out = json.output as string;
      expect(out).toContain('name = "my-crate"');
      expect(out).toContain('version = "0.1.0"');
      expect(out).toContain('serde = "1.0"');
      expect(out).toContain('[deps]');
      expect(out).not.toMatch(/\n\n\n/);
    });
  });

  test('Persona: canonical TOML comes back unchanged (Changed=false)', async ({ request }) => {
    await test.step('Why we use this test: if the file is already formatted, we do not touch it — changed=false and byte-identical output.', async () => {});

    const canonical = `[package]
name = "my-crate"
version = "0.1.0"
edition = "2021"

[dependencies]
serde = "1.0"
tokio = { version = "1", features = ["full"] }
`;

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'Cargo.toml',
          mimeType: 'text/plain',
          buffer: Buffer.from(canonical),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.format).toBe('toml');
    expect(json.changed).toBe(false);
    expect(json.output).toBe(canonical);
  });

  test('Persona: `key=value` becomes `key = value`', async ({ request }) => {
    await test.step('Why we use this test: the most common TOML spacing bug.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'test.toml',
          mimeType: 'text/plain',
          buffer: Buffer.from('name="alice"\nage=30\n'),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.changed).toBe(true);
    expect(json.output).toContain('name = "alice"');
    expect(json.output).toContain('age = 30');
    expect(json.output).not.toContain('name="alice"');
  });

  test('Persona: comments preserved', async ({ request }) => {
    await test.step('Why we use this test: TOML comments carry documentation and disable-config intent — we must never drop them.', async () => {});

    const input = `# Top-level comment
[package]
name = "x"  # inline comment
# indented comment
version = "0.1"
`;

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'test.toml',
          mimeType: 'text/plain',
          buffer: Buffer.from(input),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    const out = json.output as string;
    expect(out).toContain('# Top-level comment');
    expect(out).toContain('# inline comment');
    expect(out).toContain('# indented comment');
  });

  test('Persona: key and table order preserved', async ({ request }) => {
    await test.step('Why we use this test: TOML tables and keys often define a structure; order matters for readability and sometimes for semantic reasons (arrays of tables). We must never sort.', async () => {});

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
          name: 'test.toml',
          mimeType: 'text/plain',
          buffer: Buffer.from(input),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    const out = json.output as string;

    const zebraIdx = out.indexOf('[zebra]');
    const appleIdx = out.indexOf('[apple]');
    expect(zebraIdx).toBeGreaterThan(-1);
    expect(appleIdx).toBeGreaterThan(zebraIdx);

    const zzzIdx = out.indexOf('zzz = 1');
    const aaaIdx = out.indexOf('aaa = 2');
    expect(zzzIdx).toBeGreaterThan(-1);
    expect(aaaIdx).toBeGreaterThan(zzzIdx);
  });

  test('Persona: runs of blank lines collapse to a single blank', async ({ request }) => {
    await test.step('Why we use this test: multiple consecutive blank lines are noise; one is enough.', async () => {});

    const input = 'a = 1\n\n\n\nb = 2\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'test.toml',
          mimeType: 'text/plain',
          buffer: Buffer.from(input),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.changed).toBe(true);
    expect(json.output).not.toMatch(/\n\n\n/);
    expect(json.output).toContain('a = 1\n\nb = 2');
  });

  test('Persona: array-of-tables headers normalised', async ({ request }) => {
    await test.step('Why we use this test: `[[ product ]]` with inner whitespace should become `[[product]]`.', async () => {});

    const input = '[[ product ]]\nname = "x"\nprice = 10\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'test.toml',
          mimeType: 'text/plain',
          buffer: Buffer.from(input),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.output).toContain('[[product]]');
  });

  // ================================================================
  // GROUP 2: The "never 4xx on messy input" contract
  // ================================================================

  test('Persona: garbage line preserved with 200 response', async ({ request }) => {
    await test.step('Why we use this test: permissive formatting means a line that cannot be parsed must NOT cause a 4xx — the user gets it back with a warning.', async () => {});

    const broken = `[section]
key = "value"
this is not TOML syntax
another = "one"
`;

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'broken.toml',
          mimeType: 'text/plain',
          buffer: Buffer.from(broken),
        },
        describe: 'true',
      },
    });

    await test.step('What we expected: 200 OK — never 400.', async () => {
      expect(response.status()).toBe(200);
    });

    await test.step('What we get: the garbage line survives verbatim in the output.', async () => {
      const json = await response.json();
      expect(typeof json.output).toBe('string');
      expect(json.output).toContain('this is not TOML syntax');
      expect(json.output).toContain('key = "value"');
      expect(json.output).toContain('another = "one"');
    });
  });

  test('Persona: BOM-prefixed TOML works', async ({ request }) => {
    await test.step('Why we use this test: some editors save TOML with a UTF-8 BOM. The formatter should strip it (recorded as applied).', async () => {});

    const bom = '\uFEFF[package]\nname = "x"\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'bom.toml',
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
  });

  // ================================================================
  // GROUP 3: Headers path
  // ================================================================

  test('Persona: /format emits the X-Format-* headers for TOML', async ({ request }) => {
    await test.step('Why we use this test: the raw-bytes path (no describe) relies on the headers to convey the report.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'Cargo.toml',
          mimeType: 'text/plain',
          buffer: Buffer.from('[package]\nname="x"\nversion = "0.1"\n'),
        },
      },
    });

    expect(response.status()).toBe(200);

    const headers = response.headers();
    expect(headers['x-format-format']).toBe('toml');
    expect(headers['x-format-changed']).toBe('true');
    expect(headers['x-format-applied']).toContain('toml;changed=true');
    expect(headers['access-control-expose-headers']).toContain('X-Format-Applied');
  });

  // ================================================================
  // GROUP 4: Real 4xx cases
  // ================================================================

  test('Persona: missing file field is rejected with 400', async ({ request }) => {
    await test.step('Why we use this test: a request with no file cannot be formatted — one of the only two 4xx cases.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        describe: 'true',
      },
    });

    expect(response.status()).toBe(400);
    expect(await response.text()).toContain('file is required');
  });

  test('Persona: non-TOML garbage is rejected with 415', async ({ request }) => {
    await test.step('Why we use this test: the formatter cannot identify the format. If the extension is not .toml and the content does not look like TOML, respond 415.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'garbage.bin',
          mimeType: 'application/octet-stream',
          buffer: Buffer.from('this is not toml and not any recognised format'),
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