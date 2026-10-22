# Authentication & Login Feature Documentation (`LOGIN_FEATURE.md`)

This document provides a comprehensive developer guide for the Authentication System implemented in the Rutils utility platform.

---

## 1. Architectural Overview

The authentication system is completely isolated and decoupled from existing utility services (file conversion, text extraction, calculations). It provides:
- **User Persistence**: File-based JSON storage in `data/users.json` with thread safety (`sync.RWMutex`) and atomic file replacement.
- **Credential Security**: Passwords hashed using `bcrypt` at cost factor `12`.
- **Session Management**: Stateless JSON Web Tokens (JWT) signed with HMAC-SHA256 (`HS256`).
- **Google SMTP Integration**: Automated dispatching of temporary passwords, security notices, and task-created notifications with zero hardcoded credentials and an automatic console-logging development fallback when credentials are not configured.
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
2. For protected endpoints (`/auth/reset-password`, `/auth/change-password`, `/auth/me`, `/tasks`, `/nifty50/companies`), include the token in the HTTP `Authorization` header:
   ```http
   Authorization: Bearer <YOUR_JWT_TOKEN>
   ```
3. **Forced Reset Workflow**: If a user logs in using a temporary password issued via `/auth/forgot-password`:
   - `must_reset_password` is set to `true` in the JWT claims and login response.
   - The user must call `POST /auth/reset-password` with their new password before continuing normal operations.
4. **Embedded Claims**: every signed token carries `user_id`, `email`, `name`, and `must_reset_password`. The `name` claim is used by downstream handlers (e.g. the tasks email dispatcher) to personalise outgoing emails without a second user-store lookup.

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

### 8. Create Task (with email notification)
- **Route**: `POST /tasks`
- **Access**: Authenticated (`Authorization: Bearer <token>`)
- **Request Body**:
  ```json
  {
    "text": "Buy groceries for the weekend",
    "due_date": "2026-11-05"
  }
  ```
- **Responses**:
  - `201 Created`:
    ```json
    {
      "success": true,
      "task": {
        "id": "tsk_f1cecc9ba262012e",
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
- **Email behaviour**: On success, the backend dispatches a confirmation email to the authenticated user's email address. See **Section 7 — Task Created Notification** below for full details on the email contents, the `email_sent` field semantics, and the best-effort dispatch guarantees.

---

## 7. Task Created Notification

When an authenticated user creates a task via the **Tasks** view, the backend dispatches a confirmation email to the user's registered address. This is the newest email flow in the platform, added alongside the password-recovery emails.

**Trigger:** the user clicks the **SEND** button in the Tasks view. That action calls `POST /tasks` with the task text and due date. The handler (`internal/api/tasks_handler.go` → `Create`) saves the task to `data/tasks.json`, then immediately attempts to dispatch the confirmation email.

**What the email contains:**
- A personalised greeting using the authenticated user's name (`Hello Jane,`).
- The exact task text the user typed — multi-line notes preserve their line breaks via `white-space: pre-wrap` in the HTML body.
- The due date in `YYYY-MM-DD` format, displayed as a monospace "🗓 Due Date" pill.
- The standard Rutils footer.
- A subject line of the form `📌 Task Created: "<task text up to 60 chars>"` — newlines are stripped so the header stays RFC-safe.

**Personalisation source:** the greeting uses the `Name` claim embedded in the user's JWT (see Section 5). Every login signs a token that carries `UserID`, `Email`, and `Name`, so the tasks handler does not need a second user-store lookup to personalise the email.

**Best-effort semantics:** the email is dispatched **after** the task is saved, and a dispatch failure never turns a successful save into an error response. If SMTP is unreachable, misconfigured, or times out, the API still returns `201 Created` with the saved task, and the response body distinguishes the two outcomes:

| Response field | Meaning |
|---|---|
| `email_sent: true`  | The task was saved and the confirmation email was dispatched (or the dev fallback logged it). |
| `email_sent: false` | The task was saved, but the email could not be sent. The frontend surfaces this as a red "Task saved (email could not be sent)" toast. |

**Timeout guard:** the SMTP call is wrapped in a 10-second `select` in `dispatchTaskEmail` (`internal/api/tasks_handler.go`). If `smtp.SendMail` hangs beyond 10 seconds, the HTTP response still returns promptly — `email_sent: false` — while the stuck goroutine exits on its own once the underlying `net.DialTimeout` (15 s) fires. This prevents a slow SMTP server from creating a backlog of hanging HTTP requests.

**Dev fallback:** when `smtp_email` or `smtp_app_password` are empty in `config.json`, the sender logs a single line to stdout and returns `nil` — meaning the frontend still shows "Task saved & email sent ✅" locally. This keeps the UX testable without real SMTP credentials:

```text
[AUTH-DEV] SMTP not configured in config.json. Task notification for <user@example.com> simulated: text="Buy groceries" due="2026-11-05"
[tasks] email dispatched for task tsk_xxxx to <user@example.com>
```

**Which endpoints send email:**

| Endpoint | Sends email? | Notes |
|---|---|---|
| `POST /auth/forgot-password` | ✅ | Temporary password email |
| `POST /auth/change-password` | ✅ | Password-changed confirmation |
| `POST /auth/reset-password` | ❌ | No email — reset is synchronous, and the JWT already proves the user's identity |
| `POST /tasks` | ✅ | Task-created notification (this section) |
| `PUT /tasks/{id}` | ❌ | Edits do not notify — scope of the feature is strictly creation |
| `DELETE /tasks/{id}` | ❌ | Deletion does not notify |

**Code locations:**
- Email template + sender method: `internal/auth/email.go` → `SendTaskNotification(toEmail, userName, taskText, dueDate)`
- Handler that triggers it: `internal/api/tasks_handler.go` → `Create` + `dispatchTaskEmail`
- Response field: `internal/tasks/models.go` → `TaskResponse.EmailSent`
- Frontend feedback: `frontend/src/tasks-ui.ts` → `handleCreate` reads `res.email_sent` and picks the toast wording

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

# Run tasks handler regression tests (nil-sender safety, response shape)
go test -v ./internal/api -run TestTasksHandler

# Run entire test suite
go test -v ./...
```