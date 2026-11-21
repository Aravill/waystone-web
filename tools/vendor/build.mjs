// Rebuilds the vendored Codex libraries: the editor, the map and the graph layout. The output is committed;
// run this only when changing versions in package.json.
//
//   cd tools/vendor && npm ci && npm run build
//
// Writes static/vendor/codemirror.bundle.js (global `CM`, see entry.js) and
// static/vendor/leaflet/{leaflet.js,leaflet.css,LICENSE} and static/vendor/d3-force.bundle.js
// (global `D3F`, see entry-d3.js; d3-force is ISC, the banner carries the notice).
import { build } from 'esbuild';
import { readFileSync, mkdirSync, copyFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const here = path.dirname(fileURLToPath(import.meta.url));
const out = path.join(here, '../../static/vendor');
const pkg = JSON.parse(readFileSync(path.join(here, 'package.json'), 'utf8'));
const versions = Object.entries(pkg.dependencies)
  .filter(([n]) => n.startsWith('@') )
  .map(([n, v]) => `${n}@${v}`).join(', ');

await build({
  entryPoints: [path.join(here, 'entry.js')],
  bundle: true,
  minify: true,
  format: 'iife',
  globalName: 'CM',
  outfile: path.join(out, 'codemirror.bundle.js'),
  banner: { js: `/* CodeMirror 6 bundle (MIT): ${versions}. Built by tools/vendor/build.mjs */` },
  legalComments: 'none',
});

await build({
  entryPoints: [path.join(here, 'entry-d3.js')],
  bundle: true,
  minify: true,
  format: 'iife',
  globalName: 'D3F',
  outfile: path.join(out, 'd3-force.bundle.js'),
  banner: { js: `/* d3-force ${pkg.dependencies['d3-force']} (ISC License, Copyright 2010-2021 Mike Bostock, https://github.com/d3/d3-force). Built by tools/vendor/build.mjs */` },
  legalComments: 'none',
});

const lf = path.join(here, 'node_modules/leaflet');
const dest = path.join(out, 'leaflet');
mkdirSync(dest, { recursive: true });
copyFileSync(path.join(lf, 'dist/leaflet.js'), path.join(dest, 'leaflet.js'));
copyFileSync(path.join(lf, 'dist/leaflet.css'), path.join(dest, 'leaflet.css'));
copyFileSync(path.join(lf, 'LICENSE'), path.join(dest, 'LICENSE'));
console.log('vendor build done');
