import { copyFile, mkdir, rm } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const source = join(root, "web");
const output = join(root, ".web-dist");
const staticFiles = [
  "_headers",
  ".well-known/apple-app-site-association",
  "icon-192.png",
  "icon-512.png",
  "index.html",
  "manifest.webmanifest",
  "robots.txt",
  "styles.css",
  "sw.js",
  "third-party-notices.txt",
];

await rm(output, { recursive: true, force: true });
await mkdir(output);
await Promise.all(staticFiles.map(async (name) => {
  const destination = join(output, name);
  await mkdir(dirname(destination), { recursive: true });
  await copyFile(join(source, name), destination);
}));
await copyFile(join(root, "brand/icon-ios.svg"), join(output, "icon.svg"));
await copyFile(join(root, "brand/favicon.svg"), join(output, "favicon.svg"));
await mkdir(join(output, "opeco"));
const iosAssets = join(root, "ios/Opeco/Assets.xcassets");
await copyFile(join(iosAssets, "OpecoEmpty.imageset/opeco-outline.svg"), join(output, "opeco/outline.svg"));
for (const color of ["blue", "cyan", "green", "orange", "red", "yellow", "purple", "pink"]) {
  const suffix = color === "blue" ? "" : color[0].toUpperCase() + color.slice(1);
  const filename = color === "blue" ? "opeco-filled.svg" : `opeco-filled-${color}.svg`;
  await copyFile(join(iosAssets, `OpecoSession${suffix}.imageset`, filename), join(output, `opeco/${color}.svg`));
}
await build({
  entryPoints: [join(source, "app.js")],
  bundle: true,
  format: "esm",
  outfile: join(output, "app.js"),
  platform: "browser",
  target: "es2022",
  banner: {
    js: "/* QR Code Generator for JavaScript, Copyright (c) 2009 Kazuhiko Arase, MIT License. See /third-party-notices.txt */",
  },
});
