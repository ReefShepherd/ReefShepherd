# Install on Linux (Debian LXC or any systemd server)

ReefShepherd is one binary with the web UI inside and a SQLite database in a data directory. These steps install it as a systemd service on Debian 13. A container with 1 vCPU, 512 MB of RAM and 2 GB of disk is plenty. Run everything as root.

You need three files, from a release tarball or from `make build` in the repository. `make build` makes a binary for the CPU of the machine you run it on. To build for a 64-bit ARM server (such as a Raspberry Pi) from another machine, run `GOOS=linux GOARCH=arm64 make build` instead.

The files:

- `reefshepherd`, the binary for your CPU (amd64 or arm64)
- `reefshepherd.service`, in `deploy/systemd/`
- `reefshepherd.env.example`, in `deploy/env/`

## 1. Create the user

```sh
useradd --system --home-dir /var/lib/reefshepherd --no-create-home --shell /usr/sbin/nologin reefshepherd
```

## 2. Install the binary and the unit

```sh
install -m 0755 reefshepherd /usr/local/bin/reefshepherd
install -m 0644 reefshepherd.service /etc/systemd/system/reefshepherd.service
reefshepherd version
```

## 3. Create the env file

```sh
install -d -m 0755 /etc/reefshepherd
install -m 0640 -g reefshepherd reefshepherd.env.example /etc/reefshepherd/reefshepherd.env
```

Edit `/etc/reefshepherd/reefshepherd.env`. Every line starts with a `#`, which makes it a comment, so to use a line you must delete the `#` at its start. The defaults work, but you will usually set two:

```sh
REEFSHEPHERD_LISTEN=:8484
REEFSHEPHERD_PUBLIC_URL=http://192.168.1.50:8484
```

`REEFSHEPHERD_LISTEN` is where the server listens. The default, `:8484`, already accepts connections on every address of the machine, so you can usually leave it alone. Do not change it to `0.0.0.0:8484`, which looks equivalent but drops IPv6. `REEFSHEPHERD_PUBLIC_URL` is the address you type into the browser. Cookies are marked Secure when it starts with `https://`, and its origin is always allowed to submit forms.

## 4. Enable and start

```sh
systemctl daemon-reload
systemctl enable --now reefshepherd
systemctl status reefshepherd --no-pager
reefshepherd healthcheck --config /etc/reefshepherd/reefshepherd.env
```

The service needs a moment to start, so if the healthcheck prints "connection refused", wait a few seconds and run it once more.

Then open the public URL in a browser. The first visit shows the setup screen, where you create the administrator account and choose your units.

Logs are in the journal: `journalctl -u reefshepherd -f`.

If the service fails with `status=226/NAMESPACE` in a Proxmox container, open the container in Proxmox, go to Options > Features, enable Nesting, and restart the container.

## Updating

Replace the binary and restart. Database changes are applied on startup, and a backup of the existing data is written to the `backups` folder first.

```sh
install -m 0755 reefshepherd /usr/local/bin/reefshepherd
systemctl restart reefshepherd
reefshepherd version
```

If the unit file changed, copy it again and run `systemctl daemon-reload` before the restart.

## Where things live

| What | Where |
|---|---|
| Data directory | `/var/lib/reefshepherd` |
| Database | `/var/lib/reefshepherd/reefshepherd.db` (plus `-wal` and `-shm` files while running) |
| Backups made before a database upgrade | `/var/lib/reefshepherd/backups/` |
| Settings read at startup | `/etc/reefshepherd/reefshepherd.env` |

To back up or move an installation by hand, stop the service, copy the whole data directory, and start it again:

```sh
systemctl stop reefshepherd
cp -a /var/lib/reefshepherd /root/reefshepherd-backup
systemctl start reefshepherd
```

To restore, stop the service, replace the data directory with your copy, give it back to the service user, and start the service again:

```sh
systemctl stop reefshepherd
rm -rf /var/lib/reefshepherd
cp -a /root/reefshepherd-backup /var/lib/reefshepherd
chown -R reefshepherd:reefshepherd /var/lib/reefshepherd
systemctl start reefshepherd
```

## Locked out? The admin command

The admin command opens the same database as the server and works while the service is running. Run it as the service user and point it at the data directory:

```sh
runuser -u reefshepherd -- reefshepherd admin list-users --data-dir /var/lib/reefshepherd
runuser -u reefshepherd -- reefshepherd admin reset-password alice --data-dir /var/lib/reefshepherd
runuser -u reefshepherd -- reefshepherd admin reset-mfa alice --data-dir /var/lib/reefshepherd
runuser -u reefshepherd -- reefshepherd admin set-auth-mode disabled --data-dir /var/lib/reefshepherd
```

- `reset-password` sets a new random password, prints it once and signs that user out everywhere.
- `reset-mfa` removes the user's two-step login.
- `set-auth-mode disabled` turns the login off so anyone who can reach the page gets in; set it back to `password` from Settings once you are in. If `REEFSHEPHERD_AUTH_MODE` is set in the env file it wins, so remove it first.

## Behind a reverse proxy

ReefShepherd speaks plain HTTP, and the proxy handles TLS. It accepts form submissions only from the origin it is served on, so the proxy must pass the original `Host` header on. A proxy that replaces it with the upstream address (nginx does this by default) makes every save fail with "cross-origin request rejected".

nginx:

The two `ssl_` lines are placeholders. Replace them with the paths to your own certificate and its private key, or nginx will not start.

```nginx
server {
    listen 443 ssl;
    server_name reef.home.lan;

    ssl_certificate     /etc/ssl/reef/fullchain.pem;
    ssl_certificate_key /etc/ssl/reef/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8484;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Use `$http_host` instead of `$host` if the proxy listens on a port other than 80 or 443. Then tell ReefShepherd about the proxy in the env file and restart:

```sh
REEFSHEPHERD_LISTEN=127.0.0.1:8484
REEFSHEPHERD_PUBLIC_URL=https://reef.home.lan
REEFSHEPHERD_TRUSTED_PROXIES=127.0.0.1/32
```

`REEFSHEPHERD_TRUSTED_PROXIES` lists the addresses whose `X-Forwarded-For` and `X-Forwarded-Proto` headers are believed. If the site is reachable under more than one name, add the extra origins to `REEFSHEPHERD_TRUSTED_ORIGINS`, comma separated, for example `https://reef.example.com,https://reef.lan`.
