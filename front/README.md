# HyperSync Frontend

React + TypeScript + Vite SPA for HyperSync's post management: login, post CRUD with draft/publish, media upload, per-platform sync status, and settings (password change). UI built with shadcn/ui; API calls go through ConnectRPC (`@connectrpc/connect-web`) with a JWT interceptor.

## Development

```bash
npm install
npm run dev        # Vite dev server; proxies /api* to http://localhost:8080
```

The backend must be running locally on port 8080 (see the repo root README for backend setup, including the required `auth` config).

## Generated code

`src/gen/` is generated from `../proto` via buf:

```bash
npx buf generate --template buf.gen.yaml ../proto
```

Regenerate after changing any `.proto` file. Do not edit `src/gen/` by hand.

## Build & deploy

```bash
npm run build      # tsc -b && vite build → dist/
```

The Docker image serves `dist/` with nginx and reverse-proxies all `^/api[./]` paths (ConnectRPC + REST) to the backend:

```bash
docker build -t hypersync-front .
docker run -e BACKEND_URL=http://hypersync:8080 -p 80:80 hypersync-front
```

`BACKEND_URL` defaults to `http://backend:8080`; the nginx config is templated at container start (`nginx.conf.template`, envsubst).

## Structure

- `src/lib/` — ConnectRPC transport + JWT interceptor (`connect.ts`), auth context (`auth.tsx`), media upload helper (`media.ts`)
- `src/pages/` — login, posts list, create/edit post, post detail, settings
- `src/components/` — media upload widget, shadcn/ui primitives
