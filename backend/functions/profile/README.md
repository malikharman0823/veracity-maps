# Profile handlers

`NewHandler(ProfileRepo, Mailer, storageRoot, publicURL)` receives persistence,
delivery, and the persistent storage root (`storage` in production).
Responses use the standard `{success, data}` / `{success, error}` envelope.

| Route | Request | Result |
| --- | --- | --- |
| `GET /api/profile` | Bearer token | Safe public profile, including `full_name` and compatibility `name` alias. |
| `PATCH /api/profile` | `{"full_name":"Ada Lovelace"}` | Trimmed name, 1–255 UTF-16 units; returns updated profile. |
| `POST /api/profile/password` | `{"current_password":"...","new_password":"..."}` | Verifies current password, updates bcrypt hash, revokes other tokens; retains this active token. |
| `POST /api/profile/email-change` | `{"email":"ada@example.com"}` | 202 after sending verification. Email stays unchanged until verification. The current email can also be verified this way. |
| `POST /api/auth/verify-email` | `{"token":"<token from email>"}` | Public, single-use activation of the pending email within 30 minutes. |
| `POST /api/auth/resend-verification` | Bearer token | 202 after rotating and sending initial account verification; 200 without sending if already verified. |
| `POST /api/profile/avatar` | Multipart `avatar` file | PNG/JPEG/WebP content detection, up to 5 MiB; returns updated profile. |
| `GET /api/profile/avatar` | Bearer token | Current user's avatar bytes with private/no-store caching. |

Passwords follow the registration minimum of 6 characters and bcrypt's 72-byte
maximum. Wrong current passwords return 401. Duplicate emails return 409;
expired, superseded, or reused verification tokens return 400. Mail failure
returns 502 `EMAIL_DELIVERY_FAILED` with no provider details. The pending request
remains stored; requesting another verification rotates its token. Neither the
raw token nor the link is returned by the email-change API.

All verification email requests share a database-enforced one-minute per-user
window. A user-row update lock serializes token creation across purposes, and
requests inside the window return 429 `VERIFICATION_RATE_LIMITED` with
`Retry-After: 60`. A failed delivery also occupies the window. Rotation consumes
older tokens of the same purpose.

Links use `<APP_PUBLIC_URL>/verify-email?token=...`; the frontend verification
page must submit that token to the public POST endpoint. Registration sends an
`initial_verification` token; verification marks the existing email verified
without modifying a separately pending email change. Email-change tokens activate
only the pending address. Avatar files are generated beneath
`storage/users/<user_id>/profile/`, written before updating database metadata,
and replaced files are removed after success. The authenticated avatar URL
requires fetching bytes with the bearer token (for example, a frontend blob URL).
