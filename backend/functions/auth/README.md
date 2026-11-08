# Authentication handlers

Login calls the atomic `AuthRepo.AuthenticateAndCreateToken` operation. Invalid
credentials return 401; persistence or token-issuance failures return a safe
500 `AUTHENTICATION_ERROR`. It never mints a token from a separately verified
password snapshot.

Registration, login, logout, and middleware pass the HTTP request context to
`AuthRepo`. Registration accepts passwords of at least 6 characters and at most
72 bytes; oversized passwords return HTTP 400. Registration and login return the
new raw bearer token once with `User.Public()`. `Me` returns the public profile.
Login rejects passwords over 72 bytes with HTTP 401, including appended suffixes
on a valid 72-byte password.
No public auth response includes password hashes, salts, token hashes, or
revocation timestamps. Logout returns HTTP 500 if token revocation cannot be
persisted, allowing clients to retry instead of reporting a successful logout.

Production uses `NewHandlerWithVerification(AuthRepo, ProfileRepo, Mailer,
publicURL)`. Registration creates a 30-minute, hashed `initial_verification`
token and sends it using the injected mailer. It still returns 201, the normal
auth token, and `email_verified=false` if mail/token creation fails; the account
is retained. Additional `verification_delivery` is `sent` or `failed`; failures
include safe `verification_code` (`EMAIL_DELIVERY_FAILED` or
`VERIFICATION_UNAVAILABLE`) without provider details or verification tokens.
The compatibility `NewHandler(AuthRepo)` constructor is used by isolated legacy
tests that do not exercise account mail.

Authenticated `POST /api/auth/resend-verification` rotates initial tokens after
the shared one-minute SQL-enforced verification window (429 and `Retry-After: 60`
when limited). It returns 202 after delivery, 502 on delivery failure, or 200
without sending for an already verified email. Public
`POST /api/auth/verify-email` consumes the mail token once. The frontend handles
the `/verify-email?token=...` page and submits its token in the POST body.

---

## Passwordless Email OTP Authentication

To provide a secure, modern passwordless experience, the backend implements:
- `POST /api/auth/otp/send`: Generates a cryptographic 6-digit numeric OTP, hashes it with SHA-256, persists it in the `EmailOTPs` table with a 10-minute expiry and 60-second resend rate limit, and dispatches the code via Resend (`onboarding@resend.dev`).
  - Request: `{ "email": "user@example.com", "purpose": "login" | "register", "name": "Optional Name" }`
  - Response: `{ "status": "success", "data": { "email": "...", "purpose": "...", "resend_in_seconds": 60, "expires_in_seconds": 600 } }`
- `POST /api/auth/otp/verify`: Atomically validates and consumes the single-use OTP code.
  - Enforces max 5 attempts per code, constant-time comparison against the stored SHA-256 hash, and marks `consumed_at`.
  - For `register`: Creates the user record with `email_verified = 1`, logs in, and issues a session bearer token.
  - For `login`: Verifies user exists, logs in, and issues a session bearer token.
  - Request: `{ "email": "user@example.com", "code": "123456", "purpose": "login" | "register", "name": "Optional User Name" }`
  - Response: `{ "status": "success", "data": { "token": "...", "user": { ... } } }`

