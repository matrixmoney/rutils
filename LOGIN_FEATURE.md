# Authentication & Login Feature Documentation (`LOGIN_FEATURE.md`)

This document provides a comprehensive developer guide for the Authentication System implemented in the Rutils utility platform.

---

## 1. Architectural Overview

The authentication system is completely isolated and decoupled from existing utility services (file conversion, text extraction, calculations). It provides:
- **User Persistence**: File-based JSON storage in `data/users.json` with thread safety (`sync.RWMutex`) and atomic file replacement.
- **Credential Security**: Passwords hashed using `bcrypt` at cost factor `12`.
- **Session Management**: Stateless JSON Web Tokens (JWT) signed with HMAC-SHA256 (`HS256`).
- **Google SMTP Integration**: Automated dispatching of temporary passwords, security notices, and task event notifications (created / updated / deleted) with zero hardcoded credentials and an automatic console-logging development fallback when credentials are not configured.
- **Swagger Documentation**: OpenAPI 2.0 annotations integrated into Swagger UI with Bearer authentication support.

---

## 2. Configuration (`config.json`)

Configuration is stored in `config.json` at the project root. Never commit real credentials to version control.

### Default Structure:
```json
{
  "smtp_email": "",
  "smtp_app_password": "",
  "smtp_host": "smtp.gmail.com",
  "smtp_port": "587",
  "jwt_secret": "rutils-utility-jwt-secret-change-in-production-2026",
  "jwt_expiration_hours": 24,
  "data_dir": "data"
}
```

### Configuration Parameters:
| Key | Type | Description | Default Fallback |
| :--- | :--- | :--- | :--- |
| `smtp_email` | string | Google Account email address | `""` (Dev Fallback active) |
| `smtp_app_password` | string | 16-character Google App Password | `""` (Dev Fallback active) |
| `smtp_host` | string | SMTP server hostname | `"smtp.gmail.com"` |
| `smtp_port` | string | SMTP server port with STARTTLS | `"587"` |
| `smtp_sender_name` | string | Friendly display sender name | `"Rutils Team"` |
| `jwt_secret` | string | Secret key used to sign and verify JWT tokens | `"rutils-utility-jwt-secret-change-in-production-2026"` |
| `jwt_expiration_hours`| int | Lifetime of generated JWT tokens | `24` |
| `data_dir` | string | Directory containing JSON storage files | `"data"` |

