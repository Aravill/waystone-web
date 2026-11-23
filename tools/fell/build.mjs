// Builds static/vendor/fonts/ws-fell-400-latin-ext.woff2: the Latin Extended letters IM Fell English SC
// lacks (č ě š ř ž ů ł ą ő ...), composed from the vendored latin file's own letters and accents.
// Run: node tools/fell/build.mjs (no dependencies). The font is under the SIL Open Font License with the
// reserved name "IM FELL English SC", so the derived file has its own name, "Waystone Fell".
import fs from 'node:fs';
import zlib from 'node:zlib';

const DIR = new URL('../../static/vendor/fonts/', import.meta.url);
const SRC = new URL('im-fell-english-sc-400-latin.woff2', DIR);
const OUT = new URL('ws-fell-400-latin-ext.woff2', DIR);

const TAGS = ['cmap', 'head', 'hhea', 'hmtx', 'maxp', 'name', 'OS/2', 'post', 'cvt ', 'fpgm', 'glyf', 'loca', 'prep', 'CFF ', 'VORG', 'EBDT', 'EBLC', 'gasp', 'hdmx', 'kern', 'LTSH', 'PCLT', 'VDMX', 'vhea', 'vmtx', 'BASE', 'GDEF', 'GPOS', 'GSUB'];

/* ---------- read the woff2 ---------- */
function readWoff2(buf) {
    let o = 48;
    const base128 = () => { let r = 0; for (;;) { const c = buf[o++]; r = r * 128 + (c & 127); if (!(c & 128)) return r; } };
    const dir = [];
    for (let i = 0; i < buf.readUInt16BE(12); i++) {
        const f = buf[o++];
        let tag = TAGS[f & 63];
        if ((f & 63) === 63) { tag = buf.toString('latin1', o, o + 4); o += 4; }
        const orig = base128(), v = f >> 6;
        const transformed = tag === 'glyf' || tag === 'loca' ? v !== 3 : v !== 0;
        dir.push({ tag, transformed, len: transformed ? base128() : orig });
    }
    const data = zlib.brotliDecompressSync(buf.subarray(o, o + buf.readUInt32BE(20)));
    const t = {};
    let p = 0;
    for (const d of dir) { t[d.tag] = data.subarray(p, p + d.len); t[d.tag].transformed = d.transformed; p += d.len; }
    return t;
}

function readGlyphs(g) {
    const n = g.readUInt16BE(4);
    let p = 36;
    const [nc, np, fl, gl, co] = [8, 12, 16, 20, 24].map((at) => {
        const len = g.readUInt32BE(at), s = { b: g.subarray(p, p + len), o: 0 };
        p += len;
        return s;
    });
    const u8 = (x) => x.b[x.o++];
    const u16 = (x) => { const v = x.b.readUInt16BE(x.o); x.o += 2; return v; };
    const i16 = (x) => { const v = x.b.readInt16BE(x.o); x.o += 2; return v; };
    const u255 = (x) => { const c = u8(x); if (c === 253) return u16(x); if (c === 255) return u8(x) + 253; if (c === 254) return u8(x) + 506; return c; };
    const sign = (f, v) => (f & 1 ? v : -v);
    const out = [];
    for (let i = 0; i < n; i++) {
        const contours = nc.b.readInt16BE(i * 2);
        if (contours === 0) { out.push([]); continue; }
        if (contours > 0) {
            const counts = Array.from({ length: contours }, () => u255(np));
            let x = 0, y = 0;
            out.push(counts.map((c) => Array.from({ length: c }, () => {
                const raw = u8(fl), f = raw & 127, on = !(raw & 128);
                let dx, dy;
                if (f < 10) { dx = 0; dy = sign(f, ((f & 14) << 7) + u8(gl)); }
                else if (f < 20) { dx = sign(f, (((f - 10) & 14) << 7) + u8(gl)); dy = 0; }
                else if (f < 84) { const b0 = f - 20, b1 = u8(gl); dx = sign(f, 1 + (b0 & 48) + (b1 >> 4)); dy = sign(f >> 1, 1 + ((b0 & 12) << 2) + (b1 & 15)); }
                else if (f < 120) { const b0 = f - 84; dx = sign(f, 1 + (Math.floor(b0 / 12) << 8) + u8(gl)); dy = sign(f >> 1, 1 + (((b0 % 12) >> 2) << 8) + u8(gl)); }
                else if (f < 124) { const a = u8(gl), b = u8(gl); dx = sign(f, (a << 4) + (b >> 4)); dy = sign(f >> 1, ((b & 15) << 8) + u8(gl)); }
                else { dx = sign(f, u16(gl)); dy = sign(f >> 1, u16(gl)); }
                x += dx; y += dy;
                return { x, y, on };
            })));
            u255(gl); // instruction length (instructions are dropped)
            continue;
        }
        const parts = [];
        let flags, instr = false;
        do {
            flags = u16(co);
            const index = u16(co);
            let dx, dy;
            if (flags & 1) { dx = i16(co); dy = i16(co); } else { dx = co.b.readInt8(co.o++); dy = co.b.readInt8(co.o++); }
            const f2 = () => i16(co) / 16384;
            let m = [1, 0, 0, 1];
            if (flags & 8) { const s = f2(); m = [s, 0, 0, s]; } else if (flags & 64) { m = [f2(), 0, 0, f2()]; } else if (flags & 128) { m = [f2(), f2(), f2(), f2()]; }
            if (flags & 256) instr = true;
            parts.push({ index, dx, dy, m });
        } while (flags & 32);
        if (instr) u255(gl);
        out.push({ parts });
    }
    const flat = (k) => (Array.isArray(out[k]) ? out[k] : out[k].parts.flatMap(({ index, dx, dy, m }) =>
        flat(index).map((c) => c.map((q) => ({ x: Math.round(q.x * m[0] + q.y * m[2] + dx), y: Math.round(q.x * m[1] + q.y * m[3] + dy), on: q.on })))));
    return out.map((_, k) => flat(k));
}

