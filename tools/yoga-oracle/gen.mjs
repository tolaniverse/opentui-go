// Generates ../../testdata/yoga_cases.json: layout trees plus the rects
// real Yoga (the engine upstream OpenTUI uses) computes for them.
// layout_oracle_test.go replays every case through our Go engine.
//
//   cd tools/yoga-oracle && npm install && node gen.mjs
//
// Yoga is configured like upstream (packages/native/src/yoga.zig,
// context.zig): web defaults off; positions rounded from absolute edges.
// npm Yoga has no switch for that rounding rule, so we turn its rounding
// off (point scale 0) and round absolute edges here instead.
import Yoga, { Align, Direction, Display, Edge, FlexDirection, Gutter, Justify, MeasureMode, PositionType, Wrap } from "yoga-layout"
import { writeFileSync } from "node:fs"

const config = Yoga.Config.create()
config.setUseWebDefaults(false)
config.setPointScaleFactor(0)

const DIRS = { column: FlexDirection.Column, row: FlexDirection.Row, "column-reverse": FlexDirection.ColumnReverse, "row-reverse": FlexDirection.RowReverse }
const JUST = { start: Justify.FlexStart, center: Justify.Center, end: Justify.FlexEnd, between: Justify.SpaceBetween, around: Justify.SpaceAround, evenly: Justify.SpaceEvenly }
const ALIGN = { auto: Align.Auto, start: Align.FlexStart, center: Align.Center, end: Align.FlexEnd, stretch: Align.Stretch }
const EDGES = [Edge.Top, Edge.Right, Edge.Bottom, Edge.Left]

function build(spec) {
  const n = Yoga.Node.create(config)
  const s = spec.s ?? {}
  n.setFlexDirection(DIRS[s.dir ?? "column"])
  if (s.wrap) n.setFlexWrap(s.wrap === "wrap" ? Wrap.Wrap : Wrap.NoWrap)
  n.setJustifyContent(JUST[s.justify ?? "start"])
  n.setAlignItems(ALIGN[s.alignItems ?? "stretch"])
  n.setAlignSelf(ALIGN[s.alignSelf ?? "auto"])
  for (const [k, fn] of [["w", "setWidth"], ["h", "setHeight"], ["minW", "setMinWidth"], ["minH", "setMinHeight"], ["maxW", "setMaxWidth"], ["maxH", "setMaxHeight"], ["basis", "setFlexBasis"]]) {
    if (s[k] !== undefined) n[fn](s[k])
  }
  n.setFlexGrow(s.grow ?? 0)
  // Upstream rule (Renderable.ts): shrink 1, or 0 if width/height is a number.
  n.setFlexShrink(s.shrink ?? (typeof s.w === "number" || typeof s.h === "number" ? 0 : 1))
  if (s.gap) n.setGap(Gutter.All, s.gap)
  ;(s.pad ?? [0, 0, 0, 0]).forEach((v, i) => n.setPadding(EDGES[i], v))
  ;(s.margin ?? [0, 0, 0, 0]).forEach((v, i) => n.setMargin(EDGES[i], v))
  ;(spec.border ?? [0, 0, 0, 0]).forEach((v, i) => n.setBorder(EDGES[i], v))
  if (s.pos === "abs") n.setPositionType(PositionType.Absolute)
  for (const [k, e] of [["top", Edge.Top], ["right", Edge.Right], ["bottom", Edge.Bottom], ["left", Edge.Left]]) {
    if (s[k] !== undefined) n.setPosition(e, s[k])
  }
  if (spec.hidden) n.setDisplay(Display.None)
  if (spec.text) {
    const [tw, th] = spec.text
    n.setMeasureFunc((w, wm, h, hm) => ({
      width: wm === MeasureMode.Exactly ? w : wm === MeasureMode.AtMost ? Math.min(tw, w) : tw,
      height: hm === MeasureMode.Exactly ? h : hm === MeasureMode.AtMost ? Math.min(th, h) : th,
    }))
  }
  ;(spec.c ?? []).forEach((c, i) => n.insertChild(build(c), i))
  return n
}