### How to Configure Google SMTP:
1. Log in to your Google Account and navigate to **Security** ([myaccount.google.com/security](https://myaccount.google.com/security)).
2. Ensure **2-Step Verification** is turned **ON**.
3. Under "How you sign in to Google", select **App passwords** (or search "App passwords" in the search bar).
4. Create a new App password named `Rutils`.
5. Google will generate a 16-character password (e.g. `abcd efgh ijkl mnop`).
6. Copy the password and set it in `config.json`:
   ```json
   "smtp_email": "your-email@gmail.com",
   "smtp_app_password": "abcdefghijklmnop"
   ```

### Development / Local Mock Mode:
When `smtp_email` or `smtp_app_password` are left empty, the server automatically enters **Development Fallback Mode**:
- Emails are not dispatched over the network.
- The system logs the generated temporary password (and every other simulated email) directly to the console:
  ```text
  [AUTH-DEV] SMTP not configured in config.json. Temporary password for <user@example.com> is: Xy9#mK!28zQ1
  ```
- All API requests succeed smoothly, enabling offline development and testing.

---

## 3. Storage (`data/users.json`)

User accounts are stored in `data/users.json`. All read and write operations are synchronized with a read-write mutex (`sync.RWMutex`). Writes are performed **atomically** by writing to a temporary file (`users-*.tmp`) and renaming it to `data/users.json`, preventing data corruption or race conditions.

### User Schema:
```json
[
  {
    "id": "usr_94e5088c4b18cb6a",
    "name": "Jane Doe",
    "email": "jane@example.com",
    "password_hash": "$2a$12$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy",
    "temp_password_hash": "",
    "temp_password_expiry": null,
    "must_reset_password": false,
    "created_at": "2026-09-29T12:00:00Z",
    "updated_at": "2026-09-29T12:00:00Z"
  }
]
```

---

## 4. Password Security & Validation Rules

All user passwords must satisfy the following strict complexity requirements:
- **Minimum Length**: 8 characters
- **Uppercase Letter**: At least 1 (`A-Z`)
- **Lowercase Letter**: At least 1 (`a-z`)
- **Number**: At least 1 (`0-9`)
- **Special Character**: At least 1 (`!@#$%^&*()-_=+[]{}|;:'",.<>/?~`\``)

### Password Hashing:
- Passwords are encrypted using **bcrypt** with cost factor `12`.
- Plaintext passwords are never stored or returned in any API responses.

---

## 5. JWT Authentication Flow

1. Upon successful login (`POST /auth/login`), the server issues a signed JWT token:
   ```json
   {
     "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
     "user": {
       "id": "usr_94e5088c4b18cb6a",
       "name": "Jane Doe",
       "email": "jane@example.com",
       "must_reset_password": false,
       "created_at": "2026-09-29T12:00:00Z"
     },
     "must_reset_password": false,
     "message": "Login successful"
   }
   ```
2. For protected endpoints (`/auth/reset-password`, `/auth/change-password`, `/auth/me`, `/tasks`, `/tasks/{id}`, `/nifty50/companies`), include the token in the HTTP `Authorization` header:
   ```http
   Authorization: Bearer <YOUR_JWT_TOKEN>
   ```
3. **Forced Reset Workflow**: If a user logs in using a temporary password issued via `/auth/forgot-password`:
   - `must_reset_password` is set to `true` in the JWT claims and login response.
   - The user must call `POST /auth/reset-password` with their new password before continuing normal operations.
4. **Embedded Claims**: every signed token carries `user_id`, `email`, `name`, and `must_reset_password`. The `name` claim is used by downstream handlers (e.g. the tasks email dispatcher) to personalise outgoing emails without a second user-store lookup. The `user_id` claim is the authoritative owner for tasks and is enforced by every task endpoint (see **Section 7b**).

---

## 6. Endpoints Reference

### 1. Register User
- **Route**: `POST /auth/register`
- **Access**: Public
- **Request Body**:
  ```json
  {
    "name": "Jane Doe",
    "email": "jane@example.com",
    "password": "StrongPassword!2026",
    "confirm_password": "StrongPassword!2026"
  }
  ```
- **Responses**:
  - `201 Created`:
    ```json
    {
      "id": "usr_94e5088c4b18cb6a",
      "name": "Jane Doe",
      "email": "jane@example.com",
      "must_reset_password": false,
      "created_at": "2026-09-29T12:00:00Z"
    }
    ```
  - `400 Bad Request`: Validation failure (e.g. password mismatch or weak password).
  - `409 Conflict`: Email is already registered.

### 2. Login User
- **Route**: `POST /auth/login`
- **Access**: Public
- **Request Body**:
  ```json
  {
    "email": "jane@example.com",
    "password": "StrongPassword!2026"
  }
  ```
- **Responses**:
  - `200 OK`: Returns JWT token and user profile.
  - `401 Unauthorized`: Invalid email or password.

### 3. Logout User
- **Route**: `POST /auth/logout`
- **Access**: Public
- **Responses**:
  - `200 OK`:
    ```json
    {
      "success": true,
      "message": "Logged out successfully"
    }
    ```

### 4. Forgot Password
- **Route**: `POST /auth/forgot-password`
- **Access**: Public
- **Request Body**:
  ```json
  {
    "email": "jane@example.com"
  }
  ```
- **Responses**:
  - `200 OK`:
    ```json
    {
      "success": true,
      "message": "A temporary password has been dispatched to your email address."
    }
    ```
  - `404 Not Found`: No account registered with that email.

### 5. Reset Password (Forced)
- **Route**: `POST /auth/reset-password`
- **Access**: Authenticated (`Authorization: Bearer <token>`)
- **Request Body**:
  ```json
  {
    "new_password": "BrandNewPassword#999",
    "confirm_password": "BrandNewPassword#999"
  }
  ```
- **Responses**:
  - `200 OK`:
    ```json
    {
      "success": true,
      "message": "Permanent password updated successfully. You can now use your new password."
    }
    ```
  - `400 Bad Request`: Password mismatch or does not meet complexity rules.
  - `401 Unauthorized`: Missing or invalid Bearer token.

### 6. Change Password (Logged-in User)
- **Route**: `POST /auth/change-password`
- **Access**: Authenticated (`Authorization: Bearer <token>`)
- **Request Body**:
  ```json
  {
    "current_password": "CurrentPassword!2026",
    "new_password": "BrandNewPassword#999",
    "confirm_password": "BrandNewPassword#999"
  }
  ```
- **Responses**:
  - `200 OK`:
    ```json
    {
      "success": true,
      "message": "Password changed successfully. A confirmation email has been dispatched."
    }
    ```
  - `401 Unauthorized`: Current password incorrect or missing token.
  - `400 Bad Request`: Password mismatch or weak new password.

### 7. Get Current User Profile
- **Route**: `GET /auth/me`
- **Access**: Authenticated (`Authorization: Bearer <token>`)
- **Responses**:
  - `200 OK`: Returns user profile without password hashes.
  - `401 Unauthorized`: Invalid or expired token.

### 8. Create Task (with optional email notification)
- **Route**: `POST /tasks`
- **Access**: Authenticated (`Authorization: Bearer <token>`)
- **Request Body**:
  ```json
  {
    "text": "Buy groceries for the weekend",
    "due_date": "2026-11-05",
    "send_email": true
  }
  ```
  | Field | Type | Required | Description |
  | :--- | :--- | :--- | :--- |
  | `text` | string | Yes | Task description (max 2000 chars). |
  | `due_date` | string | Yes | `YYYY-MM-DD` format. |
  | `send_email` | boolean | No | `true` → dispatch "Task Created" email. `false` → silent save. Omitted → defaults to `true` (backwards-compatible with the pre-flag behaviour). |
  > **Note:** the request body has **no `user_id` field** by design. The owner is derived from the JWT. Clients cannot create tasks on behalf of another user.
- **Responses**:
  - `201 Created`:
    ```json
    {
      "success": true,
      "task": {
        "id": "tsk_f1cecc9ba262012e",
        "user_id": "usr_94e5088c4b18cb6a",
        "text": "Buy groceries for the weekend",
        "due_date": "2026-11-05",
        "created_at": "2026-10-22T12:00:00Z",
        "updated_at": "2026-10-22T12:00:00Z"
      },
      "email_sent": true
    }
    ```
  - `400 Bad Request`: Validation failure (empty text, missing/invalid due_date, or text > 2000 chars).
  - `401 Unauthorized`: Missing or invalid Bearer token.
- **Email behaviour**: When `send_email` is true (or omitted), the backend dispatches a "Task Created" confirmation email. `email_sent` reflects whether the dispatch succeeded. The task is always saved regardless of email outcome. See **Section 7 — Task Email Notifications** below for full details.
- **Ownership**: The task is stamped with the caller's `user_id` at creation. See **Section 7b — Task Ownership & User Scoping** for the complete enforcement model.

### 9. List Tasks
- **Route**: `GET /tasks`
- **Access**: Authenticated (`Authorization: Bearer <token>`)
- **Responses**:
  - `200 OK`: Returns only the tasks owned by the caller, newest first.
    ```json
    {
      "success": true,
      "tasks": [
        {
          "id": "tsk_f1cecc9ba262012e",
          "user_id": "usr_94e5088c4b18cb6a",
          "text": "Buy groceries for the weekend",
          "due_date": "2026-11-05",
          "created_at": "2026-10-22T12:00:00Z",
          "updated_at": "2026-10-22T12:00:00Z"
        }
      ]
    }
    ```
  - `401 Unauthorized`: Missing or invalid Bearer token.

### 10. Update Task (with optional email notification)
- **Route**: `PUT /tasks/{id}`
- **Access**: Authenticated (`Authorization: Bearer <token>`)
- **Request Body**:
  ```json
  {
    "text": "Buy groceries (rescheduled)",
    "due_date": "2026-11-12",
    "send_email": true
  }
  ```
  | Field | Type | Required | Description |
  | :--- | :--- | :--- | :--- |
  | `text` | string | Yes | Updated task description. |
  | `due_date` | string | Yes | `YYYY-MM-DD` format. |
  | `send_email` | boolean | No | `true` → dispatch "Task Updated" email with old→new diff. `false` → silent save. Omitted → defaults to `false` (backwards-compatible with the pre-flag behaviour). |
- **Responses**:
  - `200 OK`: Returns the updated task and `email_sent`.
  - `400 Bad Request`: Validation error.
  - `401 Unauthorized`: Missing or invalid Bearer token.
  - `404 Not Found`: The task does not exist **or is not owned by the caller** (indistinguishable by design).
- **Email behaviour**: When `send_email` is true, the backend loads the task's pre-edit state from the store, applies the update, and dispatches a "Task Updated" email showing the old values (strikethrough) above the new values. The "old" side is captured server-side and cannot be forged by the client. `email_sent` reflects whether the dispatch succeeded.

### 11. Delete Task (with optional email notification)
- **Route**: `DELETE /tasks/{id}?send_email=true`
- **Access**: Authenticated (`Authorization: Bearer <token>`)
- **Query Parameters**:
  | Param | Type | Required | Description |
  | :--- | :--- | :--- | :--- |
  | `send_email` | string | No | `"true"` or `"1"` → dispatch "Task Deleted" email. `"false"`, `"0"`, `"no"`, `"off"` → silent delete. Omitted → defaults to `"true"` (delete is destructive, so we notify by default). |
  > **Note:** the flag goes in the **query string**, not the body — some HTTP clients and proxies strip request bodies on DELETE.
- **Responses**:
  - `200 OK`:
    ```json
    {
      "success": true,
      "message": "Task deleted successfully",
      "email_sent": true
    }
    ```
  - `401 Unauthorized`: Missing or invalid Bearer token.
  - `404 Not Found`: The task does not exist **or is not owned by the caller** (indistinguishable by design).
- **Email behaviour**: When `send_email` is true (or omitted), the backend captures the deleted task's text and due date before removal and dispatches a "Task Deleted" email. `email_sent` reflects whether the dispatch succeeded. The task is always deleted regardless of email outcome.

---

## 7. Task Email Notifications

The Tasks feature dispatches confirmation emails for **three events**: task creation, task update, and task deletion. Each event has its own template in `internal/auth/email.go`, and each event is gated by a `send_email` flag that the client controls.

**Trigger:** the user clicks one of the action buttons in the Tasks view:

| UI button | API call | `send_email` | Email fires? |
|---|---|---|---|
| **Add** (new task card) | `POST /tasks` | `false` | ❌ |
| **Email** (new task card) | `POST /tasks` | `true` | ✅ Task Created |
| **Update** (edit card) | `PUT /tasks/{id}` | `false` | ❌ |
| **Email** (edit card) | `PUT /tasks/{id}` | `true` | ✅ Task Updated |
| **Delete** (task card) | `DELETE /tasks/{id}?send_email=true` | `true` | ✅ Task Deleted |

### The three email templates

| Event | Subject prefix | Banner colour | Body content |
|---|---|---|---|
| **Created** | `📌 Task Created: "…"` | Indigo gradient | Task text + due date |
| **Updated** | `✏️ Task Updated: "…"` | Teal gradient | Old text (strikethrough) + old due date above new text + new due date |
| **Deleted** | `🗑️ Task Deleted: "…"` | Red gradient | Deleted task text (strikethrough) + due date, plus a "if this wasn't you, contact support" notice |

All three share the same visual language (dark banner, "Rutils Tasks" branding, monospace due-date pill, Rutils footer) and are dual-body (plain-text + HTML) so they render correctly in every client.

**Personalisation source:** the greeting uses the `Name` claim embedded in the user's JWT (see Section 5). Every login signs a token that carries `UserID`, `Email`, and `Name`, so the tasks handler does not need a second user-store lookup to personalise the email.

**The "old" side of the update email is captured server-side.** `TasksHandler.Update` calls `service.GetByID` *before* applying the edit, so the "before" snapshot reflects what was actually on disk a moment ago. A malicious client cannot forge the "old" values by lying in the request body.

### Best-effort semantics

Emails are dispatched **after** the state change is persisted, and a dispatch failure **never** turns a successful save/update/delete into an error response. The API always returns 200/201 with the operation result, and the response body distinguishes the two outcomes via `email_sent`:

| Response field | Meaning |
|---|---|
| `email_sent: true`  | The operation succeeded and the notification email was dispatched (or the dev fallback logged it). |
| `email_sent: false` | The operation succeeded, but the email could not be sent. The frontend surfaces this as a red "…could not be sent" toast. |

**Timeout guard:** every dispatch is wrapped in a 10-second `select` in `dispatchTaskEmail`, `dispatchTaskUpdatedEmail`, and `dispatchTaskDeletedEmail` (`internal/api/tasks_handler.go`). If `smtp.SendMail` hangs beyond 10 seconds, the HTTP response still returns promptly — `email_sent: false` — while the stuck goroutine exits on its own once the underlying `net.DialTimeout` (15 s) fires. This prevents a slow SMTP server from creating a backlog of hanging HTTP requests.

### Dev fallback

When `smtp_email` or `smtp_app_password` are empty in `config.json`, every `Send*Notification` method logs a single line to stdout and returns `nil` — meaning `email_sent` comes back as `true` locally, so the UX stays testable without real SMTP credentials:

```text
[AUTH-DEV] SMTP not configured in config.json. Task notification for <user@example.com> simulated: text="Buy groceries" due="2026-11-05"
[tasks] email dispatched for task tsk_xxxx to <user@example.com>

[AUTH-DEV] SMTP not configured in config.json. Task update notification for <user@example.com> simulated: old_text="Buy groceries" old_due="2026-11-05" new_text="Buy groceries (rescheduled)" new_due="2026-11-12"
[tasks] update email dispatched for task tsk_xxxx to <user@example.com>

[AUTH-DEV] SMTP not configured in config.json. Task delete notification for <user@example.com> simulated: text="Buy groceries" due="2026-11-05"
[tasks] delete email dispatched for task tsk_xxxx to <user@example.com>
```

### Summary — which endpoint sends which email

| Endpoint | Sends email? | Template | Default when flag omitted |
|---|---|---|---|
| `POST /auth/forgot-password` | ✅ | Temporary password | (no flag — always sends) |
| `POST /auth/change-password` | ✅ | Password-changed confirmation | (no flag — always sends) |
| `POST /auth/reset-password` | ❌ | — | — |
| `POST /tasks` | ⚙️ conditional | Task Created | `send_email: true` |
| `PUT /tasks/{id}` | ⚙️ conditional | Task Updated | `send_email: false` |
| `DELETE /tasks/{id}` | ⚙️ conditional | Task Deleted | `?send_email=true` |

⚙️ = controlled by the `send_email` flag.

### Code locations

- Email templates + senders: `internal/auth/email.go`
  - `SendTaskNotification(toEmail, userName, taskText, dueDate)`
  - `SendTaskUpdatedNotification(toEmail, userName, oldText, oldDueDate, newText, newDueDate)`
  - `SendTaskDeletedNotification(toEmail, userName, taskText, dueDate)`
- Handlers that trigger them: `internal/api/tasks_handler.go`
  - `Create` + `dispatchTaskEmail`
  - `Update` + `dispatchTaskUpdatedEmail`
  - `Delete` + `dispatchTaskDeletedEmail`
- Request/response fields: `internal/tasks/models.go`
  - `CreateTaskRequest.SendEmail *bool`
  - `UpdateTaskRequest.SendEmail *bool`
  - `TaskResponse.EmailSent bool`
  - `MessageResponse.EmailSent bool`

For the full UI flow (button layout, toast matrix, request-parameter order), see **Section 7c** below.

---

## 7b. Task Ownership & User Scoping

Every task is **owned by exactly one user**. The owner is set from the authenticated JWT at creation time and can never be forged by the client.

### How ownership is enforced

| Layer | Responsibility |
|---|---|
| `internal/auth/jwt.go` | Signs the `user_id` claim into every token. |
| `internal/api/tasks_handler.go` | Extracts `claims.UserID` from the request context and passes it to the service. A missing claim returns **401 Unauthorized**. |
| `internal/tasks/service.go` | Rejects an empty `userID` defensively (belt-and-braces against a handler bug). |
| `internal/tasks/store.go` | Filters by `UserID` on `ListByUser`; checks it on `Update`/`Delete` before mutating. |
| `internal/tasks/models.go` | The `CreateTaskRequest` / `UpdateTaskRequest` structs deliberately have **no `user_id` field** — clients cannot set the owner. |

### Cross-user access behaviour

A task that exists but belongs to another user responds **exactly as if it did not exist**:

- `PUT /tasks/{id}` → **404 Not Found**
- `DELETE /tasks/{id}` → **404 Not Found**
- `GET /tasks` → the task simply does not appear in the list

Returning **404 instead of 403** is a deliberate security decision. It prevents a malicious user from enumerating valid task IDs by probing for `403 Forbidden` responses.

### Legacy task migration

Tasks stored in `data/tasks.json` **before** the scoping feature have no `user_id`. They are:

- **Preserved on disk** — never deleted.
- **Invisible to every user** — `ListByUser` requires an exact match against a non-empty owner.
- **Counted and logged** at server startup:

  ```text
  [tasks] 3 legacy task(s) with no owner detected; they will not appear in any user's list
  ```

If you need to recover a legacy task, edit `data/tasks.json` directly and add a `"user_id": "usr_..."` value matching a real account.

### Frontend Task Export

The **Export** button in the Tasks view lets a user download their own saved tasks as **CSV** or **JSON**.

- **Where it lives**: top-right of the My Tasks header, next to the Refresh button. Disabled when there are no saved tasks.
- **Where the data comes from**: the in-memory `allTasks` array, which was populated by `GET /tasks` — so it is already user-scoped by the backend.
- **What the CSV contains**: columns `id, text, due_date, created_at, updated_at`. Cells are RFC 4180-quoted, and any cell starting with `= + - @` is prefixed with a single quote to prevent spreadsheet formula injection.
- **Filename**: `my-tasks-YYYY-MM-DD.csv` or `my-tasks-YYYY-MM-DD.json`.
- **No server round-trip**: the file is generated entirely in the browser. If a server-side export is ever needed (e.g. scheduled email of tasks as CSV), a `GET /tasks/export` endpoint can be added later without changing the current client behaviour.

---

## 7c. Task Notifications — the Full Picture (UI + API)

This section is the one-page reference for "which button does what". It documents both the frontend behaviour and the corresponding API calls, so that anyone debugging a "my email never arrived" ticket can trace the whole path in one place.

### 7c.1 — The UI flows

**New task card** (top of the Tasks view):

```
┌─────────────────────────────────────────────────────────────┐
│  Write your task                                            │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  (textarea)                                           │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  Due date: [2026-11-05]                 [ + Add ] [ ✉ Email ]│
└─────────────────────────────────────────────────────────────┘
```

- **`Add`** → `POST /tasks` with `send_email: false` → silent save.
- **`Email`** → `POST /tasks` with `send_email: true` → save + Task Created email.

**Edit card** (appears in-place when you click `Edit` on a task):

```
┌─────────────────────────────────────────────────────────────┐
│  EDITING TASK                                               │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  (textarea — pre-filled with task text)               │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  Due date: [2026-11-12]     [ Cancel ] [ Update ] [ ✉ Email ]│
└─────────────────────────────────────────────────────────────┘
```

- **`Cancel`** → discard edits, no API call.
- **`Update`** → `PUT /tasks/{id}` with `send_email: false` → silent save.
- **`Email`** → `PUT /tasks/{id}` with `send_email: true` → save + Task Updated email.

**Delete** (the red trash icon on any saved task):

```
Delete this task?   [Cancel] [OK]
```

- **`OK`** → `DELETE /tasks/{id}?send_email=true` → delete + Task Deleted email.
  There is currently no "silent delete" button in the UI. The `?send_email=false` query param exists in the API for future use and for direct API consumers.

### 7c.2 — The toast matrix

The frontend reads `email_sent` from every response and picks the toast accordingly:

| Action | `send_email` | `email_sent` | Toast | Colour |
|---|---|---|---|---|
| Add | `false` | — | `Task saved ✅` | green |
| Email (create) | `true` | `true` | `Task saved & email sent ✅` | green |
| Email (create) | `true` | `false` | `Task saved (email could not be sent)` | red |
| Cancel | — | — | *(no toast — card reverts)* | — |
| Update | `false` | — | `Task updated ✅` | green |
| Email (update) | `true` | `true` | `Task updated & email sent ✅` | green |
| Email (update) | `true` | `false` | `Task updated (email could not be sent)` | red |
| Delete | (always `true`) | `true` | `Task deleted & email sent ✅` | green |
| Delete | (always `true`) | `false` | `Task deleted (email could not be sent)` | red |

### 7c.3 — The `send_email` tri-state default table

The backend accepts `send_email` as a `*bool` on `POST` / `PUT` (JSON body) so it can distinguish three cases. The table shows what happens for each.

| Endpoint | `send_email: true` | `send_email: false` | `send_email` omitted / `null` |
|---|---|---|---|
| `POST /tasks` | email fires | silent | **email fires** (backwards-compat default) |
| `PUT /tasks/{id}` | email fires | silent | **silent** (backwards-compat default) |
| `DELETE /tasks/{id}?send_email=…` | `"true"` → email fires | `"false"` → silent | **email fires** (new default — delete is destructive) |

The frontend always passes the flag explicitly, so the "omitted" column only matters for third-party API consumers or older cached builds.

### 7c.4 — Frontend parameter order reference

`frontend/src/tasks-service.ts` — the three call signatures:

```typescript
createTask(text, dueDate, token?, sendEmail = true)      // POST
updateTask(id, text, dueDate, token?, sendEmail = false) // PUT
deleteTask(id, token?, sendEmail = true)                 // DELETE
```

Every parameter after `token` is optional and has a **client-side default that matches the server-side default** for that endpoint, so omitting it produces identical behaviour at both layers.

### 7c.5 — Manual test checklist

After a fresh `./dev.sh`:

- [ ] New task → **Add**: task appears in list, **no** `[tasks] email dispatched` log line.
- [ ] New task → **Email**: task appears, log shows `[tasks] email dispatched for task …`.
- [ ] Edit → **Cancel**: card reverts, **no** API call (check Network tab).
- [ ] Edit → **Update**: text changes, **no** update-email log line.
- [ ] Edit → **Email**: text changes, log shows `[tasks] update email dispatched …`.
- [ ] Delete → confirm: task gone, log shows `[tasks] delete email dispatched …`.
- [ ] With SMTP misconfigured: green vs red toast flips according to `email_sent`.
- [ ] Cross-user delete (via API directly): 404, no email.

### 7c.6 — Migration note for external scripts

If you have any script, browser snippet, or Playwright spec that references the **old** `#tasks-new-send` button ID, update it:

| Old | New |
|---|---|
| `#tasks-new-send` | `#tasks-new-add` (silent) or `#tasks-new-email` (with email) |
| `[data-task-save]` on the old edit card | still exists, but now means "silent save"; the email-save button is `[data-task-email]` |

The `persona-tests/tests/api/api-tasks-persona.spec.ts` file does not reference any of these — it only tests the API — so it continues to pass unchanged.

---

## 8. Swagger Documentation

All authentication endpoints are fully documented and integrated with Swagger UI:
- **Swagger JSON**: `http://localhost:8080/swagger/doc.json`
- **Swagger UI**: `http://localhost:8080/swagger/`

To authorize in Swagger UI:
1. Click the **Authorize** button (green lock icon).
2. Enter your Bearer token in the format: `Bearer <YOUR_TOKEN>`.
3. Click **Authorize** to test protected endpoints (`/auth/me`, `/auth/reset-password`, `/auth/change-password`, `/tasks`, `/nifty50/companies`).

To regenerate documentation after making changes:
```bash
~/go/bin/swag init -g cmd/server/main.go -o docs
```

---

## 9. Automated Testing

Run all unit and integration tests across the project:
```bash
# Run internal auth unit tests (password validation, hashing, JWT, user store)
go test -v ./internal/auth

# Run end-to-end API integration tests (includes auth + CORS)
go test -v ./cmd/server -run TestAuth

# Run tasks handler regression tests
#
# Covers:
#   - nil-sender safety on all three operations
#   - dev-fallback semantics for create / update / delete
#   - send_email = true / false / omitted for each endpoint
#   - the new email-diff path for update
#   - the new email path for delete
#   - cross-user isolation with send_email=true
go test -v ./internal/api -run TestTasksHandler

# Run entire test suite
go test -v ./...
```