// wine renders an animated 3D scene in the terminal: two glasses with
// glowing rims, wine poured into them from above, sloshing as they fill,
// and red light cast onto the table. Then the glasses rise and clink, a
// few drops slip over the rims, and they leave a little puddle on the
// table. It is ray marched on the CPU in plain Go, then drawn with half
// blocks: each cell shows "▀" with the upper pixel
// as the foreground color and the lower pixel as the background, so a
// terminal of W×H cells is a W×2H pixel canvas.
//
// Space pauses, R restarts the pour, Left/Right orbit the camera, Q or
// Ctrl+C quits.
//
//	go run ./examples/wine
package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	tui "github.com/tolaniverse/opentui-go"
)

// ---- Vectors ---------------------------------------------------------------

type vec struct{ x, y, z float64 }

func v(x, y, z float64) vec           { return vec{x, y, z} }
func (a vec) add(b vec) vec           { return vec{a.x + b.x, a.y + b.y, a.z + b.z} }
func (a vec) sub(b vec) vec           { return vec{a.x - b.x, a.y - b.y, a.z - b.z} }
func (a vec) mul(s float64) vec       { return vec{a.x * s, a.y * s, a.z * s} }
func (a vec) dot(b vec) float64       { return a.x*b.x + a.y*b.y + a.z*b.z }
func (a vec) length() float64         { return math.Sqrt(a.dot(a)) }
func (a vec) norm() vec               { return a.mul(1 / a.length()) }
func (a vec) cross(b vec) vec         { return vec{a.y*b.z - a.z*b.y, a.z*b.x - a.x*b.z, a.x*b.y - a.y*b.x} }
func reflect(d, n vec) vec            { return d.sub(n.mul(2 * d.dot(n))) }
func clamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }

func smoothstep(a, b, x float64) float64 {
	t := clamp((x-a)/(b-a), 0, 1)
	return t * t * (3 - 2*t)
}

// smin is a polynomial smooth minimum: it blends two shapes with a fillet
// of size k, so the stem flows into the bowl and the foot.
func smin(a, b, k float64) float64 {
	h := clamp(0.5+0.5*(b-a)/k, 0, 1)
	return b + (a-b)*h - k*h*(1-h)
}

// ---- Scene -----------------------------------------------------------------

const (
	bowlY     = 1.55 // bowl center height when the glass stands on the table
	bowlR     = 0.62 // bowl radius
	rimY      = bowlY + 0.40
	shell     = 0.012 // glass thickness
	stemR     = 0.03
	footR     = 0.42
	footH     = 0.03
	streamR   = 0.022
	pourTime  = 5.0  // seconds a pour lasts
	loopTime  = 20.0 // seconds before the scene restarts
	skyY      = 6.0  // where the stream comes from, above the frame
	gravity   = 12.0
	clinkTime = 11.0
	restX     = 1.25
	// The bowls are spheres, so they touch when their centers are two
	// radii apart: each center is contactX from the middle.
	contactX = bowlR + shell
)

var (
	light = v(-0.55, 0.75, -0.35).norm()
	wine  = v(0.40, 0.012, 0.028)
)

// ---- Choreography ----------------------------------------------------------

// key is one pose of the cheers, mirrored for the two glasses: how far the
// glass is from the middle, how high it is lifted, and how far its top
// leans in. ease shapes the move that ends at this key.
type key struct {
	t, x, lift, tilt float64
	ease             func(float64) float64
}

func easeIn(x float64) float64     { return x * x * x } // speeds up into the clink
func easeOut(x float64) float64    { return 1 - (1-x)*(1-x)*(1-x) }
func easeSmooth(x float64) float64 { return x * x * (3 - 2*x) }

var choreography = []key{
	{0, restX, 0, 0, easeSmooth},
	{9.0, restX, 0, 0, easeSmooth},                // both full, settled
	{10.3, 0.95, 0.50, 0.06, easeSmooth},          // lift
	{clinkTime, contactX, 0.56, 0.30, easeIn},     // swing in: clink
	{clinkTime + 0.35, 0.76, 0.58, 0.20, easeOut}, // rebound
	{12.6, 0.82, 0.62, 0.14, easeSmooth},          // cheers, held up
	{14.4, restX, 0, 0, easeSmooth},               // back down
}

