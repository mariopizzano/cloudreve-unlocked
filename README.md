# cloudreve-unlocked

English | [Italiano](README.it.md)

Open-source fork of [Cloudreve](https://github.com/cloudreve/Cloudreve) 4.19.1 that implements, without any proprietary code, some features of the Cloudreve Pro Edition.

The goal of the fork is to cover the **storage management** and **system** areas, which are the essential ones for a public instance. The other Pro areas are explicitly declared as not implemented below.

## Implemented features

### Storage management

- **Per-directory policy**: a storage policy can be mounted on a folder; new uploads in that folder and its subfolders use that policy. The closest mount wins; folders without a mount inherit from their parent. It also works over WebDAV.
- **Effective and mounted policy** shown in the folder details.
- **Multiple policies per user group**: the admin assigns a pool of policies to a group; the user picks their preferred policy among the assigned ones. Precedence for new uploads:
  1. policy of the existing file;
  2. closest directory mount;
  3. user preference, if valid within the group pool;
  4. group default;
  5. system default.
- **Load balancing policy**: a virtual policy with multiple member policies and **weights**; uploads are distributed across members in proportion to their weight. Files and entities always point to concrete policies, never to the virtual one. A load balancing policy cannot be selected as a relocation target.
- **Storage policy relocation (migration)**:
  - dry-run with estimates of files, entities, versions, thumbnails and size;
  - moves the primary entity, versions and thumbnails;
  - supports encrypted sources and destinations;
  - clones shared entities without affecting the other files;
  - choice between keeping or deleting the source copy;
  - per-entity audit in the task and in the UI;
  - **automatic cleanup** of retained source objects (`delete_source=false`) with configurable retention (7 days by default), safety check on live references, and admin API for status and forced cleanup.

### System

- **Activity log**: persistent audit of sign-ins (successes/failures) and file/share operations, with IP address and user agent. Users see their own file history and recent sign-ins; the admin has a dedicated page with filters and details. Best-effort logging: an audit failure does not block the original operation.
- **i18n pipeline** with build-versioned translation cache and build-time check of the required keys.

### Fixes

- correct upload speed on resume;
- SQLite `busy_timeout` to reduce lock contention;
- safe handling of old relocation entities (hard delete, no orphan records).

## What is missing

The following Pro Edition features are **not implemented** and are considered out of scope for this fork:

- **Abuse reports**: no reporting channel for users/visitors and no admin review panel.
- **Advanced sharing**: edit/delete through share links, granular file/folder permissions, anonymous upload, default shares, paid sharing.
- **Monetization**: storage plans, memberships, redeem codes, credits, orders and payments, receipt/quota-exceeded email templates.
- **Authentication**: multi-account switching, Logto SSO, OIDC, QQ Connect, email filtering on registration.
- **Other system extensions**: site announcements, node selection, user-type custom properties, iOS VOL.

For the implemented features there are no "upgrade to Pro" popups or upsells: the related Pro controls have been removed or unlocked. Entries for unimplemented features are not exposed as working.

## Build

Requirements: Go 1.26+, Node.js 22+, Python 3, `zip`.

```bash
./scripts/build-release.sh 4.19.1
```

Output: `dist/cloudreve-4.19.1-linux-amd64`, with the frontend embedded in the binary.

## License

GPL-3.0, see `cloudreve/LICENSE`. Independent fork, not affiliated with Cloudreve.