// Yoga's pixel-grid rounding: near-half values round up. OpenTUI's patch
// feeds it absolute edges rather than relative positions.
const inexact = (a, b) => Math.abs(a - b) < 0.0001
function grid(v) {
  let f = v % 1
  if (f < 0) f++
  if (inexact(f, 0)) return v - f
  if (inexact(f, 1)) return v - f + 1
  return v - f + (f > 0.5 || inexact(f, 0.5) ? 1 : 0)
}

// Depth-first rects, rounded from absolute edges like upstream.
function collect(spec, node, px, py, out) {
  if (spec.hidden) return
  const l = node.getComputedLayout()
  const x = px + l.left, y = py + l.top
  const rx = grid(x), ry = grid(y)
  out.push([rx, ry, grid(x + l.width) - rx, grid(y + l.height) - ry])
  ;(spec.c ?? []).forEach((c, i) => collect(c, node.getChild(i), x, y, out))
}

function solve(tc) {
  const root = build(tc.root)
  root.setWidth(tc.width)
  root.setHeight(tc.height)
  root.calculateLayout(tc.width, tc.height, Direction.LTR)
  const out = []
  collect(tc.root, root, 0, 0, out)
  root.freeRecursive()
  return out
}

// ---- cases ------------------------------------------------------------------

const hand = [
  { name: "column stretch", width: 20, height: 10, root: { c: [{ s: { h: 3 } }, { s: { h: 2 } }] } },
  { name: "sidebar + grow", width: 50, height: 8, root: { s: { dir: "row" }, c: [{ s: { w: 10 } }, { s: { grow: 1 } }] } },
  { name: "grow 1:2 rounding", width: 10, height: 1, root: { s: { dir: "row" }, c: [{ s: { grow: 1 } }, { s: { grow: 2 } }] } },
  { name: "nested fractions", width: 11, height: 3, root: { s: { dir: "row" }, c: [1, 1, 1].map(() => ({ s: { grow: 1, dir: "row" }, c: [{ s: { grow: 1 } }, { s: { grow: 1 } }] })) } },
  { name: "box border padding gap", width: 20, height: 5, root: { c: [{ border: [1, 1, 1, 1], s: { dir: "row", pad: [0, 1, 0, 1], gap: 2, h: 5 }, c: [{ s: { grow: 1 } }, { s: { grow: 1 } }] }] } },
  { name: "max freezes", width: 30, height: 1, root: { s: { dir: "row" }, c: [{ s: { grow: 1, maxW: 5 } }, { s: { grow: 1 } }] } },
  { name: "shrink by basis", width: 24, height: 1, root: { s: { dir: "row" }, c: [{ s: { basis: 20 } }, { s: { basis: 10 } }] } },
  { name: "text auto size", width: 40, height: 20, root: { c: [{ border: [1, 1, 1, 1], s: { alignSelf: "start" }, c: [{ text: [6, 2] }] }] } },
  { name: "absolute insets", width: 20, height: 10, root: { c: [{ border: [1, 1, 1, 1], s: { grow: 1 }, c: [
    { s: { h: 2 } },
    { s: { pos: "abs", left: 1, top: 1, w: 4, h: 2 } },
    { s: { pos: "abs", right: 0, bottom: 0, w: 3, h: 1 } },
    { s: { pos: "abs", left: 2, right: 2, top: 0, h: 1 } },
  ] }] } },
  { name: "two panel demo", width: 80, height: 24, root: { s: { dir: "column" }, c: [
    { s: { h: 3 }, border: [1, 1, 1, 1], c: [{ text: [12, 1] }] },
    { s: { grow: 1, dir: "row", gap: 1 }, c: [
      { s: { w: "30%", minW: 20 }, border: [1, 1, 1, 1], c: [{ text: [8, 1] }, { text: [10, 1] }] },
      { s: { grow: 1, pad: [0, 1, 0, 1] }, border: [1, 1, 1, 1], c: [{ text: [30, 3] }] },
    ] },
    { s: { h: 1 } },
  ] } },
  { name: "wrap row basic", width: 10, height: 5, root: { s: { dir: "row", wrap: "wrap" }, c: [4, 4, 4].map(w => ({ s: { w, h: 1 } })) } },
  { name: "wrap row gap", width: 10, height: 8, root: { s: { dir: "row", wrap: "wrap", gap: 1 }, c: [4, 4, 4].map(w => ({ s: { w, h: 2 } })) } },
  { name: "wrap column", width: 8, height: 5, root: { s: { wrap: "wrap" }, c: [2, 2, 2].map(h => ({ s: { w: 2, h } })) } },
  { name: "wrap align center", width: 10, height: 8, root: { s: { dir: "row", wrap: "wrap", alignItems: "center" }, c: [{ s: { w: 6, h: 3 } }, { s: { w: 4, h: 1 } }, { s: { w: 6, h: 2 } }] } },
  { name: "wrap stretch per line", width: 10, height: 8, root: { s: { dir: "row", wrap: "wrap" }, c: [{ s: { w: 6, h: 3 } }, { s: { w: 4 } }, { s: { w: 6, h: 2 } }, { s: { w: 4 } }] } },
  { name: "wrap grow per line", width: 10, height: 8, root: { s: { dir: "row", wrap: "wrap" }, c: [{ s: { w: 6 } }, { s: { grow: 1, basis: 3 } }, { s: { w: 6 } }, { s: { grow: 1, basis: 3 } }] } },
  { name: "wrap measured children", width: 8, height: 6, root: { s: { dir: "row", wrap: "wrap", alignItems: "start" }, c: [{ text: [5, 2] }, { text: [5, 1] }, { text: [2, 2] }] } },
  { name: "wrap reverse row", width: 10, height: 6, root: { s: { dir: "row-reverse", wrap: "wrap", gap: 1 }, c: [4, 4, 4].map(w => ({ s: { w, h: 2 } })) } },
  { name: "wrap absolute child", width: 10, height: 6, root: { s: { dir: "row", wrap: "wrap" }, c: [{ s: { w: 7, h: 2 } }, { s: { pos: "abs", top: 1, left: 1, w: 2, h: 2 } }, { s: { w: 7, h: 2 } }] } },
  { name: "wrap row margins", width: 11, height: 9, root: { s: { dir: "row", wrap: "wrap" }, c: [{ s: { w: 4, h: 3, margin: [0, 1, 1, 0] } }, { s: { w: 7, h: 3, margin: [1, 1, 0, 0] } }, { s: { w: 6, h: 1 } }] } },
  { name: "wrap nested and padded", width: 16, height: 10, root: { s: { dir: "row", wrap: "wrap", gap: 1, pad: [1, 1, 1, 1] }, c: [{ s: { w: 8, dir: "row", h: 3, pad: [0, 1, 0, 1] }, c: [{ text: [3, 1] }, { s: { grow: 1 } }] }, { s: { w: 7, h: 2 } }, { s: { w: 8, h: 3 }, c: [{ s: { grow: 1 } }] }] } },
]

