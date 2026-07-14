# HyperSync

A personal content publishing hub. Author posts in HyperSync (React frontend + ConnectRPC API) and sync them to social platforms — Mastodon, Bluesky, Threads, and Memos — with media upload to S3-compatible storage. Also ingests content from Telegram channels (including multi-photo/video albums) and includes the original Memos → social networks sync pipeline.

## Features

- **Post management** — draft/publish lifecycle, per-platform sync targets, edit with re-sync, delete with cascade to all synced platforms
- **Media upload** — S3-compatible object storage with CDN URLs, attached to posts on sync
- **Web frontend** — React + shadcn/ui under `front/`, shipped as a separate Docker image
- **JWT auth** — single-user login; `auth.jwt_secret` is required or the server refuses to start
- **Telegram ingestion** — pull content from a Telegram channel via Bot API; multi-photo/video albums are merged into a single Post
- **Legacy sync** — the original Memos → Mastodon/Bluesky/Threads pull-based sync still runs alongside

## Configuration

### Configuration File Example (local.yaml)

```yaml
# Required: the server fails to start without auth
auth:
  username: admin
  password: your-login-password        # seeded into MongoDB on first startup
  jwt_secret: "use-32-plus-random-characters-here"

# Optional: S3-compatible media storage (falls back to in-memory for dev)
storage:
  s3:
    endpoint: https://s3.example.com
    bucket: hypersync-media
    access_key: your_access_key
    secret_key: your_secret_key
    region: auto
    cdn_domain: https://cdn.example.com

socials:
  # Main content source - Memos
  memos:
    name: memos
    type: memos
    enabled: true
    sync_to:
      - bluesky
      - mastodon
      - threads
    memos:
      endpoint: https://your-memos-instance.com
      token: your_memos_access_token

  # Mastodon configuration
  mastodon:
    name: mastodon
    type: mastodon
    enabled: true
    mastodon:
      instance: https://mastodon.world
      token: your_mastodon_access_token

  # Bluesky configuration
  bluesky:
    name: bluesky
    type: bluesky
    enabled: true
    bluesky:
      host: https://bsky.social
      handle: your-handle.bsky.social
      password: your_app_password

  # Threads configuration
  threads:
    name: threads
    type: threads
    enabled: true
    threads:
      client_id: "your_client_id"
      client_secret: "your_client_secret"
      access_token: "your_access_token"
      user_id: 1234567890

  # Telegram channel ingestion (content source only)
  telegram:
    name: telegram
    type: telegram
    enabled: true
    sync_to:
      - mastodon
      - bluesky
    telegram:
      bot_token: "123456:ABC-DEF..."
      channel_id: "-1001234567890"

# Data storage configuration
store:
  mongo:
    main:
      uri: "mongodb://localhost:27017/hypersync"
  redis:
    locker:
      addr: "localhost:6379"

# Optional: tune sync behavior (defaults shown)
sync:
  interval: 30s
  batch_size: 100
  skip_older: 1h
  max_retries: 3
```

### Configuration Details

#### Social Platform Configuration

- **memos**: Main content source, supports fetching content from Memos instances
  - `endpoint`: Memos server address
  - `token`: Memos API access token
  - `sync_to`: Specify which platforms to sync to

- **mastodon**: Mastodon instance configuration
  - `instance`: Mastodon instance URL
  - `token`: Access token obtained from Mastodon

- **bluesky**: Bluesky social network configuration
  - `host`: Bluesky server address (usually `https://bsky.social`)
  - `handle`: Your Bluesky username
  - `password`: App-specific password

- **threads**: Threads (Meta) configuration
  - `client_id`: Meta app client ID
  - `client_secret`: Meta app client secret (for token exchange)
  - `access_token`: Initial access token (written to DB on first startup, then DB takes precedence)
  - `user_id`: Your Threads user ID

- **telegram**: Telegram channel ingestion (content source only, not a sync target)
  - `bot_token`: Bot API token from [@BotFather](https://t.me/BotFather)
  - `channel_id`: Channel ID (numeric, usually starts with `-100`)

#### Storage Configuration

- **mongo**: MongoDB database configuration
  - `uri`: MongoDB connection string
- **redis**: Redis configuration (required for distributed locks)
  - `addr`: Redis server address

### Getting Access Tokens

#### Memos
1. Log in to your Memos instance
2. Go to Settings page
3. Generate access token in the API section

#### Mastodon
1. Log in to your Mastodon instance
2. Go to Settings → Development
3. Create a new application
4. Get the access token

#### Bluesky
1. Log in to Bluesky
2. Go to Settings → App Passwords
3. Create a new app password

#### Threads
1. Create a Meta app at [Meta for Developers](https://developers.facebook.com/)
2. Configure Threads API permissions
3. Obtain an access token via OAuth flow

#### Telegram
1. Create a bot via [@BotFather](https://t.me/BotFather) and copy the bot token
2. Add the bot as an admin to your channel
3. Get the channel ID (numeric form, e.g. `-1001234567890`)

## Running

```bash
# Build the project
make build

# Run the service
./bin/hyper-sync
```

## Frontend

The web UI lives in `front/` (Vite + React) and is deployed as its own Docker image (nginx). The nginx config proxies `^/api[./]` (ConnectRPC + REST) to the backend via the `BACKEND_URL` environment variable:

```bash
# Development (proxies /api to localhost:8080)
cd front && npm install && npm run dev

# Production image
docker build -t hypersync-front ./front
docker run -e BACKEND_URL=http://hypersync:8080 -p 80:80 hypersync-front
```

Published images are available from GitHub Container Registry:

- Backend: `ghcr.io/orvice/hypersync`
- Frontend: `ghcr.io/orvice/hypersync-front`

See `docs/` for API reference (`api.md`), configuration details (`configuration.md`), data model (`data-model.md`), and module layout (`modules.md`).