function readCmap(c) {
    const map = new Map();
    for (let i = 0; i < c.readUInt16BE(2); i++) {
        const q = c.readUInt32BE(8 + i * 8);
        if (c.readUInt16BE(q) !== 4) continue;
        const sc = c.readUInt16BE(q + 6) / 2;
        for (let k = 0; k < sc; k++) {
            const end = c.readUInt16BE(q + 14 + k * 2), start = c.readUInt16BE(q + 16 + sc * 2 + k * 2);
            const delta = c.readUInt16BE(q + 16 + sc * 4 + k * 2), ro = q + 16 + sc * 6 + k * 2, off = c.readUInt16BE(ro);
            for (let ch = start; ch <= end && ch < 65535; ch++) {
                let gid = off ? c.readUInt16BE(ro + off + (ch - start) * 2) : ch;
                if (gid || !off) gid = (gid + delta) & 65535;
                if (gid) map.set(ch, gid);
            }
        }
    }
    return map;
}

const t = readWoff2(fs.readFileSync(SRC));
if (!t.glyf.transformed) throw new Error('expected a transformed glyf table');
const glyphs = readGlyphs(t.glyf);
const cmap = readCmap(t.cmap);
const nHM = t.hhea.readUInt16BE(34);
// A transformed hmtx starts with a flags byte, then lists the advances alone.
const adv = (gid) => { const i = Math.min(gid, nHM - 1); return t.hmtx.transformed ? t.hmtx.readUInt16BE(1 + i * 2) : t.hmtx.readUInt16BE(i * 4); };

/* ---------- compose ---------- */
const gidOf = (ch) => { const g = cmap.get(ch.codePointAt(0)); if (!g) throw new Error('no glyph for ' + ch); return g; };
const shape = (ch) => glyphs[gidOf(ch)];
const box = (cs) => {
    const xs = cs.flat().map((p) => p.x), ys = cs.flat().map((p) => p.y);
    const l = Math.min(...xs), r = Math.max(...xs), b = Math.min(...ys), top = Math.max(...ys);
    return { l, r, b, t: top, w: r - l, h: top - b };
};
const move = (cs, dx, dy) => cs.map((c) => c.map((p) => ({ x: p.x + dx, y: p.y + dy, on: p.on })));
// A mirrored shape is reversed, so its outline still winds the right way.
const scale = (cs, sx, sy) => cs.map((c) => { const r = c.map((p) => ({ x: p.x * sx, y: p.y * sy, on: p.on })); return sx * sy < 0 ? r.reverse() : r; });
const contour = (...pts) => [pts.map(([x, y, off]) => ({ x, y, on: !off }))]; // clockwise
const cap = (ch) => ch === ch.toUpperCase();
const small = (ch) => (cap(ch) ? 1 : 0.8);
const k = t.head.readUInt16BE(18) / 1000;