func pose(t float64) (x, lift, tilt float64) {
	k := choreography
	for i := 1; i < len(k); i++ {
		if t < k[i].t {
			a, b := k[i-1], k[i]
			f := b.ease(clamp((t-a.t)/(b.t-a.t), 0, 1))
			lerp := func(p, q float64) float64 { return p + (q-p)*f }
			return lerp(a.x, b.x), lerp(a.lift, b.lift), lerp(a.tilt, b.tilt)
		}
	}
	last := k[len(k)-1]
	return last.x, last.lift, last.tilt
}

// ---- Glasses ---------------------------------------------------------------

// glass is the per-glass state for one frame.
type glass struct {
	side     float64 // -1 left, +1 right
	cx, cy   float64 // bowl center
	lift     float64
	cos, sin float64 // of the lean angle
	// level is the wine surface height above the bowl center. Tilting a
	// sphere about its center does not change how full it looks, so the
	// surface stays level while the glass leans.
	level        float64
	slosh        float64 // surface tilt amplitude
	jolt         float64 // inward wave from the clink
	phase        float64
	stream       bool
	streamBottom float64
	streamTop    float64
	impact       float64 // ripple strength where the stream lands
}

// local maps a world point into the glass's own frame, where it stands
// upright on the origin: translate to the bowl center, undo the lean.
func (g *glass) local(p vec) vec {
	x, y := p.x-g.cx, p.y-g.cy
	return v(g.cos*x+g.sin*y, -g.sin*x+g.cos*y+bowlY, p.z)
}

// world maps a point of the upright glass into the world.
func (g *glass) world(q vec) vec {
	x, y := q.x, q.y-bowlY
	return v(g.cos*x-g.sin*y+g.cx, g.sin*x+g.cos*y+g.cy, q.z)
}

func placeGlass(i int, t float64) glass {
	side := float64(2*i - 1)
	x, lift, tilt := pose(t)
	// The top leans toward the middle: clockwise on the left.
	angle := side * tilt
	return glass{side: side, cx: side * x, cy: bowlY + lift, lift: lift, cos: math.Cos(angle), sin: math.Sin(angle)}
}

// ---- Drops -----------------------------------------------------------------

// drop is wine knocked over a rim by the clink.
type drop struct {
	t0     float64
	p0, v0 vec
	r      float64
	land   float64 // seconds after t0 when it reaches the floor
}

func (d *drop) at(t float64) vec {
	dt := t - d.t0
	return d.p0.add(d.v0.mul(dt)).add(v(0, -0.5*gravity*dt*dt, 0))
}

// drops are fixed, so every loop spills the same way.
var drops = func() []drop {
	var out []drop
	for i := range 7 {
		g := placeGlass(i%2, clinkTime)
		// A point on the inner rim, spread around it a little.
		a := -0.35 + 0.12*float64(i)
		rim := math.Sqrt(bowlR*bowlR - 0.40*0.40)
		in := -g.side // toward the middle
		p0 := g.world(v(in*rim*math.Cos(a), rimY, -rim*math.Sin(a)*0.8))
		j := float64((i*37)%11) / 10 // 0..1, deterministic
		d := drop{
			t0: clinkTime + 0.02*float64(i%3),
			p0: p0,
			v0: v(in*(0.25+0.35*j), 0.4+0.9*(1-j), -0.35-0.5*j),
			r:  0.024 + 0.016*j,
		}
		d.land = (d.v0.y + math.Sqrt(d.v0.y*d.v0.y+2*gravity*(d.p0.y-d.r*0.5))) / gravity
		out = append(out, d)
	}
	return out
}()

type falling struct {
	head, tail vec
	r          float64
}

// ---- Scene -----------------------------------------------------------------

type scene struct {
	t        float64
	glass    [2]glass
	falling  []falling
	flash    float64 // brightness of the clink glint
	flashPos vec
	camera   float64 // orbit angle

	right, up vec // camera axes, for the screen-space glint
}

