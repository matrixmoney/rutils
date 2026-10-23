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

  // ----------------------------------------------------------------
  // Helper: register + login a fresh user, return both token AND
  // their generated user ID. Needed by the isolation tests below,
  // which must assert on `task.user_id`.
  // ----------------------------------------------------------------
  async function freshUser(request: any): Promise<{ token: string; userId: string; email: string }> {
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
    const regBody = await reg.json();
    expect(regBody.id).toBeTruthy();

    const login = await request.post(`${API_URL}/auth/login`, {
      data: { email, password },
    });
    expect(login.status()).toBe(200);
    const loginBody = await login.json();
    expect(loginBody.token).toBeTruthy();

    return { token: loginBody.token as string, userId: regBody.id as string, email };
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
  // GROUP 2b: OWNERSHIP STAMPING (NEW)
  // ================================================================

  test('Persona: Created task carries the caller\'s user_id as its owner', async ({ request }) => {
    await test.step('Why we use this test: The scoping fix hinges on every task carrying a user_id stamped from the JWT. If create silently left it blank, the whole isolation model collapses.', async () => {});

    const { token, userId } = await freshUser(request);
    const auth = { Authorization: `Bearer ${token}` };

    let createdTask: any;

    await test.step('What we use: POST /tasks as a freshly-registered user.', async () => {
      const res = await request.post(`${API_URL}/tasks`, {
        headers: auth,
        data: { text: 'Ownership check', due_date: '2026-11-05' },
      });
      expect(res.status()).toBe(201);
      createdTask = (await res.json()).task;
    });

    await test.step('What we expected: response.task.user_id equals the caller\'s registration ID.', async () => {
      expect(createdTask).toBeDefined();
      expect(createdTask.user_id).toBe(userId);
    });

    await test.step('What we get: GET /tasks returns the same task with the same owner.', async () => {
      const list = await request.get(`${API_URL}/tasks`, { headers: auth });
      expect(list.status()).toBe(200);
      const body = await list.json();
      const found = body.tasks.find((t: any) => t.id === createdTask.id);
      expect(found).toBeDefined();
      expect(found.user_id).toBe(userId);
    });

    await test.step('Why it got this output: the handler extracts claims.UserID from the JWT and passes it to Store.Create, which stamps it on the record.', async () => {});
  });

  test('Persona: Client cannot forge user_id via the request body', async ({ request }) => {
    await test.step('Why we use this test: A malicious client might try to set user_id themselves. The request struct has no such field, so any extra key must be silently ignored — and the stamped owner must still be the caller.', async () => {});

    const { token, userId } = await freshUser(request);
    const auth = { Authorization: `Bearer ${token}` };

    let createdTask: any;

    await test.step('What we use: POST /tasks with an extra "user_id" key set to a fake value.', async () => {
      const res = await request.post(`${API_URL}/tasks`, {
        headers: auth,
        data: {
          text: 'Forgery attempt',
          due_date: '2026-11-05',
          user_id: 'usr_victim_account', // ← attacker-controlled, must be ignored
        },
      });
      expect(res.status()).toBe(201);
      createdTask = (await res.json()).task;
    });

    await test.step('What we expected: the stored owner is the CALLER, not the forged value.', async () => {
      expect(createdTask.user_id).toBe(userId);
      expect(createdTask.user_id).not.toBe('usr_victim_account');
    });

    await test.step('Why it got this output: Go\'s encoding/json silently drops unknown struct fields, and the handler stamps UserID from the JWT claims — never from the body.', async () => {});
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

  test('Persona: Every task in the list carries a non-empty user_id', async ({ request }) => {
    await test.step('Why we use this test: Legacy pre-scoping rows had no owner. Post-migration, every visible task must carry a user_id — otherwise the frontend export would show blank owner columns.', async () => {});

    const token = await freshToken(request);
    const auth = { Authorization: `Bearer ${token}` };

    await request.post(`${API_URL}/tasks`, {
      headers: auth,
      data: { text: 'Shape check', due_date: '2026-11-05' },
    });

    const res = await request.get(`${API_URL}/tasks`, { headers: auth });
    expect(res.status()).toBe(200);
    const body = await res.json();

    expect(body.tasks.length).toBeGreaterThan(0);
    for (const t of body.tasks) {
      expect(typeof t.user_id).toBe('string');
      expect(t.user_id.length).toBeGreaterThan(0);
    }
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
    expect([400, 404, 405]).toContain(res.status());
  });

  // ================================================================
  // GROUP 6: PERSISTENCE ACROSS LOGINS
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

  // ================================================================
  // GROUP 6b: USER ISOLATION (NEW)
  // ================================================================

  test('Persona: Tasks created by one user are NOT visible to another user', async ({ request }) => {
    await test.step('Why we use this test: the entire point of the scoping fix — a task must be visible ONLY to its owner.', async () => {});

    // --- User A: register, login, create a task ---
    const alice = await freshUser(request);
    const authA = { Authorization: `Bearer ${alice.token}` };

    const created = await request.post(`${API_URL}/tasks`, {
      headers: authA,
      data: { text: "Alice's private note", due_date: '2026-11-05' },
    });
    expect(created.status()).toBe(201);
    const aliceTaskId = (await created.json()).task.id as string;
    expect(aliceTaskId).toMatch(/^tsk_/);

    // --- User B: register, login ---
    const bob = await freshUser(request);
    const authB = { Authorization: `Bearer ${bob.token}` };

    // --- Bob's list must not contain Alice's task ---
    await test.step("What we use: GET /tasks as Bob and confirm Alice's task is invisible.", async () => {
      const listB = await request.get(`${API_URL}/tasks`, { headers: authB });
      expect(listB.status()).toBe(200);
      const bodyB = await listB.json();
      expect(bodyB.success).toBe(true);
      expect(Array.isArray(bodyB.tasks)).toBe(true);
      expect(bodyB.tasks.some((t: any) => t.id === aliceTaskId)).toBe(false);

      // Bonus: Bob must not see any task whose user_id !== his own.
      for (const t of bodyB.tasks) {
        expect(t.user_id).toBe(bob.userId);
      }
    });

    // --- Bob cannot update Alice's task ---
    await test.step("What we use: PUT Alice's task as Bob → expect 404.", async () => {
      const update = await request.put(`${API_URL}/tasks/${aliceTaskId}`, {
        headers: authB,
        data: { text: 'Hijacked', due_date: '2027-01-01' },
      });
      expect(update.status()).toBe(404);
    });

    // --- Bob cannot delete Alice's task ---
    await test.step("What we use: DELETE Alice's task as Bob → expect 404.", async () => {
      const del = await request.delete(`${API_URL}/tasks/${aliceTaskId}`, {
        headers: authB,
      });
      expect(del.status()).toBe(404);
    });

    // --- Alice's task survived, unchanged ---
    await test.step("What we use: GET /tasks as Alice and confirm the task survived and is intact.", async () => {
      const listA = await request.get(`${API_URL}/tasks`, { headers: authA });
      expect(listA.status()).toBe(200);
      const bodyA = await listA.json();
      const found = bodyA.tasks.find((t: any) => t.id === aliceTaskId);
      expect(found).toBeDefined();
      expect(found.text).toBe("Alice's private note");
      expect(found.due_date).toBe('2026-11-05');
      expect(found.user_id).toBe(alice.userId);
    });

    await test.step('Why it got this output: the backend extracts the caller\'s user_id from the JWT and filters/checks it at the store layer for every operation.', async () => {});
  });

  test('Persona: Two users\' lists are disjoint even when both have tasks', async ({ request }) => {
    await test.step('Why we use this test: The strongest isolation check — both users have data, and neither list contains a single task belonging to the other.', async () => {});

    const alice = await freshUser(request);
    const bob = await freshUser(request);
    const authA = { Authorization: `Bearer ${alice.token}` };
    const authB = { Authorization: `Bearer ${bob.token}` };

    // Each user creates 2 tasks.
    const aIds: string[] = [];
    const bIds: string[] = [];

    for (let i = 0; i < 2; i++) {
      const ra = await request.post(`${API_URL}/tasks`, {
        headers: authA,
        data: { text: `Alice #${i + 1}`, due_date: '2026-11-05' },
      });
      expect(ra.status()).toBe(201);
      aIds.push((await ra.json()).task.id);

      const rb = await request.post(`${API_URL}/tasks`, {
        headers: authB,
        data: { text: `Bob #${i + 1}`, due_date: '2026-11-05' },
      });
      expect(rb.status()).toBe(201);
      bIds.push((await rb.json()).task.id);
    }

    // Fetch both lists.
    const listA = await request.get(`${API_URL}/tasks`, { headers: authA });
    const listB = await request.get(`${API_URL}/tasks`, { headers: authB });
    expect(listA.status()).toBe(200);
    expect(listB.status()).toBe(200);

    const bodyA = await listA.json();
    const bodyB = await listB.json();

    const aSet = new Set(bodyA.tasks.map((t: any) => t.id));
    const bSet = new Set(bodyB.tasks.map((t: any) => t.id));

    // All of Alice's tasks are in her list.
    for (const id of aIds) expect(aSet.has(id)).toBe(true);
    // All of Bob's tasks are in his list.
    for (const id of bIds) expect(bSet.has(id)).toBe(true);

    // Zero overlap between the two ID sets.
    for (const id of aIds) expect(bSet.has(id)).toBe(false);
    for (const id of bIds) expect(aSet.has(id)).toBe(false);

    // Every task in Alice's list belongs to Alice.
    for (const t of bodyA.tasks) expect(t.user_id).toBe(alice.userId);
    // Every task in Bob's list belongs to Bob.
    for (const t of bodyB.tasks) expect(t.user_id).toBe(bob.userId);
  });

  // ================================================================
  // GROUP 8: CROSS-USER MUTATION GUARDS (NEW)
  // ================================================================

  test('Persona: Cross-user DELETE with a KNOWN but not-owned ID returns 404', async ({ request }) => {
    await test.step('Why we use this test: Guards against a subtle regression where Delete might accidentally skip the ownership check for valid IDs. The ID IS real — but it is not the caller\'s. The response must still be 404, not 200 or 403.', async () => {});

    const alice = await freshUser(request);
    const bob = await freshUser(request);
    const authA = { Authorization: `Bearer ${alice.token}` };
    const authB = { Authorization: `Bearer ${bob.token}` };

    // Alice creates a task.
    const created = await request.post(`${API_URL}/tasks`, {
      headers: authA,
      data: { text: 'Do not delete me', due_date: '2026-11-05' },
    });
    expect(created.status()).toBe(201);
    const taskId = (await created.json()).task.id;

    // Bob tries to delete it.
    const del = await request.delete(`${API_URL}/tasks/${taskId}`, { headers: authB });
    expect(del.status()).toBe(404);

    // Sanity: the task still exists for Alice.
    const listA = await request.get(`${API_URL}/tasks`, { headers: authA });
    const bodyA = await listA.json();
    expect(bodyA.tasks.some((t: any) => t.id === taskId)).toBe(true);
  });

  test('Persona: Cross-user UPDATE with a KNOWN but not-owned ID returns 404', async ({ request }) => {
    await test.step('Why we use this test: Mirror of the previous test for PUT. Bob cannot modify Alice\'s task text or due date, and Alice\'s task must be byte-identical after the attempt.', async () => {});

    const alice = await freshUser(request);
    const bob = await freshUser(request);
    const authA = { Authorization: `Bearer ${alice.token}` };
    const authB = { Authorization: `Bearer ${bob.token}` };

    // Alice creates a task.
    const created = await request.post(`${API_URL}/tasks`, {
      headers: authA,
      data: { text: 'Original text', due_date: '2026-11-05' },
    });
    expect(created.status()).toBe(201);
    const taskId = (await created.json()).task.id;
    const originalCreatedAt = (await created.json()).task.created_at;

    // Bob tries to update it.
    const update = await request.put(`${API_URL}/tasks/${taskId}`, {
      headers: authB,
      data: { text: 'Hijacked text', due_date: '2099-01-01' },
    });
    expect(update.status()).toBe(404);

    // Sanity: Alice's task is unchanged.
    const listA = await request.get(`${API_URL}/tasks`, { headers: authA });
    const bodyA = await listA.json();
    const found = bodyA.tasks.find((t: any) => t.id === taskId);
    expect(found).toBeDefined();
    expect(found.text).toBe('Original text');
    expect(found.due_date).toBe('2026-11-05');
    expect(found.created_at).toBe(originalCreatedAt);
  });

  test('Persona: Cross-user GET /tasks/{id} — no direct endpoint exists, but list is the surface', async ({ request }) => {
    await test.step('Why we use this test: There is no GET /tasks/{id} endpoint — the list is the only read surface. Verify that Bob\'s list, filtered by Alice\'s known task ID, yields nothing.', async () => {});

    const alice = await freshUser(request);
    const bob = await freshUser(request);
    const authA = { Authorization: `Bearer ${alice.token}` };
    const authB = { Authorization: `Bearer ${bob.token}` };

    // Alice creates a task.
    const created = await request.post(`${API_URL}/tasks`, {
      headers: authA,
      data: { text: 'Only Alice sees this', due_date: '2026-11-05' },
    });
    expect(created.status()).toBe(201);
    const taskId = (await created.json()).task.id;

    // Bob fetches his list — Alice's task must not be there.
    const listB = await request.get(`${API_URL}/tasks`, { headers: authB });
    expect(listB.status()).toBe(200);
    const bodyB = await listB.json();
    const found = bodyB.tasks.find((t: any) => t.id === taskId);
    expect(found).toBeUndefined();
  });
});