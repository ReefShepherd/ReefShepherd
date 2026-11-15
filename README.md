# ReefShepherd
A self hosted app for Saltwater Aquarium enthusiasts. Track your parameters, maintenance &amp; feeding schedules, and so much more, all privately on your own hardware.

## Build

Requires Go 1.27 and, once the web UI exists, Node and npm.

```sh
make build        # builds the web UI (when present) and bin/reefshepherd
./bin/reefshepherd serve
make test         # Go tests, plus web tests when present
make lint
```

`serve` listens on `:8484` and keeps its data in `./data`; see `deploy/env/reefshepherd.env.example` for every setting.

## License

ReefShepherd is licensed under the GNU Affero General Public License, version 3 or (at your option) any later version (AGPL-3.0-or-later). See [LICENSE](LICENSE).