func newScene(t, camera float64) *scene {
	s := &scene{t: t, camera: camera}
	low, high := -bowlR+0.10, 0.06
	for i := range s.glass {
		g := placeGlass(i, t)
		start := 1.0 + 1.6*float64(i)
		end := start + pourTime
		g.level = low + (high-low)*smoothstep(start+0.35, end, t)
		// The stream's head falls from the sky when the pour starts and its
		// tail falls after it ends, both under gravity. Pours happen while
		// the glasses stand still.
		if t > start {
			surface := g.cy + g.level
			g.streamBottom = math.Max(surface, skyY-0.5*14*(t-start)*(t-start))
			g.streamTop = skyY
			if t > end {
				g.streamTop = skyY - 0.5*14*(t-end)*(t-end)
			}
			g.stream = g.streamTop > g.streamBottom+0.02
		}
		pouring := smoothstep(start, start+0.4, t) * (1 - smoothstep(end, end+1.2, t))
		g.impact = pouring
		g.slosh = 0.03*pouring + 0.02*math.Exp(-1.2*math.Max(t-end, 0))*smoothstep(start, start+1, t)
		// Lifting and setting down rock the wine a little.
		_, _, tilt := pose(t)
		g.slosh += 0.04 * tilt
		if t > clinkTime {
			g.jolt = 0.16 * math.Exp(-2.4*(t-clinkTime))
		}
		g.phase = float64(i) * 1.7
		s.glass[i] = g
	}
	for i := range drops {
		d := &drops[i]
		if dt := t - d.t0; dt > 0 && dt < d.land {
			head := d.at(t)
			// Stretch the drop along its path: a short streak, like the eye
			// sees a fast drop.
			vel := d.v0.add(v(0, -gravity*dt, 0))
			s.falling = append(s.falling, falling{head, head.sub(vel.mul(0.018)), d.r})
		}
	}
	if dt := t - clinkTime; dt > 0 {
		s.flash = math.Exp(-dt * 9)
		s.flashPos = v(0, s.glass[1].cy, 0)
	}
	return s
}

// surface is the height of the wine at world (x, z), with a slosh tilt, the
// clink's wave toward the middle, and ripples where the stream lands.
func (g *glass) surface(x, z, t float64) float64 {
	dx, dz := x-g.cx, z
	h := g.cy + g.level + g.slosh*(math.Sin(2.6*t+g.phase)*dx+0.6*math.Cos(2.1*t+g.phase)*dz)
	if g.jolt > 0 {
		h += g.jolt * math.Cos(9*(t-clinkTime)) * (-g.side * dx)
	}
	if g.impact > 0 {
		r := math.Hypot(dx-0.05, dz)
		h += 0.012 * g.impact * math.Sin(26*r-15*t) * math.Exp(-3.5*r)
	}
	return h
}

func glassSDF(q vec) float64 {
	b := q.sub(v(0, bowlY, 0))
	bowl := math.Max(math.Abs(b.length()-bowlR)-shell, q.y-rimY)
	r := math.Hypot(q.x, q.z)
	stemTop := bowlY - bowlR + 0.03
	stem := math.Max(r-stemR, math.Abs(q.y-stemTop/2)-stemTop/2)
	foot := math.Max(r-footR+0.012, math.Abs(q.y-footH/2)-footH/2) - 0.012
	return smin(smin(bowl, stem, 0.06), foot, 0.05)
}

// nearest is the glass on p's side; the glasses never cross the middle.
func (s *scene) nearest(p vec) *glass {
	if p.x < 0 {
		return &s.glass[0]
	}
	return &s.glass[1]
}

func (s *scene) glassDist(p vec) float64 {
	g := s.nearest(p)
	return glassSDF(g.local(p))
}

func segment(p, a, b vec) float64 {
	pa, ba := p.sub(a), b.sub(a)
	h := clamp(pa.dot(ba)/math.Max(ba.dot(ba), 1e-9), 0, 1)
	return pa.sub(ba.mul(h)).length()
}

func (s *scene) wineDist(p vec) float64 {
	g := s.nearest(p)
	d := math.Inf(1)
	if g.level > -bowlR+0.01 {
		inner := v(p.x-g.cx, p.y-g.cy, p.z).length() - (bowlR - shell - 0.004)
		// The surface is a height field, not an exact distance; the 0.8
		// keeps the march from stepping through the ripples.
		d = math.Max(inner, 0.8*(p.y-g.surface(p.x, p.z, s.t)))
	}
	if g.stream {
		sx := g.cx + 0.05 + 0.008*math.Sin(p.y*7+s.t*11)
		y := clamp(p.y, g.streamBottom, g.streamTop)
		stream := v(p.x-sx, p.y-y, p.z).length() - streamR
		d = smin(d, stream, 0.05)
	}
	// Drops occupy a tiny region; avoid seven segment tests on every
	// ray-march step elsewhere in the room.
	if p.y >= 0 && p.y < 3 && math.Abs(p.x) < 2 && math.Abs(p.z) < 2 {
		for _, f := range s.falling {
			d = math.Min(d, segment(p, f.head, f.tail)-f.r)
		}
	}
	return d
}

