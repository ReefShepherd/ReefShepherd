# Backups and restore

ReefShepherd automatically backs up your database every night. You can also make manual backups, and restore from any backup if something goes wrong.

## What is backed up

Only the database is backed up: the file `reefshepherd.db` containing all your tanks, parameters, test results, maintenance tasks, users and settings. Photos are not backed up (they stay on the machine where the app runs). Other backups of the whole data directory, including photos, belong to your regular system backup.

Backups contain password hashes and the secrets of notification channels (webhook URLs, email passwords, etc.), so keep backups in a secure place or off the machine.

## How backups happen

Each night, at the time you set in Settings > Backups (default 03:30 in your local timezone), the app makes a backup if backups are enabled. At most one scheduled backup is made per local day (the backup job runs once daily and is skipped if a backup (scheduled or Back up now) was already made after today's backup time). If the app is not running at the scheduled time, a backup is made the next time the app starts, on the first tick after that time has passed in the instance timezone.

Backups are kept in the `backups/` folder in the data directory. By default, the 14 newest scheduled backups are kept; older ones are deleted. The app also keeps:

- The 5 newest backups made before a database migration (`premigrate` kind), so you can restore an older version if a migration causes problems.
- The 5 newest copies of the database as it was just before a restore (`prerestore` kind), so you can undo a restore.

## Backup file names

Backup file names show their kind and the UTC time they were made:

- Scheduled backups (made at night or on demand): `reefshepherd-20241217T033051Z.db`
- Before a migration (made automatically at the start of an upgrade): `reefshepherd-premigrate-00002-20241217T030000Z.db`
- Before a restore (made when you run `reefshepherd restore`): `reefshepherd-prerestore-20241217T033000Z.db`

The five-digit number in `premigrate` file names is the target schema version (the last migration pending). Premigrate backups are made only when the app detects pending migrations and the database already has data.

## Managing backups

Open Settings > Backups to see a list of all backups, their kind, size and creation time.

From this page you can:

- **Back up now**: make an immediate backup. It counts toward the keep limit along with scheduled backups.
- **Download**: save a backup to your computer. The file is named the same as shown.
- **Delete**: remove a backup.

## Restore from a backup

Restoring replaces the current database with a backup, so use it only when something has gone wrong. Make sure no one is using the app while you restore.

### With systemd

1. Stop the server:
```sh
systemctl stop reefshepherd
```

2. Restore the backup as the service user:
```sh
runuser -u reefshepherd -- reefshepherd restore /var/lib/reefshepherd/backups/reefshepherd-20241217T033051Z.db --config /etc/reefshepherd/reefshepherd.env --data-dir /var/lib/reefshepherd
```
The --data-dir flag tells the restore command where the app's database and backups folders are located, since the restore command defaults to ./data if not told otherwise.

3. Start the server again:
```sh
systemctl start reefshepherd
```

### Without systemd

Stop the app however you normally do, then run:

```
reefshepherd restore /path/to/backup.db --data-dir /path/to/data
```

Then restart the app.

## What restore does

Restore will refuse to run if:

- The server is running (listening on the configured address).
- The backup file is corrupt or is not a valid ReefShepherd database.
- The backup file is the current database.
- The backup was made by a newer version of ReefShepherd and cannot be opened by this binary.

If you pass these checks, restore will:

1. Move the current database to the `backups/` folder as `reefshepherd-prerestore-<UTC time>.db`.
2. Put the backup file in place of the current database.
3. Sync the data directory to disk.

If the backup is from an older schema version, that is fine. When the app starts, it will apply any newer migrations (and write a `premigrate-<version>` backup before all pending migrations).

The old database is never deleted, so if you restore by mistake you can restore again from the `prerestore` backup to undo it.

## Restore permissions

The restore command must be run as the service user (or the user that will run the app). If you run it as root, the restored database file becomes root-owned and the service user cannot read it, and the app will fail to start.

Run restore as the service user:

```sh
runuser -u reefshepherd -- reefshepherd restore /var/lib/reefshepherd/backups/reefshepherd-20241217T033051Z.db --data-dir /var/lib/reefshepherd
```

Or log in as that user first, then run the command.

## Testing your backups

To make sure your backups are good and restore works, try restoring to a test machine or to a separate folder on the same machine, using a different `--data-dir`. This way you can check that the old data comes back without affecting the live app.
