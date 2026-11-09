// Rebuilds the vendored Codex editor and map libraries. The output is committed;
// run this only when changing versions in package.json.
//
//   cd tools/vendor && npm ci && npm run build
//
// Writes static/vendor/codemirror.bundle.js (global `CM`, see entry.js) and
// static/vendor/leaflet/{leaflet.js,leaflet.css,LICENSE}.
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

const lf = path.join(here, 'node_modules/leaflet');
const dest = path.join(out, 'leaflet');
mkdirSync(dest, { recursive: true });
copyFileSync(path.join(lf, 'dist/leaflet.js'), path.join(dest, 'leaflet.js'));
copyFileSync(path.join(lf, 'dist/leaflet.css'), path.join(dest, 'leaflet.css'));
copyFileSync(path.join(lf, 'LICENSE'), path.join(dest, 'LICENSE'));
console.log('vendor build done');