const (
	hitNone = iota
	hitGlass
	hitWine
	hitFloor
)

func (s *scene) dist(p vec) (float64, int) {
	d, what := p.y, hitFloor
	if g := s.glassDist(p); g < d {
		d, what = g, hitGlass
	}
	if w := s.wineDist(p); w < d {
		d, what = w, hitWine
	}
	return d, what
}

func normal(f func(vec) float64, p vec) vec {
	const e = 0.0015
	return v(
		f(p.add(v(e, 0, 0)))-f(p.sub(v(e, 0, 0))),
		f(p.add(v(0, e, 0)))-f(p.sub(v(0, e, 0))),
		f(p.add(v(0, 0, e)))-f(p.sub(v(0, 0, e))),
	).norm()
}

// trace adds the clink glint to what march sees.
func (s *scene) trace(ro, rd vec) vec {
	col := s.march(ro, rd)
	if s.flash > 0.01 {
		// The clink: a glint where the bowls touch, a tight core in a soft halo.
		// The offset of the ray from the contact point, measured along the
		// screen axes, draws a four-pointed star.
		w := s.flashPos.sub(ro)
		if along := w.dot(rd); along > 0 {
			o := w.sub(rd.mul(along)).mul(5 / along) // roughly screen units
			x, y := math.Abs(o.dot(s.right)), math.Abs(o.dot(s.up))
			core := math.Exp(-(x*x + y*y) * 220)
			halo := math.Exp(-(x*x + y*y) * 9)
			star := math.Exp(-x*14-y*y*2500) + math.Exp(-y*14-x*x*2500)
			col = col.add(v(1, 0.93, 0.88).mul(s.flash * (0.8*core + 0.10*halo + 0.35*star)))
		}
	}
	return col
}

// march marches one ray. Glass is see-through: each time the ray touches
// it, a Fresnel highlight is added and the ray carries on, dimmed. Glow is
// accumulated near glass surfaces, a cheap bloom around the rims.
func (s *scene) march(ro, rd vec) vec {
	col, trans, glow := vec{}, 1.0, 0.0
	t, glassHits := 0.02, 0
	for i := 0; i < 140 && t < 18; i++ {
		p := ro.add(rd.mul(t))
		d, what := s.dist(p)
		if gd := s.glassDist(p); gd > 0 {
			glow += trans * math.Exp(-gd*95) * math.Min(math.Max(d, 0.004), 0.035)
		}
		if d < 0.0015 {
			switch what {
			case hitGlass:
				n := normal(s.glassDist, p)
				fres := math.Pow(1-math.Abs(n.dot(rd)), 5)
				spec := math.Pow(math.Max(reflect(rd, n).dot(light), 0), 120)
				// The bowl catches narrow side lights; the stem and foot are
				// nearly black except for tiny specular highlights.
				material := 1.0
				if s.nearest(p).local(p).y < bowlY-bowlR+0.1 {
					material = 0.35
				}
				col = col.add(v(0.95, 0.91, 0.86).mul(trans * material * (fres*environment(reflect(rd, n)) + 0.6*spec)))
				trans *= 0.97 - 0.42*fres
				glassHits++
				if glassHits > 6 {
					return col.add(v(1, 0.85, 0.78).mul(glow * 0.65))
				}
				t += 0.03 // through the shell
				continue
			case hitWine:
				col = col.add(s.shadeWine(p, rd).mul(trans))
			case hitFloor:
				col = col.add(s.shadeFloor(p, rd).mul(trans))
			}
			return col.add(v(1, 0.85, 0.78).mul(glow * 0.65))
		}
		t += math.Max(d*0.85, 0.002)
	}
	return col.add(background(rd).mul(trans)).add(v(1, 0.85, 0.78).mul(glow * 0.65))
}

func (s *scene) shadeWine(p, rd vec) vec {
	n := normal(s.wineDist, p)
	base := wine
	if n.y > 0.75 && p.y > s.nearest(p).cy-bowlR+0.1 {
		// The center of the wine's top is dark; only its far edge and
		// narrow reflections should catch light.
		reflected := math.Pow(math.Max(reflect(rd, n).dot(light), 0), 70)
		return v(0.045, 0.001, 0.003).add(v(0.6, 0.1, 0.08).mul(reflected * 0.3))
	}
	diff := 0.22 + 0.78*math.Max(n.dot(light), 0)
	fres := math.Pow(1-math.Abs(n.dot(rd)), 3)
	spec := math.Pow(math.Max(reflect(rd, n).dot(light), 0), 70)
	return base.mul(diff).add(v(0.9, 0.035, 0.045).mul(0.85 * fres)).add(v(1, 0.65, 0.6).mul(0.5 * spec))
}

