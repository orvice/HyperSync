# HyperSync

A personal content publishing hub that manages posts and syncs them to social platforms.

## Language

**Post**:
A piece of content authored in HyperSync. The source of truth for all published content. Has a lifecycle (draft → published) and tracks sync state per target platform.
_Avoid_: Memo, status, toot, note

**Media**:
An uploaded file (image/video) stored in S3-compatible object storage. Exists independently of Posts and can be referenced by multiple Posts.
_Avoid_: Attachment, resource, asset

**Platform**:
An external social service that HyperSync syncs Posts to (e.g. Mastodon, Bluesky, Threads, Memos). All platforms are sync targets; none are content sources.
_Avoid_: Social, target, destination

**Sync**:
The process of publishing a Post's content and media to a Platform. Performed asynchronously by a background worker.
_Avoid_: Cross-post, federation, relay

**CrossPostStatus**:
The per-platform record of a Post's sync outcome — tracks whether sync succeeded, failed, or is pending, along with the platform-side post ID for update/delete operations.
_Avoid_: SyncRecord, publish status

**Visibility**:
The audience scope of a Post. Four levels: `public`, `unlisted`, `private`, `direct`. Only `public` and `unlisted` Posts are synced to Platforms.
_Avoid_: Privacy, access level

**Draft**:
A Post that has been saved but not yet published. Drafts are not picked up by the sync worker.
_Avoid_: Unpublished, pending
