# HyperSync

A multi-platform content synchronization tool that supports syncing content from Memos and other platforms to social networks like Mastodon and Bluesky.

## Configuration

### Configuration File Example (local.yaml)

```yaml
socials:
  # Main content source - Memos
  memos:
    name: memos
    type: memos
    enabled: true
    sync_enabled: true
    main: true
    sync_to:
      - blueskey
      - mastodon
    memos:
      endpoint: https://your-memos-instance.com
      token: your_memos_access_token

  # Mastodon configuration
  mastodon:
    name: mastodon
    type: mastodon
    enabled: true
    sync_enabled: true
    mastodon:
      instance: https://mastodon.world
      token: your_mastodon_access_token

  # Bluesky configuration
  blueskey:
    name: blueskey
    type: bluesky
    enabled: true
    sync_enabled: true
    bluesky:
      host: https://bsky.social
      handle: your-handle.bsky.social
      password: your_app_password

# Data storage configuration
store:
  mongo:
    main: 
      uri: "mongodb://localhost:27017/hypersync"
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
  - `host`: Bluesky server address (usually bsky.social)
  - `handle`: Your Bluesky username
  - `password`: App-specific password

#### Storage Configuration

- **mongo**: MongoDB database configuration
  - `uri`: MongoDB connection string

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

## Running

```bash
# Build the project
make build

# Run the service
./bin/hypersync
```