// How far the font's own accents float above a capital and a small capital.
const gapOver = (plain, accented) => box(shape(accented)).t - box(shape(plain)).t - box(shape('´')).h;
const GAP = { cap: Math.max(gapOver('E', 'É'), 25 * k), low: Math.max(gapOver('e', 'é'), 25 * k) };

const MARKS = {
    caron: () => scale(shape('ˆ'), 1, -1),
    circ: () => shape('ˆ'),
    acute: () => shape('´'),
    ring: () => shape('˚'),
    tilde: () => shape('˜'),
    dot: () => shape('.'),
    breve: () => contour([-150 * k, 130 * k], [-125 * k, 130 * k], [-95 * k, 48 * k, 1], [0, 48 * k], [95 * k, 48 * k, 1], [125 * k, 130 * k], [150 * k, 130 * k], [120 * k, 0, 1], [0, 0], [-120 * k, 0, 1]),
    macron: () => scale(shape('–'), (260 * k) / box(shape('–')).w, 1),
    dacute: () => { const a = shape('´'); return [...a, ...move(a, box(a).w * 0.85, 0)]; },
};
const above = (base, mark) => {
    const b = box(shape(base)), m = MARKS[mark](), q = box(m);
    const lift = { caron: 25, circ: 25, dot: 20, macron: 35 }[mark] || 0;
    const room = GAP[cap(base) ? 'cap' : 'low'] + lift * k;
    return [...shape(base), ...move(m, (b.l + b.r) / 2 - (q.l + q.r) / 2, b.t + room - q.b)];
};
const below = (base, mark, at = 0.5) => {
    const b = box(shape(base)), s = small(base);
    const m = mark === 'ogonek' ? scale(shape('¸'), -s, s) : mark === 'cedilla' ? scale(shape('¸'), s, s) : scale(shape(','), 0.75 * s, 0.75 * s);
    const q = box(m);
    return [...shape(base), ...move(m, b.l + b.w * at - (q.l + q.r) / 2, mark === 'comma' ? -45 * k - q.t : 15 * k - q.t)];
};
const bar = (base, from, to, y, tilt = 0) => {
    const b = box(shape(base)), x0 = b.l + b.w * from, x1 = b.l + b.w * to, mid = b.b + b.h * y, th = 24 * k, dy = b.h * tilt;
    return [...shape(base), ...contour([x0, mid - dy - th], [x0, mid - dy + th], [x1, mid + dy + th], [x1, mid + dy - th])];
};
const apostrophe = (base) => { // L with caron is written with an apostrophe
    const b = box(shape(base)), m = scale(shape('’'), 0.85 * small(base), 0.85 * small(base)), q = box(m);
    return [...shape(base), ...move(m, b.l + b.w * 0.62 - q.l, b.t - q.t)];
};

