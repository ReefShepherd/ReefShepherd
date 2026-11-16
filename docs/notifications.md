# Notifications

ReefShepherd can send notifications to your phone, computer or other devices when important events happen. Notifications go to an in-app inbox (shown with a bell icon in the header) and to any channels you set up: Discord, Slack, ntfy, email or a generic webhook.

## How notifications work

Four kinds of event raise notifications:

- **Maintenance due**: a task with reminders on has reached its due time. It is sent within a minute of that time, or as soon as the app starts again if it was off. Each task is reminded once per due date; a task left undone is not reminded again, and paused tasks and tasks on archived tanks are never reminded.
- **Tests due**: one or more water tests are now due on a tank. One summary per tank per day at your reminder time, listing only the tests newly due since the last summary for that tank. If you change the instance timezone or a test's schedule, a test that is still overdue can be listed once more.
- **Out of range**: a test result you just logged is outside the ideal range for that parameter. Only for results measured in the last 24 hours.
- **Backup failed**: the scheduled backup of the database failed. The backup job runs once a day at the backup time, and once more after a restart if that day's backup is missing, so a failure is retried the next day or at the next restart. The notification is raised once for each scheduled backup.

When an event fires, the app creates a notification in your in-app inbox (newest first). If you have set up notification channels, the app queues a copy to each channel subscribed to that event type and tries to send it.

## The inbox and the bell

The bell icon in the header shows the number of unread notifications (up to 99+). Tap it to open your inbox. Each notification shows a severity label (Info; Warning in amber; Critical in red), the title and body, and when it was created.

Tap a notification to mark it read and open the link to the part of the app the notification is about (for example, a task or a chart). Unread notifications can be filtered from the list, and you can mark all as read at once.

## Setting up notification channels

Notification channels are admin-only. Open Settings > Notifications to see, add or edit channels.

When you add a channel, you pick a kind (Discord, Slack, ntfy, webhook or email), give it a name, set up its details, and choose which event types it should receive. You can toggle the channel on and off, and you can test it with a sample message by clicking "Send test". The test runs synchronously and shows the result immediately on the page without changing the channel's last sent or error time.

Secrets like webhook URLs, the ntfy topic, and passwords are never shown again after you save the channel. If you need to change a secret, the field shows an "Unchanged" placeholder. Leave it empty to keep the saved secret, or type a new value to replace it.

### Getting URLs and secrets

#### Discord

1. Open your Discord server, go to Server settings > Integrations > Webhooks.
2. Create a new webhook or use an existing one.
3. Copy the webhook URL. It will look like `https://discord.com/api/webhooks/123456789/ABCDEFG...`.
4. Paste it into the ReefShepherd channel settings under "Webhook URL".

Discord messages are coloured blue for info, orange for warning and red for critical.

#### Slack

1. Open your Slack workspace at slack.com, and go to the app directory at the top left.
2. Search for "Incoming Webhooks", click it and click "Install" or "Add to Slack".
3. Choose a channel and click "Allow".
4. Copy the webhook URL. It will look like `https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXX`.
5. Paste it into ReefShepherd under "Incoming webhook URL".

#### ntfy (public server)

1. Go to ntfy.sh. Choose a topic name (like `reef-tank-alerts`). This is enough.
2. In ReefShepherd, leave "Server" as https://ntfy.sh (the default) and paste your topic name under "Topic".
3. Optionally, paste a bearer token under "Access token (optional)" if your ntfy server requires authentication.

#### ntfy (self-hosted)

1. Run a ntfy server on your network (see ntfy documentation).
2. In ReefShepherd, paste the server URL under "Server" (like `http://192.168.1.100:2586`).
3. Paste your topic name under "Topic".
4. If your server requires a token, paste it under "Access token (optional)".

#### Generic webhook

A generic webhook receives POST requests with a JSON body in ReefShepherd's versioned format. This lets you forward notifications to a custom system or another app that accepts webhooks.

The body is:

```json
{
  "version": 1,
  "event": "maintenance_due",
  "severity": "info",
  "title": "Water change is due on Display",
  "body": "Due Tue 17 Dec 2024, 09:00 UTC.",
  "link": "http://192.168.1.50:8484/tanks/1/maintenance",
  "tank": {
    "id": 1,
    "name": "Display"
  },
  "occurred_at": "2024-12-17T09:00:04Z"
}
```

