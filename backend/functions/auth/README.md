# Authentication handlers

Registration, login, logout, and middleware pass the HTTP request context to
`AuthRepo`. Registration accepts passwords of at least 6 characters and at most
72 bytes; oversized passwords return HTTP 400. Registration and login return the
new raw bearer token once with `User.Public()`. `Me` returns the public profile.
Login rejects passwords over 72 bytes with HTTP 401, including appended suffixes
on a valid 72-byte password.
No public auth response includes password hashes, salts, token hashes, or
revocation timestamps. Logout returns HTTP 500 if token revocation cannot be
persisted, allowing clients to retry instead of reporting a successful logout.