const made = new Map();
const add = (ch, cs, base) => made.set(ch.codePointAt(0), { cs: cs.map((c) => c.map((p) => ({ x: Math.round(p.x), y: Math.round(p.y), on: p.on }))), adv: adv(gidOf(base)) });
const each = (list, make) => { for (const s of list.split(' ')) { const [ch, base] = [...s]; add(ch, make(base), base); } };
each('ČC čc ĎD ďd ĚE ěe ŇN ňn ŘR řr ŠS šs ŤT ťt ŽZ žz ǍA ǎa ǓU ǔu ǦG ǧg', (b) => above(b, 'caron'));
each('ĆC ćc ĹL ĺl ŃN ńn ŔR ŕr ŚS śs ŹZ źz ǴG ǵg', (b) => above(b, 'acute'));
each('ŮU ůu', (b) => above(b, 'ring'));
each('ŐO őo ŰU űu', (b) => above(b, 'dacute'));
each('ĊC ċc ĖE ėe ĠG ġg İI ŻZ żz', (b) => above(b, 'dot'));
each('ĂA ăa ĔE ĕe ĞG ğg ĬI ĭi ŎO ŏo ŬU ŭu', (b) => above(b, 'breve'));
each('ĀA āa ĒE ēe ĪI īi ŌO ōo ŪU ūu', (b) => above(b, 'macron'));
each('ĈC ĉc ĜG ĝg ĤH ĥh ĴJ ĵj ŜS ŝs ŴW ŵw ŶY ŷy', (b) => above(b, 'circ'));
each('ĨI ĩi ŨU ũu', (b) => above(b, 'tilde'));
each('ĄA ąa', (b) => below(b, 'ogonek', 0.86));
each('ĘE ęe', (b) => below(b, 'ogonek', 0.7));
each('ĮI įi ŲU ųu', (b) => below(b, 'ogonek', 0.58));
each('ŞS şs ŢT ţt', (b) => below(b, 'cedilla'));
each('ȘS șs ȚT țt ĢG ģg ĶK ķk ĻL ļl ŅN ņn ŖR ŗr', (b) => below(b, 'comma'));
each('ĽL ľl', apostrophe);
each('ŁL łl', (b) => bar(b, -0.04, 0.6, 0.52, 0.1));
each('ĐD đd', (b) => bar(b, -0.06, 0.42, 0.52));
each('ĦH ħh', (b) => bar(b, -0.02, 1.02, 0.76));
each('ŦT ŧt', (b) => bar(b, 0.24, 0.76, 0.45));

/* ---------- write the font ---------- */
const codes = [...made.keys()].sort((a, b) => a - b);
const list = [{ cs: [], adv: Math.round(500 * k) }, ...codes.map((c) => made.get(c))];
const i16 = (v) => { const b = Buffer.alloc(2); b.writeInt16BE(v); return b; };
const u16 = (...v) => Buffer.concat(v.map((x) => { const b = Buffer.alloc(2); b.writeUInt16BE(x & 65535); return b; }));
const u32 = (v) => { const b = Buffer.alloc(4); b.writeUInt32BE(v >>> 0); return b; };
const pad4 = (b) => Buffer.concat([b, Buffer.alloc((4 - (b.length % 4)) % 4)]);
const boxes = list.map((g) => (g.cs.length ? box(g.cs) : { l: 0, r: 0, b: 0, t: 0 }));

const glyfs = list.map((g, i) => {
    if (!g.cs.length) return Buffer.alloc(0);
    const pts = g.cs.flat(), b = boxes[i];
    let end = -1, px = 0, py = 0;
    const flags = Buffer.from(pts.map((p, j) => (p.on ? 1 : 0) | (j === 0 ? 64 : 0))); // 64: contours may overlap
    const xs = pts.map((p) => { const d = p.x - px; px = p.x; return i16(d); });
    const ys = pts.map((p) => { const d = p.y - py; py = p.y; return i16(d); });
    return pad4(Buffer.concat([i16(g.cs.length), i16(b.l), i16(b.b), i16(b.r), i16(b.t), ...g.cs.map((c) => u16((end += c.length))), u16(0), flags, ...xs, ...ys]));
});
let off = 0;
const loca = Buffer.concat([...glyfs.map((b) => { const v = u32(off); off += b.length; return v; }), u32(off)]);

const n = codes.length + 1, range = 2 ** Math.floor(Math.log2(n)) * 2;
const zeros = codes.map(() => 0);
const sub = Buffer.concat([u16(4, 16 + n * 8, 0, n * 2, range, Math.log2(range / 2), n * 2 - range), u16(...codes, 65535), u16(0), u16(...codes, 65535), u16(...codes.map((c, i) => i + 1 - c), 1), u16(...zeros, 0)]);

const NAMES = {
    0: '© 2007 Igino Marini (www.iginomarini.com), with Reserved Font Name IM FELL English SC. Latin Extended letters composed from it for Waystone.',
    1: 'Waystone Fell', 2: 'Regular', 3: 'Waystone Fell 1.0', 4: 'Waystone Fell', 5: 'Version 1.000', 6: 'WaystoneFell-Regular',
    13: 'This Font Software is licensed under the SIL Open Font License, Version 1.1.', 14: 'http://scripts.sil.org/OFL',
};
let so = 0;
const strs = Object.entries(NAMES).map(([id, v]) => { const b = Buffer.from(v, 'utf16le').swap16(); const rec = u16(3, 1, 1033, Number(id), b.length, so); so += b.length; return { rec, b }; });
const name = Buffer.concat([u16(0, strs.length, 6 + strs.length * 12), ...strs.map((s) => s.rec), ...strs.map((s) => s.b)]);

