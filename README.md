# HyperSync

A multi-platform content synchronization service that syncs content from Memos to social networks like Mastodon, Bluesky, and Threads.

## Configuration

### Configuration File Example (local.yaml)

```yaml
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

## Running

```bash
# Build the project
make build

# Run the service
./bin/hyper-sync
```