func (s *scene) shadeFloor(p, rd vec) vec {
	c := v(0.035, 0.030, 0.028).mul(math.Exp(-0.04 * (p.x*p.x + p.z*p.z)))
	for i := range s.glass {
		g := &s.glass[i]
		dx, dz := p.x-g.cx, p.z
		// Contact shadow under the foot: softer and fainter as it lifts.
		soft := 1 + 4*g.lift
		c = c.mul(1 - 0.75/soft*math.Exp(-(dx*dx+dz*dz)*9/soft))
		if g.lift < 0.15 {
			// The near-black foot is a flattened disk, not a white line.
			weight := 1 - g.lift/0.15
			radial := math.Hypot(dx, dz) / footR
			disc := 1 - smoothstep(0.82, 1.05, radial)
			c = c.mul(1 - 0.9*disc*weight)
			c = c.add(v(0.15, 0.13, 0.12).mul(0.22 * weight * math.Exp(-dx*dx*80-(dz+0.08)*(dz+0.08)*100)))
		}
		// Light through the wine lands to the right of the glass, further
		// away the higher the glass is.
		fill := smoothstep(-bowlR, 0.06, g.level)
		lx, lz := dx-0.8-0.73*g.lift, dz-0.25-0.45*g.lift
		c = c.add(v(0.42, 0.02, 0.03).mul(fill / (1 + 1.5*g.lift) * math.Exp(-(lx*lx*2.2+lz*lz*9)/(1+2*g.lift))))
		// A faint, broken reflection on the tabletop below each foot.
		// Its visible lobe sits in front of the glass from this camera.
		rx, rz := dx, dz+0.85
		reflection := math.Exp(-rx*rx*7 - rz*rz*12)
		c = c.add(v(0.025, 0.019, 0.018).mul(reflection / (1 + 2*g.lift)))
	}
	// A dim, blurry reflection of the rims.
	spec := math.Pow(math.Max(reflect(rd, v(0, 1, 0)).dot(light), 0), 8)
	c = c.add(v(0.03, 0.022, 0.022).mul(spec))
	c = s.puddle(p, rd, c)
	// A subdued but visible horizon separates the dark room from the table.
	fog := smoothstep(4, 5.5, math.Hypot(p.x, p.z+2))
	return c.mul(1 - fog).add(background(rd).mul(fog))
}

// puddle draws spilled wine on the floor. Each landed drop spreads into a
// blob; the blobs add up like metaballs, so neighbors merge into one
// puddle with a soft, organic edge.
func (s *scene) puddle(p, rd, floor vec) vec {
	field := 0.0
	for i := range drops {
		d := &drops[i]
		since := s.t - d.t0 - d.land
		if since < 0 {
			continue
		}
		at := d.at(d.t0 + d.land)
		radius := (0.055 + 1.9*d.r) * (0.35 + 0.65*smoothstep(0, 0.35, since))
		dx, dz := p.x-at.x, p.z-at.z
		field += math.Exp(-(dx*dx + dz*dz) / (radius * radius))
	}
	if field < 0.2 {
		return floor
	}
	inside := smoothstep(0.42, 0.55, field)
	edge := smoothstep(0.3, 0.45, field) * (1 - inside)
	// Wine on the table is dark and glossy: it mostly reflects.
	r := reflect(rd, v(0, 1, 0))
	spec := math.Pow(math.Max(r.dot(light), 0), 30)
	gloss := 0.15*environment(r) + 0.8*spec
	pool := v(0.055, 0.002, 0.004).add(v(0.9, 0.5, 0.5).mul(gloss * 0.15))
	c := floor.mul(1 - inside).add(pool.mul(inside))
	return c.add(v(0.35, 0.05, 0.06).mul(edge * 0.4))
}

// environment is how bright the dark room looks in a reflected direction:
// two tall soft lights stand off to the sides, so glass edges that face
// sideways catch them and the rest stays dark.
func environment(r vec) float64 {
	return 0.04 + 11*smoothstep(0.45, 0.92, math.Abs(r.x))*smoothstep(-0.3, 0.2, r.y)
}

