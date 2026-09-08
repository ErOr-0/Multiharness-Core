# Multiharness product website

A responsive React website for Multiharness Core. It introduces the local CLI,
shows the plan → implement → validate → review/repair workflow, and links to the
real repository, documentation, and releases.

## Run locally

Use Node.js 22.12 or newer.

```sh
npm install
npm run dev
```

Open the URL printed by Vite. To build and inspect the static production website:

```sh
npm run format
npm run build
npm run preview
```

Deploy the contents of `dist/` to a static host. No backend, API keys, model
credentials, external fonts, or environment variables are needed. Configure Vite's
`base` if hosting under a URL subdirectory rather than a domain root.

## Content and behavior

- `src/App.jsx`: navigation, explorable workflow, feature sections, installation,
  FAQ and footer.
- `src/components/WorkflowDemo.jsx`: deliberately simulated, cancellable workflow
  preview. It never invokes agents or executes the displayed commands.
- `src/components/Roadmap.jsx`: responsive development and upcoming-work lineup.
- `src/content.js`: repository URLs, workflow examples, FAQ copy and roadmap items.
- `src/styles.css`: design tokens, components, mobile breakpoints and reduced motion.
- Fonts are bundled locally through Fontsource; icons come from Lucide.

The planner, builder and reviewer selectors independently change only the displayed
example configuration. The builder can show big-pickle or the configured OpenCode
default; the site does not query live model catalogs. The
setup copy button copies the selected platform's Docker image extraction and
launcher commands. Docker Hub is the primary download path; the native binary
and source-build guide remains an alternative. Keep the image reference and
launcher commands in `src/content.js` aligned with `../docs/docker.md`. The site
explains project mounts, separate container login and persistent state. It does
not promise zero setup, native Windows execution, universally verified platform
support, free model usage or guaranteed agent approval.

## Roadmap content

Update the `roadmap` array in `src/content.js` when work changes status.
The current under-development lane describes release verification with existing
implementation; the coming-next lane describes planned primary Codex builder
selection and first-launch guidance. Local settings already work through `/config`
and `/save`; the roadmap describes extending that flow, not adding persistence
from scratch. No release dates or native Windows support are promised.

## Browser checks

Run `npm run test:content` for browser-free checks of the rendered Docker setup
and every platform command against the shipped Docker guide.

```sh
npx playwright install chromium
npm run test:e2e
```

To use an installed Chrome instead of downloading Chromium, set
`PLAYWRIGHT_CHANNEL=chrome` for the test process. Tests exercise the workflow demo,
stage tabs, mobile menu, platform selection, FAQ, clipboard, external links, and
responsive overflow, and automated WCAG accessibility checks. Screenshots are available through Playwright's test artifacts.
