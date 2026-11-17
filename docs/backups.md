# Backups

## Backups go to a backio agent

Set `APPSTOREANALYTICS_BACKUP_URL` to a [backio-agent] and appstoreanalytics posts it a gzipped
`VACUUM INTO` snapshot of the database. Leave it unset and nothing is backed up. The snapshot is
written to a `backup-*` folder in the data directory and removed once sent; one left by a process
killed mid-pass is removed on the next start.

```
appstoreanalytics ──POST archive──▶ agent ──▶ wherever the agent was told
                  (no credential)    (holds the token, names the file, prunes old ones)
```

```yaml
services:
  appstoreanalytics:
    image: ghcr.io/reeywhaar/appstoreanalytics:latest
    environment:
      APPSTOREANALYTICS_BACKUP_URL: http://backup:8080/backup
    networks: [default, caddy]

  backup:
    image: ghcr.io/reeywhaar/backio-agent:latest
    restart: unless-stopped
    environment:
      # …destination and credential, which are the agent's business
```

Behind Caddy, appstoreanalytics is on both networks: Caddy reaches it on one and it reaches the
agent on the other.

**appstoreanalytics holds no credential, no provider, no bucket and no retention policy.** It
decides when to back up and what goes in the archive; everything after that belongs to the
agent. A compromised appstoreanalytics container cannot read, overwrite or delete a single
existing backup. It can only hand over one more archive.

**An archive holds the mail relay's password and the webhook addresses' secrets** as the
database does, in the clear. Keep the backups as private as the server.

[backio-agent]: https://github.com/reeywhaar/backio/tree/main/agent

## A copy goes out when what it is for changed, and at no other time

The loop looks every half hour and sends only when something was written since the last archive
the agent accepted that a backup is for:

- an analytics report instance read: Apple deletes one 35 days after making it, so what it said
  is kept only here;
- a day's sales report read for the first time;
- a webhook delivery, and how mailing it went, which are nowhere else;
- the relay, a webhook address, or somebody mailed of webhooks, set, changed or removed. Saving
  what is already there is not a change.

What the process notices of itself, and what the next sync reads anyway, are not: sign-ins, a
sync's bookkeeping, report requests, rates, the list of apps, and a sales day read again while
its territories arrive.

A restart sends one archive, because a process that has just started may be on a volume nobody
has a copy of yet. There is no heartbeat: an instance with nothing new sends nothing, and
whether it is running is `/healthz`'s question.

## The snapshot is read, never written

`VACUUM INTO` runs on a reader connection. A WAL reader sees one moment and blocks nobody, so a
sync keeps writing while the copy is made; on the one writer connection, every write would wait
for it to finish.

## Waiting as long as the agent does

The agent answers once the archive is on the remote. appstoreanalytics waits up to 30 minutes,
the agent's own upload timeout, so a slow upload is not recorded as a failure for an archive that
was stored anyway. The archive is sent from a file, with its size, rather than held in memory or
sent chunked.

## Restoring is putting the file back

An archive is a gzipped tar of one member, `appstoreanalytics.db`, mode `0600`, with no directory
entries. Stop the container, delete `appstoreanalytics.db-wal` and `appstoreanalytics.db-shm` if
they are there, since they belong to the database being replaced and would be replayed onto the
restored one, extract the archive into the data directory, and start it: the migrations bring an
older one up to date. Browsers stay signed in, because sessions are in the same file.
