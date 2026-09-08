# Multiharness website

React/Vite product site. Run `npm ci`, `npm run dev`; `npm run build` creates
static `dist/` output. The prebuild/predev step packages the maintained Compose
and sandbox files into one standard ZIP using `scripts/package-docker.py`.
There are no platform-specific host executables or browser ZIP/config generators.

GettingStarted shows one installation path: download, run setup, configure.
Setup already downloads the image and applies the required Docker policies; do
not add a separate pull step. All three steps and the everyday
`docker start -ai multiharness` command fit together on desktop screens
(1024×768 and larger tested); narrow screens use readable stacked steps. Keep
its commands aligned with `docs/docker.md` and `compose.yaml`. The workflow preview is a simulation and makes no model calls.

Run `npm run test:content` for configuration/guide/package checks and
`npm run test:e2e` for downloads, clipboard, accessibility and responsive behavior.
Use `PLAYWRIGHT_CHANNEL=chrome` to use installed Chrome. Deploy `dist/` to the
existing static host; no backend, secrets or new service are required.