const real = boxes.slice(1);
const head = Buffer.from(t.head);
head.writeUInt32BE(0, 8);
head.writeUInt16BE(head.readUInt16BE(16) & ~0x1c, 16); // no hinting
head.writeInt16BE(Math.min(...real.map((b) => b.l)), 36);
head.writeInt16BE(Math.min(...real.map((b) => b.b)), 38);
head.writeInt16BE(Math.max(...real.map((b) => b.r)), 40);
head.writeInt16BE(Math.max(...real.map((b) => b.t)), 42);
head.writeInt16BE(1, 50); // long loca
const hhea = Buffer.from(t.hhea);
hhea.writeUInt16BE(Math.max(...list.map((g) => g.adv)), 10);
hhea.writeUInt16BE(list.length, 34);
const maxp = Buffer.concat([u32(0x10000), u16(list.length, Math.max(...list.map((g) => g.cs.flat().length)), Math.max(...list.map((g) => g.cs.length)), 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0)]);
const os2 = Buffer.from(t['OS/2']);
os2.writeUInt16BE(codes[0], 64);
os2.writeUInt16BE(codes.at(-1), 66);
const post = Buffer.concat([u32(0x30000), t.post.subarray(4, 32)]);
const hmtx = Buffer.concat(list.map((g, i) => Buffer.concat([u16(g.adv), i16(boxes[i].l)])));

const tables = { 'OS/2': os2, cmap: Buffer.concat([u16(0, 1, 3, 1), u32(12), sub]), glyf: Buffer.concat(glyfs), loca, head, hhea, hmtx, maxp, name, post };
const order = Object.keys(tables); // loca has to follow glyf in a woff2

// head.checkSumAdjustment, over the sfnt these tables unpack to
const sum = (b) => { const p = pad4(b); let s = 0; for (let i = 0; i < p.length; i += 4) s = (s + p.readUInt32BE(i)) >>> 0; return s; };
const sorted = [...order].sort();
const exp = Math.floor(Math.log2(sorted.length));
let at = 12 + 16 * sorted.length;
const sfnt = [u32(0x10000), u16(sorted.length, 16 * 2 ** exp, exp, sorted.length * 16 - 16 * 2 ** exp)];
for (const tag of sorted) { sfnt.push(Buffer.from(tag), u32(sum(tables[tag])), u32(at), u32(tables[tag].length)); at += pad4(tables[tag]).length; }
const total = sorted.reduce((s, tag) => s + sum(tables[tag]), sum(Buffer.concat(sfnt))) >>> 0;
head.writeUInt32BE((0xb1b0afba - total) >>> 0, 8);

const base128 = (v) => { const out = [v & 127]; while ((v = Math.floor(v / 128))) out.unshift((v & 127) | 128); return Buffer.from(out); };
const dir = Buffer.concat(order.map((tag) => Buffer.concat([Buffer.from([TAGS.indexOf(tag) | (tag === 'glyf' || tag === 'loca' ? 0xc0 : 0)]), base128(tables[tag].length)]))); // 0xc0: not transformed
const packed = zlib.brotliCompressSync(Buffer.concat(order.map((tag) => tables[tag])), { params: { [zlib.constants.BROTLI_PARAM_QUALITY]: 11, [zlib.constants.BROTLI_PARAM_MODE]: zlib.constants.BROTLI_MODE_FONT } });
const size = 48 + dir.length + packed.length, padded = size + ((4 - (size % 4)) % 4);
fs.writeFileSync(OUT, pad4(Buffer.concat([Buffer.from('wOF2'), u32(0x10000), u32(padded), u16(sorted.length, 0), u32(at), u32(packed.length), u16(1, 0), u32(0), u32(0), u32(0), u32(0), u32(0), dir, packed])));
console.log(`${codes.length} letters, ${padded} bytes`);
