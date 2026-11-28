# Banning with fail2ban

A refused sign-in is one `WARN` line in the container's log, with the caller's address, for
fail2ban or anything like it to count.

## The lines

The log is JSON, a line each:

```json
{"time":"2026-11-22T09:14:03.120Z","level":"WARN","msg":"sign-in refused","ip":"203.0.113.9"}
{"time":"2026-11-22T09:14:05.871Z","level":"WARN","msg":"sign-in rate limited","ip":"203.0.113.9"}
```

| message | when |
| --- | --- |
| `sign-in refused` | a wrong username or a wrong password; the two read the same |
| `sign-in rate limited` | too many tries for the username, or for the whole instance |

The message is fixed and the address comes right after it, so a filter is one expression. The
username that was tried is never written: it is sometimes a password typed into the wrong field.

A webhook address that is not one of ours is not counted. Its token is not guessed in any number
of tries, and the likeliest sender is Apple, still delivering to an address removed here: a ban
would cut Apple off from every other address too.

## Behind a proxy

Behind a proxy the connection comes from the proxy, so the caller is read from
`X-Forwarded-For`, but only when the connection comes from a trusted proxy. The address a session
is shown as last seen from, under Settings, is read the same way. From anybody else the
header is the caller's own claim, and believing it would let them name the address that gets
banned.

`APPSTOREANALYTICS_TRUSTED_PROXIES` names the trusted proxies: addresses, ranges, `private`, or
`none`, separated by commas or spaces.

| setup | value |
| --- | --- |
| Proxy on the same host or Docker network | `private`, the default |
| Proxy on another machine | its address, e.g. `198.51.100.3` |
| A CDN in front of that proxy | `private` plus the CDN's published ranges |
| No proxy, or the port is reachable without it | `none` |

`private` is 10/8, 172.16/12, 192.168/16, fc00::/7 and loopback. It assumes every caller with
such an address is yours; where that is not so, name the proxy itself instead.

The header is read right to left. Each proxy appends who it accepted, so the first hop that is
not a trusted proxy is the caller, and anything before it is whatever the caller sent. Your proxy
must set the header: Caddy does by default, nginx needs
`proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for`. `X-Real-IP` is never read: a
proxy that does not set it passes the caller's along.

Rootless Docker and Podman can deliver every connection from one private address, so a caller
reaching the port directly looks like the proxy. Keep that port unpublished, or set `none`.

## The filter and the jail

A filter, `/etc/fail2ban/filter.d/appstoreanalytics.conf`. The optional backslashes match the same
line inside Docker's JSON log, which escapes its quotes:

```ini
[Definition]
failregex = \\?"msg\\?":\\?"(?:sign-in refused|sign-in rate limited)\\?",\\?"ip\\?":\\?"<HOST>\\?"
```

And a jail, `/etc/fail2ban/jail.d/appstoreanalytics.local`:

```ini
[appstoreanalytics]
enabled  = true
filter   = appstoreanalytics
logpath  = /var/lib/docker/containers/*/*-json.log
maxretry = 10
findtime = 10m
bantime  = 1h
```

With the container logging to journald instead, `backend = systemd` and
`journalmatch = CONTAINER_NAME=appstoreanalytics` replace `logpath`.

The ban has to land where the caller's packets do. With the proxy on the same host the default
works, since it is the proxy's port that is blocked. A port Docker publishes straight from the
container goes around the host's `INPUT` chain, and wants `chain = DOCKER-USER`. Behind a proxy
on another machine or a CDN, a firewall here never sees the caller at all: run fail2ban there,
or have the CDN do the banning.
