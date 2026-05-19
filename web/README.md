# Cloud Lab Gateway Web

React + Vite + TypeScript frontend for the Cloud Lab Gateway.

## Commands

```bash
npm install
npm run dev
npm run gen:api
npm run typecheck
npm run lint
npm run build
```

The dev server runs on `http://127.0.0.1:5173` and proxies:

- `/api` to `http://localhost:8080`
- `/sse` to `http://localhost:8080`
- `/lti` to `http://localhost:8080`

## Demo Mode

Until Moodle and KI/OpenStack access are available, the login page provides
local demo users for student, teacher, and admin roles. The student page uses
mock lab data that mirrors the real state machine and can later be replaced by
TanStack Query calls to the generated OpenAPI client.