Fields:
- `version`: the format version, currently 1. It changes only if the body's format changes, so check it before reading the other fields.
- `event`: one of `maintenance_due`, `tests_due`, `out_of_range` or `backup_failed`, or `test` for a message sent with Send test.
- `severity`: `info`, `warning` or `critical`.
- `title`: a short text describing the event.
- `body`: more details.
- `link`: the URL to open in the app (empty if `REEFSHEPHERD_PUBLIC_URL` is not set).
- `tank`: null for system-wide events like backups, otherwise an object with the tank ID and name.
- `occurred_at`: the UTC time the notification was raised.

In ReefShepherd, paste your webhook URL under "URL".

#### Email

Email is sent through an SMTP server. You can use a mail provider like Gmail or Fastmail, or a local relay on your network.

Set these fields:
- **Mail server**: the SMTP server address (e.g. `smtp.gmail.com`, `127.0.0.1` for a local relay).
- **Port**: the SMTP port, usually 587 (STARTTLS) or 465 (implicit TLS), 25 for a local relay.
- **Encryption**: how to connect.
  - `STARTTLS (starts plain, then switches to encryption)`: connect plaintext, then upgrade to TLS. Usually port 587.
  - `TLS (encrypted from the start)`: connect directly with TLS. Usually port 465.
  - `None (only for a server on this machine)`: no encryption. Works for a local relay on the same machine. If you set a username, the server must be on a loopback address.
- **Username (optional)** and **Password (optional)**: your login (may be empty for a local relay).
- **From address**: the email address the messages come from (you must be able to send from this address).
- **To**: a list of email addresses to receive notifications, separated by commas.

Examples:

Gmail (with 2-step verification enabled):
- Mail server: `smtp.gmail.com`
- Port: 587
- Encryption: `STARTTLS (starts plain, then switches to encryption)`
- Username: `your-email@gmail.com`
- Password: generate an app password at https://myaccount.google.com/apppasswords
- From address: `your-email@gmail.com`

Fastmail:
- Mail server: `smtp.fastmail.com`
- Port: 465
- Encryption: `TLS (encrypted from the start)`
- Username: `your-email@fastmail.com`
- Password: generate an app password at https://app.fastmail.com/settings/security
- From address: `your-email@fastmail.com`

Local relay (on the same machine):
- Mail server: `127.0.0.1`
- Port: 25
- Encryption: `None (only for a server on this machine)`
- Username: (empty)
- Password: (empty)
- From address: `reefshepherd@localhost`

## Links in notifications

Notifications can include a link to the relevant part of ReefShepherd (like a task or a chart). For the link to work, set `REEFSHEPHERD_PUBLIC_URL` in your environment to the address you use to open the app. For example, if you open ReefShepherd at `http://192.168.1.50:8484`, set:

```
REEFSHEPHERD_PUBLIC_URL=http://192.168.1.50:8484
```

Without a public URL, links are empty in channel messages (the inbox still shows working links).

## Retries

If a channel fails to send a notification, the app tries again:

1. First attempt (right away)
2. Retry after 1 minute
3. Retry after 10 minutes
4. Retry after 1 hour

If it still failed after the fourth attempt, the delivery is marked as failed.

If a provider responds with a `Retry-After` header asking for a longer delay, the app honours it (up to 24 hours), which can only lengthen the wait between retries.

Discord stops retrying when it returns HTTP 404 (webhook URL no longer exists). Slack stops when it returns 404, no_service, no_service_id, no_active_hooks or channel_not_found (the webhook was removed or the channel is gone). Webhook and ntfy channels retry normally on any error.

## Where to see errors

If a notification fails to send, the error appears on the Notifications settings page next to the channel name, as "The last attempt failed {when}: {error}". It is updated after every failed attempt, showing the provider's error message and when the last failure occurred.

The "Send test" button sends a test message immediately and shows the result on the page. If it fails, you see "The test message was not sent: {error}" right away but it does not change the channel's real last error or last sent time.

All events, including failures, are logged to the system journal.
