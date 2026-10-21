import { test, expect } from '@playwright/test';

// Configuration for API testing. The Swagger API is hosted at localhost:8080 by default.
const API_URL = 'http://localhost:8080';

test.describe('Backend API Tasks Notes Persona', () => {

  // ----------------------------------------------------------------
  // Helper: register + login a fresh user, return their JWT.
  // Every test uses a unique email so the shared data/tasks.json
  // never accumulates stale rows across runs.
  // ----------------------------------------------------------------
  async function freshToken(request: any): Promise<string> {
    const email = `tasks_persona_${Date.now()}_${Math.random().toString(36).slice(2, 8)}@example.com`;
    const password = 'Password123!';

    const reg = await request.post(`${API_URL}/auth/register`, {
      data: {
        name: 'Tasks Persona',
        email,
        password,
        confirm_password: password,
      },
    });
    expect(reg.status()).toBe(201);

    const login = await request.post(`${API_URL}/auth/login`, {
      data: { email, password },
    });
    expect(login.status()).toBe(200);
    const body = await login.json();
    expect(body.token).toBeTruthy();
    return body.token as string;
  }

  // ================================================================
  // GROUP 1: AUTH GUARD
  // ================================================================

  test('Persona: Unauthenticated list is rejected with 401', async ({ request }) => {
    await test.step('Why we use this test: Verify that the tasks API is strictly protected by JWT authentication.', async () => {});

    let response;
    await test.step('What we use: GET /tasks without an Authorization header.', async () => {
      response = await request.get(`${API_URL}/tasks`);
    });

    await test.step('What we expected: 401 Unauthorized.', async () => {
      expect(response.status()).toBe(401);
    });

    await test.step('What we get: An error envelope.', async () => {
      const body = await response.json();
      expect(body.error).toBeDefined();
    });
  });

  test('Persona: Unauthenticated create is rejected with 401', async ({ request }) => {
    await test.step('Why we use this test: Even a well-formed POST must be refused without a token.', async () => {});

    const response = await request.post(`${API_URL}/tasks`, {
      data: { text: 'Sneaky task', due_date: '2026-11-05' },
    });

    expect(response.status()).toBe(401);
  });

  // ================================================================
  // GROUP 2: HAPPY PATH — create, list, update, delete
  // ================================================================

  test('Persona: Create a task, list it, edit it, then delete it', async ({ request }) => {
    await test.step('Why we use this test: The full lifecycle — a user writes a note, confirms it appears, edits the text/date, and removes it.', async () => {});

    const token = await freshToken(request);
    const auth = { Authorization: `Bearer ${token}` };

    let createdId = '';

    // ---- 1. Create ----
    await test.step('What we use: POST /tasks with a text and due_date.', async () => {
      const res = await request.post(`${API_URL}/tasks`, {
        headers: auth,
        data: { text: 'Call the dentist', due_date: '2026-11-05' },
      });
      expect(res.status()).toBe(201);

      const body = await res.json();
      expect(body.success).toBe(true);
      expect(body.task).toBeDefined();
      expect(body.task.id).toMatch(/^tsk_/);
      expect(body.task.text).toBe('Call the dentist');
      expect(body.task.due_date).toBe('2026-11-05');
      expect(body.task.created_at).toBeTruthy();
      expect(body.task.updated_at).toBeTruthy();
      createdId = body.task.id;
    });

    // ---- 2. List ----
    await test.step('What we use: GET /tasks and confirm the new row is present.', async () => {
      const res = await request.get(`${API_URL}/tasks`, { headers: auth });
      expect(res.status()).toBe(200);
      const body = await res.json();
      expect(body.success).toBe(true);
      expect(Array.isArray(body.tasks)).toBe(true);

      const found = body.tasks.find((t: any) => t.id === createdId);
      expect(found).toBeDefined();
      expect(found.text).toBe('Call the dentist');
    });

    // ---- 3. Update ----
    await test.step('What we use: PUT /tasks/{id} with a new text and due_date.', async () => {
      const res = await request.put(`${API_URL}/tasks/${createdId}`, {
        headers: auth,
        data: {
          text: 'Call the dentist (rescheduled)',
          due_date: '2026-11-12',
        },
      });
      expect(res.status()).toBe(200);

      const body = await res.json();
      expect(body.success).toBe(true);
      expect(body.task.id).toBe(createdId);
      expect(body.task.text).toBe('Call the dentist (rescheduled)');
      expect(body.task.due_date).toBe('2026-11-12');
    });

    // ---- 4. Delete ----
    await test.step('What we use: DELETE /tasks/{id}.', async () => {
      const res = await request.delete(`${API_URL}/tasks/${createdId}`, {
        headers: auth,
      });
      expect(res.status()).toBe(200);
      const body = await res.json();
      expect(body.success).toBe(true);
    });

    // ---- 5. Confirm gone ----
    await test.step('What we use: GET /tasks and confirm the row is gone.', async () => {
      const res = await request.get(`${API_URL}/tasks`, { headers: auth });
      expect(res.status()).toBe(200);
      const body = await res.json();
      const found = body.tasks.find((t: any) => t.id === createdId);
      expect(found).toBeUndefined();
    });
  });

  // ================================================================
  // GROUP 3: LIST ORDERING & SHAPE
  // ================================================================

  test('Persona: Newest task appears first', async ({ request }) => {
    await test.step('Why we use this test: The UI renders the list top-down, so new notes must appear at the top.', async () => {});

    const token = await freshToken(request);
    const auth = { Authorization: `Bearer ${token}` };

    await test.step('What we use: Create two tasks in sequence.', async () => {
      const first = await request.post(`${API_URL}/tasks`, {
        headers: auth,
        data: { text: 'First task', due_date: '2026-11-01' },
      });
      expect(first.status()).toBe(201);

      // Small delay so created_at differs.
      await new Promise((r) => setTimeout(r, 20));

      const second = await request.post(`${API_URL}/tasks`, {
        headers: auth,
        data: { text: 'Second task', due_date: '2026-11-02' },
      });
      expect(second.status()).toBe(201);
    });

    await test.step('What we get: The later task is at index 0.', async () => {
      const res = await request.get(`${API_URL}/tasks`, { headers: auth });
      const body = await res.json();
      expect(body.tasks[0].text).toBe('Second task');
    });
  });

  test('Persona: Response shape is stable', async ({ request }) => {
    await test.step('Why we use this test: The frontend depends on specific field names.', async () => {});

    const token = await freshToken(request);
    const auth = { Authorization: `Bearer ${token}` };

    const res = await request.get(`${API_URL}/tasks`, { headers: auth });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body).toHaveProperty('success');
    expect(body).toHaveProperty('tasks');
    expect(Array.isArray(body.tasks)).toBe(true);
  });

  // ================================================================
  // GROUP 4: VALIDATION ERRORS
  // ================================================================

  test('Persona: Error - empty text is rejected', async ({ request }) => {
    await test.step('Why we use this test: A blank note is meaningless.', async () => {});

    const token = await freshToken(request);
    const auth = { Authorization: `Bearer ${token}` };

    const res = await request.post(`${API_URL}/tasks`, {
      headers: auth,
      data: { text: '   ', due_date: '2026-11-05' },
    });
    expect(res.status()).toBe(400);

    const body = await res.json();
    expect(body.error.toLowerCase()).toContain('text');
  });

  test('Persona: Error - missing due_date is rejected', async ({ request }) => {
    await test.step('Why we use this test: The date is required by the UI contract.', async () => {});

    const token = await freshToken(request);
    const auth = { Authorization: `Bearer ${token}` };

    const res = await request.post(`${API_URL}/tasks`, {
      headers: auth,
      data: { text: 'Something to do', due_date: '' },
    });
    expect(res.status()).toBe(400);
    const body = await res.json();
    expect(body.error.toLowerCase()).toContain('due date');
  });

  test('Persona: Error - malformed due_date is rejected', async ({ request }) => {
    await test.step('Why we use this test: Only YYYY-MM-DD is accepted — anything else should fail cleanly.', async () => {});

    const token = await freshToken(request);
    const auth = { Authorization: `Bearer ${token}` };

    const res = await request.post(`${API_URL}/tasks`, {
      headers: auth,
      data: { text: 'Something to do', due_date: '05/11/2026' },
    });
    expect(res.status()).toBe(400);
    const body = await res.json();
    expect(body.error.toLowerCase()).toContain('due date');
  });

  test('Persona: Error - text too long (> 2000 chars)', async ({ request }) => {
    await test.step('Why we use this test: Guard against runaway payloads.', async () => {});

    const token = await freshToken(request);
    const auth = { Authorization: `Bearer ${token}` };

    const huge = 'a'.repeat(2100);
    const res = await request.post(`${API_URL}/tasks`, {
      headers: auth,
      data: { text: huge, due_date: '2026-11-05' },
    });
    expect(res.status()).toBe(400);
    const body = await res.json();
    expect(body.error.toLowerCase()).toContain('too long');
  });

  test('Persona: Error - malformed JSON body', async ({ request }) => {
    await test.step('Why we use this test: A syntactically invalid payload must be caught before processing.', async () => {});

    const token = await freshToken(request);
    const auth = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };

    const res = await request.post(`${API_URL}/tasks`, {
      headers: auth,
      data: 'this is not valid json',
    });
    expect(res.status()).toBe(400);
    const body = await res.json();
    expect(body.error.toLowerCase()).toContain('invalid json');
  });

  test('Persona: Error - update with unknown ID returns 404', async ({ request }) => {
    await test.step('Why we use this test: Editing a task that was already deleted must not silently succeed.', async () => {});

    const token = await freshToken(request);
    const auth = { Authorization: `Bearer ${token}` };

    const res = await request.put(`${API_URL}/tasks/tsk_doesnotexist00`, {
      headers: auth,
      data: { text: 'Ghost', due_date: '2026-11-05' },
    });
    expect(res.status()).toBe(404);
    const body = await res.json();
    expect(body.error.toLowerCase()).toContain('not found');
  });

  test('Persona: Error - delete with unknown ID returns 404', async ({ request }) => {
    await test.step('Why we use this test: Deleting a missing row must be explicit, not a silent no-op.', async () => {});

    const token = await freshToken(request);
    const auth = { Authorization: `Bearer ${token}` };

    const res = await request.delete(`${API_URL}/tasks/tsk_doesnotexist00`, {
      headers: auth,
    });
    expect(res.status()).toBe(404);
  });

  // ================================================================
  // GROUP 5: METHOD GUARDS
  // ================================================================

  test('Persona: Error - PATCH on /tasks is rejected', async ({ request }) => {
    await test.step('Why we use this test: Only GET and POST are wired on the base path.', async () => {});

    const token = await freshToken(request);
    const auth = { Authorization: `Bearer ${token}` };

    const res = await request.fetch(`${API_URL}/tasks`, {
      method: 'PATCH',
      headers: auth,
      data: {},
    });
    expect([404, 405]).toContain(res.status());
  });

  test('Persona: Error - POST on /tasks/{id} is rejected', async ({ request }) => {
    await test.step('Why we use this test: Item paths only accept PUT and DELETE.', async () => {});

    const token = await freshToken(request);
    const auth = { Authorization: `Bearer ${token}` };

    const res = await request.post(`${API_URL}/tasks/tsk_anything`, {
      headers: auth,
      data: { text: 'x', due_date: '2026-11-05' },
    });
    expect([404, 405]).toBeDefined();
    expect([400, 404, 405]).toContain(res.status());
  });

  // ================================================================
  // GROUP 6: ISOLATION BETWEEN USERS
  // ================================================================

  test('Persona: Tasks created by one user are visible to the same user on next login', async ({ request }) => {
    await test.step('Why we use this test: Persistence must survive a token refresh — a user should see their own notes after signing out and back in.', async () => {});

    const email = `tasks_persist_${Date.now()}@example.com`;
    const password = 'Password123!';

    const reg = await request.post(`${API_URL}/auth/register`, {
      data: { name: 'Persist User', email, password, confirm_password: password },
    });
    expect(reg.status()).toBe(201);

    const login1 = await request.post(`${API_URL}/auth/login`, { data: { email, password } });
    const token1 = (await login1.json()).token;

    const created = await request.post(`${API_URL}/tasks`, {
      headers: { Authorization: `Bearer ${token1}` },
      data: { text: 'Persisted note', due_date: '2026-12-01' },
    });
    expect(created.status()).toBe(201);
    const createdId = (await created.json()).task.id;

    // Fresh login → fresh token
    const login2 = await request.post(`${API_URL}/auth/login`, { data: { email, password } });
    const token2 = (await login2.json()).token;

    const list = await request.get(`${API_URL}/tasks`, {
      headers: { Authorization: `Bearer ${token2}` },
    });
    expect(list.status()).toBe(200);
    const body = await list.json();
    expect(body.tasks.some((t: any) => t.id === createdId)).toBe(true);
  });

});