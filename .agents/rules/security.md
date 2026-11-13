# Always-on security and safeguarding constraints

- No cross-tenant read or write. Validate tenant membership and resource-level child/teacher/guardian relation server-side on every endpoint.
- Bind tenant scope to trusted server context; never trust request body or URL `tenant_id` as authority.
- Use parameterized SQL, transactional data writes and tenant composite foreign keys; test for IDOR and unauthorized access.
- No secrets in client bundles, Git commits or logs. Use runtime env secrets and redaction; redact student PII in error reporting.
- Secure cookie/session handling, CSRF protection, authentication rate limiting and invitation token hashing.
- Uploaded media private by default; validate types/sizes, scan and serve through short-lived signed access.
- AI provider calls only from Go gateway. No child profile, guardian contact, grades or other PII in prompts by default.
- No unsupervised Early Years chatbot; no auto-publishing AI drafts; no automatic high-stakes educational decisions.
- Never claim a child is monitored by a human unless a staffed and tested workflow exists.
- Grade/report edits require permissions, validation and audit; published reports are versioned snapshots.
- All fixtures fictional; never use actual children or staff data without authorization and deployment compliance review.