// Seeded random trees inside the supported subset.
// mulberry32: tiny seeded PRNG with a long period.
let seed = 12345
const rnd = () => {
  seed = (seed + 0x6d2b79f5) | 0
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed)
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
  return ((t ^ (t >>> 14)) >>> 0) / 4294967296
}
const pick = (a) => a[Math.floor(rnd() * a.length)]
const int = (lo, hi) => lo + Math.floor(rnd() * (hi - lo + 1))
const maybe = (p) => rnd() < p

function randDim(max) {
  if (maybe(0.25)) return `${pick([10, 25, 33, 50, 75, 100])}%`
  return int(0, max)
}

function randNode(depth) {
  const s = {}
  if (maybe(0.5)) s.dir = pick(Object.keys(DIRS))
  if (maybe(0.4)) s.justify = pick(Object.keys(JUST))
  if (maybe(0.3)) s.alignItems = pick(["start", "center", "end", "stretch"])
  if (maybe(0.3)) s.alignSelf = pick(Object.keys(ALIGN))
  if (maybe(0.35)) s.w = randDim(30)
  if (maybe(0.35)) s.h = randDim(12)
  if (maybe(0.15)) s.minW = int(0, 15)
  if (maybe(0.15)) s.maxW = int(5, 30)
  if (maybe(0.1)) s.minH = int(0, 6)
  if (maybe(0.1)) s.maxH = int(2, 10)
  if (maybe(0.4)) s.grow = pick([1, 1, 2, 3])
  if (maybe(0.2)) s.shrink = pick([0, 1, 2])
  if (maybe(0.15)) s.basis = int(0, 20)
  if (maybe(0.3)) s.gap = int(1, 3)
  if (maybe(0.3)) s.pad = [int(0, 2), int(0, 2), int(0, 2), int(0, 2)]
  if (maybe(0.2)) s.margin = [int(0, 2), int(0, 2), int(0, 2), int(0, 2)]
  const spec = { s }
  if (maybe(0.3)) spec.border = [1, 1, 1, 1].map(() => (maybe(0.8) ? 1 : 0))
  if (maybe(0.08)) spec.hidden = true
  if (depth > 0 && maybe(0.75)) {
    spec.c = Array.from({ length: int(1, 4) }, () => randNode(depth - 1))
    if (maybe(0.2)) {
      const a = { s: { pos: "abs", w: int(1, 10), h: int(1, 4) } }
      if (maybe(0.5)) a.s.left = int(0, 5); else a.s.right = int(0, 5)
      if (maybe(0.5)) a.s.top = int(0, 3); else a.s.bottom = int(0, 3)
      spec.c.push(a)
    }
  } else {
    spec.text = [int(0, 20), int(1, 3)]
  }
  return spec
}