func background(rd vec) vec {
	h := 0.5 + 0.5*rd.y
	spot := math.Exp(-rd.x * rd.x * 2.5)
	return v(0.013, 0.005, 0.005).mul(0.3 + 1.0*h*spot)
}

// ---- Pixels ----------------------------------------------------------------

// toRGB tone-maps a linear color and applies gamma.
func toRGB(c vec) tui.RGBA {
	m := func(x float64) uint8 {
		x = 1 - math.Exp(-1.25*math.Max(x, 0))
		return uint8(255 * clamp(math.Pow(x, 1/2.2), 0, 1))
	}
	return tui.RGB(m(c.x), m(c.y), m(c.z))
}

// render fills pixels (w × h, row-major) for scene s, splitting rows across
// CPUs.
func render(s *scene, pixels []tui.RGBA, w, h int, fade float64) {
	ro := v(5.0*math.Sin(s.camera), 1.65, -5.0*math.Cos(s.camera))
	target := v(0, 1.05, 0)
	fw := target.sub(ro).norm()
	right := v(0, 1, 0).cross(fw).norm()
	up := fw.cross(right)
	s.right, s.up = right, up
	const focal = 2.2

	var next atomic.Int64
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				y := int(next.Add(1)) - 1
				if y >= h {
					return
				}
				for x := range w {
					// Half-block pixels are about square, so one scale fits both axes.
					u := (2*float64(x) + 1 - float64(w)) / float64(h)
					vv := -(2*float64(y) + 1 - float64(h)) / float64(h)
					rd := fw.mul(focal).add(right.mul(u)).add(up.mul(vv)).norm()
					c := s.trace(ro, rd)
					// Vignette.
					r2 := (u*u*0.35 + vv*vv) * 0.45
					pixels[y*w+x] = toRGB(c.mul(fade * (1 - math.Min(r2, 0.85))))
				}
			}
		}()
	}
	wg.Wait()
}

// ---- App -------------------------------------------------------------------

func main() {
	r, err := tui.New(tui.Options{Background: tui.RGB(0, 0, 0), Title: "opentui-go · wine", NoMouse: true})
	if err != nil {
		fmt.Fprintln(os.Stderr, "wine:", err)
		os.Exit(1)
	}

	var (
		mu        sync.Mutex
		clock     float64 // scene time, seconds
		paused    bool
		camera    float64
		last      = time.Now()
		frameTime time.Duration
		pixels    []tui.RGBA
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.SetKeyHandler(func(k *tui.KeyEvent) {
		if k.Type == tui.KeyRelease {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch k.Name {
		case "space":
			paused = !paused
		case "r":
			clock = 0
		case "left":
			camera -= 0.08
		case "right":
			camera += 0.08
		case "q":
			cancel()
		}
	})

	go func() {
		tick := time.NewTicker(time.Second / 30)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				r.RequestRender()
			}
		}
	}()

	draw := func(b *tui.Buffer) {
		mu.Lock()
		now := time.Now()
		if !paused {
			clock = math.Mod(clock+now.Sub(last).Seconds(), loopTime)
		}
		last = now
		t, cam, isPaused := clock, camera, paused
		mu.Unlock()

		w, h := b.Width(), b.Height()*2
		if w == 0 || h == 0 {
			return
		}
		if len(pixels) != w*h {
			pixels = make([]tui.RGBA, w*h)
		}
		fade := smoothstep(0, 0.8, t) * (1 - smoothstep(loopTime-1, loopTime, t))
		start := time.Now()
		render(newScene(t, cam), pixels, w, h, fade)
		frameTime = time.Since(start)

		for y := 0; y < b.Height(); y++ {
			for x := range w {
				b.Set(x, y, tui.Cell{Char: '▀', FG: pixels[2*y*w+x], BG: pixels[(2*y+1)*w+x]})
			}
		}
		status := fmt.Sprintf(" opentui-go · half-block pixels · %d×%d · %.0f ms ", w, h, float64(frameTime.Microseconds())/1000)
		if isPaused {
			status += "· paused "
		}
		dim := tui.RGB(150, 120, 120)
		b.DrawText(status, 1, 0, dim, tui.RGB(12, 8, 8), tui.AttrNone)
		b.DrawText(" Space pause · R restart · ←/→ orbit · Q quit ", 1, b.Height()-1, dim, tui.RGB(12, 8, 8), tui.AttrNone)
	}

	if err := r.Run(ctx, draw); err != nil {
		fmt.Fprintln(os.Stderr, "wine:", err)
		os.Exit(1)
	}
}
