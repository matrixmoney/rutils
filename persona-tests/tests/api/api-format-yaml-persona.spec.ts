import { test, expect } from '@playwright/test';

// ================================================================
// Backend API — Formatter Persona (YAML)
//
// This spec exercises the new POST /format endpoint for the YAML
// formatter. YAML formatting is deliberately conservative (see
// yaml.go for the design rationale), so the tests focus on the
// safe transformations and the "never 4xx on messy input" contract.
// ================================================================

const API_URL = 'http://localhost:8080';

// ================================================================
// GROUP 1: Happy path — formatting
// ================================================================

test.describe('Backend API Formatter — YAML Persona', () => {

  test('Persona: messy YAML comes back formatted', async ({ request }) => {
    await test.step('Why we use this test: the core use case — missing space after colon, tab indent, and blank-line runs should all be canonicalised.', async () => {});

    const messy = 'apiVersion: v1\nkind:ConfigMap\nmetadata:\n\tname: my-config\n\n\n\ndata:\n  key1:value1\n';

    let response;
    await test.step('What we use: POST /format with the messy YAML and describe=true.', async () => {
      response = await request.post(`${API_URL}/format`, {
        multipart: {
          file: {
            name: 'config.yaml',
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

    await test.step('What we get: format=yaml, changed=true, colon spacing fixed, tabs expanded, blank lines collapsed.', async () => {
      const json = await response.json();
      expect(json.format).toBe('yaml');
      expect(json.changed).toBe(true);
      expect(Array.isArray(json.applied)).toBe(true);

      const out = json.output as string;
      expect(out).toContain('kind: ConfigMap');
      expect(out).toContain('key1: value1');
      expect(out).not.toContain('\t');
      expect(out).not.toMatch(/\n\n\n/);
    });
  });

  test('Persona: canonical YAML comes back unchanged (Changed=false)', async ({ request }) => {
    await test.step('Why we use this test: if the file is already formatted, we do not touch it — changed=false and byte-identical output.', async () => {});

    const canonical = `apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config
  namespace: default
data:
  key1: value1
  key2: value2
`;

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'config.yaml',
          mimeType: 'text/plain',
          buffer: Buffer.from(canonical),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.format).toBe('yaml');
    expect(json.changed).toBe(false);
    expect(json.output).toBe(canonical);
  });

  test('Persona: `key:value` becomes `key: value`', async ({ request }) => {
    await test.step('Why we use this test: the most common YAML formatting bug — missing space after colon.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'config.yaml',
          mimeType: 'text/plain',
          buffer: Buffer.from('name:Alice\nage:30\n'),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    expect(json.changed).toBe(true);
    expect(json.output).toContain('name: Alice');
    expect(json.output).toContain('age: 30');
    expect(json.output).not.toContain('name:Alice');
  });

  test('Persona: comments preserved', async ({ request }) => {
    await test.step('Why we use this test: YAML comments carry documentation and disable-config intent — we must never drop them.', async () => {});

    const input = `# Top-level comment
apiVersion: v1
kind: ConfigMap  # inline comment
metadata:
  # indented comment
  name: my-config
`;

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'config.yaml',
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

  test('Persona: key and section order preserved', async ({ request }) => {
    await test.step('Why we use this test: YAML sometimes depends on key order (e.g. Kubernetes strategic merge patches). We must never sort.', async () => {});

    const input = `zebra: 1
alpha: 2
apple: 3
beta: 4
`;

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'config.yaml',
          mimeType: 'text/plain',
          buffer: Buffer.from(input),
        },
        describe: 'true',
      },
    });

    expect(response.status()).toBe(200);
    const json = await response.json();
    const out = json.output as string;

    const zebraIdx = out.indexOf('zebra:');
    const alphaIdx = out.indexOf('alpha:');
    const appleIdx = out.indexOf('apple:');
    const betaIdx = out.indexOf('beta:');
    expect(zebraIdx).toBeGreaterThan(-1);
    expect(alphaIdx).toBeGreaterThan(zebraIdx);
    expect(appleIdx).toBeGreaterThan(alphaIdx);
    expect(betaIdx).toBeGreaterThan(appleIdx);
  });

  test('Persona: runs of blank lines collapse to a single blank', async ({ request }) => {
    await test.step('Why we use this test: multiple consecutive blank lines are noise; one blank line is enough separation.', async () => {});

    const input = 'a: 1\n\n\n\nb: 2\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'config.yaml',
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
    expect(json.output).toContain('a: 1\n\nb: 2');
  });

  // ================================================================
  // GROUP 2: The "never 4xx on broken input" contract
  // ================================================================

  test('Persona: tab-indented YAML is expanded to spaces, still 200', async ({ request }) => {
    await test.step('Why we use this test: tabs in indentation are illegal in YAML. The formatter should expand them to spaces rather than fail.', async () => {});

    const input = 'a:\n\tb: 1\nc: 2\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'tabs.yaml',
          mimeType: 'text/plain',
          buffer: Buffer.from(input),
        },
        describe: 'true',
      },
    });

    await test.step('What we expected: 200 OK — never 400.', async () => {
      expect(response.status()).toBe(200);
    });

    await test.step('What we get: changed=true, no tabs in output, and the tab-indent-expanded fix is recorded.', async () => {
      const json = await response.json();
      expect(json.changed).toBe(true);
      expect(json.output).not.toContain('\t');
      expect(json.applied).toContain('tab-indent-expanded');
    });
  });

  test('Persona: BOM-prefixed YAML works', async ({ request }) => {
    await test.step('Why we use this test: many editors save YAML with a UTF-8 BOM. The formatter should strip it (recorded as applied) and continue.', async () => {});

    const bom = '\uFEFFapiVersion: v1\nkind: ConfigMap\n';

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'bom.yaml',
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
    expect(json.output).toContain('apiVersion: v1');
  });

  // ================================================================
  // GROUP 3: Headers path
  // ================================================================

  test('Persona: /format emits the X-Format-* headers for YAML', async ({ request }) => {
    await test.step('Why we use this test: the raw-bytes path (no describe) relies on the headers to convey the report.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'config.yaml',
          mimeType: 'text/plain',
          buffer: Buffer.from('kind:ConfigMap\nname:test\n'),
        },
      },
    });

    expect(response.status()).toBe(200);

    const headers = response.headers();
    expect(headers['x-format-format']).toBe('yaml');
    expect(headers['x-format-changed']).toBe('true');
    expect(headers['x-format-applied']).toContain('yaml;changed=true');
    expect(headers['access-control-expose-headers']).toContain('X-Format-Applied');
  });

  // ================================================================
  // GROUP 4: Real 4xx cases (only the ones the spec allows)
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

  test('Persona: non-YAML garbage is rejected with 415', async ({ request }) => {
    await test.step('Why we use this test: the formatter cannot identify the format. If the extension is not .yaml/.yml and the content does not look like YAML, respond 415.', async () => {});

    const response = await request.post(`${API_URL}/format`, {
      multipart: {
        file: {
          name: 'garbage.bin',
          mimeType: 'application/octet-stream',
          buffer: Buffer.from('this is not yaml and not any recognised format'),
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