// The root is always exactly terminal-sized upstream, so keep only the
// properties that shape its children.
function randRoot() {
  const r = randNode(3)
  for (const k of ["w", "h", "minW", "maxW", "minH", "maxH", "margin", "grow", "shrink", "basis", "alignSelf"]) delete r.s[k]
  delete r.hidden
  if (!r.c) { delete r.text; r.c = [randNode(2)] }
  return r
}

const random = Array.from({ length: 400 }, (_, i) => ({
  name: `random #${i}`, width: int(10, 90), height: int(4, 30), root: randRoot(),
}))
const wrapped = Array.from({ length: 40 }, (_, i) => {
  const width = int(8, 30), height = int(4, 14)
  const dir = pick(["row", "column", "row-reverse", "column-reverse"])
  const root = {
    s: { dir, wrap: "wrap", gap: int(0, 2), alignItems: pick(["start", "center", "end", "stretch"]) },
    c: Array.from({ length: int(2, 6) }, () => ({ s: {
      w: int(2, 9), h: int(1, 4),
      margin: [int(0, 1), int(0, 1), int(0, 1), int(0, 1)],
    } })),
  }
  return { name: `wrapped #${i}`, width, height, root }
})

const cases = [...hand, ...random, ...wrapped].map((tc) => ({
  ...tc,
  // npm Yoga 3.2.1 overflows in this generated case. OpenTUI carries
  // yoga-flex-shrink-residue.patch (facebook/yoga#1974 and #2044), so the
  // Go port must reject npm's gigantic geometry rather than match it.
  ...(tc.name === "random #258" ? { openTUIPatch: "flex-shrink-residue" } : {}),
  want: solve(tc),
}))
writeFileSync(new URL("../../testdata/yoga_cases.json", import.meta.url), JSON.stringify({ yoga: "3.2.1", cases }, null, 0) + "\n")
console.log(`wrote ${cases.length} cases`)

// Debug one case's raw, unrounded Yoga layout:
if (process.env.ORACLE_CASE) {
  const tc = cases.find(c => c.name === process.env.ORACLE_CASE)
  const root = build(tc.root)
  root.setWidth(tc.width); root.setHeight(tc.height)
  root.calculateLayout(tc.width, tc.height, Direction.LTR)
  let i = 0
  const walk = (spec, n, px=0, py=0, d=0) => {
    if (spec.hidden) return
    const l=n.getComputedLayout(), x=px+l.left, y=py+l.top
    console.log(`#${i++} d${d} rel=(${l.left},${l.top}) abs=(${x},${y}) size=(${l.width},${l.height})`)
    ;(spec.c??[]).forEach((c,j)=>walk(c,n.getChild(j),x,y,d+1))
  }
  walk(tc.root, root)
  root.freeRecursive()
}
