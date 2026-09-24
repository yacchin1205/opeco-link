import { copyFile, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { marked } from "marked";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const output = join(root, ".docs-dist");
const repository = "https://github.com/yacchin1205/opeco-link/blob/main/";
const pages = [
  { source: "PRIVACY.md", name: "privacy" },
  { source: "README.md", name: "support" },
];
const images = ["brand/readme-icon.svg", "brand/architecture.svg"];

// Relative links point at repository files that the site does not carry.
const renderer = {
  link({ href, title, tokens }) {
    const target = /^(https?:|mailto:|#)/.test(href) ? href : repository + href;
    const text = this.parser.parseInline(tokens);
    return `<a href="${target}"${title ? ` title="${title}"` : ""}>${text}</a>`;
  },
};
marked.use({ gfm: true, renderer });

function page(title, body) {
  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${title}</title>
<style>
  :root { color-scheme: light dark; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
  body { margin: 0 auto; max-width: 760px; padding: 24px 16px 48px; line-height: 1.6; }
  h1 { font-size: 1.6rem; } h2 { font-size: 1.25rem; margin-top: 2em; } h3 { font-size: 1.05rem; }
  pre { overflow-x: auto; padding: 12px; border-radius: 8px; background: rgba(127, 127, 127, 0.12); }
  code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 0.92em; }
  table { border-collapse: collapse; display: block; overflow-x: auto; }
  th, td { border: 1px solid rgba(127, 127, 127, 0.4); padding: 6px 10px; text-align: left; vertical-align: top; }
  img { max-width: 100%; height: auto; }
</style>
</head>
<body>
${body}</body>
</html>
`;
}

await rm(output, { recursive: true, force: true });
await mkdir(join(output, "brand"), { recursive: true });
for (const image of images) await copyFile(join(root, image), join(output, image));
for (const { source, name } of pages) {
  const markdown = await readFile(join(root, source), "utf8");
  const heading = /^# (.+)$/m.exec(markdown);
  if (heading === null) throw new Error(`${source} has no top-level heading`);
  await writeFile(join(output, `${name}.html`), page(heading[1], await marked.parse(markdown)));
}
await writeFile(join(output, "_redirects"), "/ /support 302\n");
