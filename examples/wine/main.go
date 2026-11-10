// wine renders an animated 3D scene in the terminal: two glasses with
// glowing rims, wine poured into them from above, sloshing as they fill,
// and red light cast onto the table. It is ray marched on the CPU in plain
// Go, then drawn with half blocks: each cell shows "▀" with the upper pixel
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
	bowlY    = 1.55 // bowl center height
	bowlR    = 0.62 // bowl radius
	rimY     = bowlY + 0.40
	shell    = 0.012 // glass thickness
	stemR    = 0.03
	footR    = 0.42
	footH    = 0.03
	streamR  = 0.022
	pourTime = 5.0  // seconds a pour lasts
	loopTime = 17.0 // seconds before the scene restarts
	skyY     = 6.0  // where the stream comes from, above the frame
)

var (
	glassX = [2]float64{-1.25, 1.25}
	light  = v(-0.55, 0.75, -0.35).norm()
	wine   = v(0.40, 0.012, 0.028)
)

// pour is the per-glass state for one frame.
type pour struct {
	x            float64
	level        float64 // wine surface height
	slosh        float64 // surface tilt amplitude
	phase        float64
	stream       bool
	streamBottom float64
	streamTop    float64
	impact       float64 // ripple strength where the stream lands
}

type scene struct {
	t      float64
	glass  [2]pour
	camera float64 // orbit angle
}

func newScene(t, camera float64) *scene {
	s := &scene{t: t, camera: camera}
	low, high := bowlY-bowlR+0.10, bowlY+0.06
	for i := range s.glass {
		p := &s.glass[i]
		p.x = glassX[i]
		start := 1.0 + 1.6*float64(i)
		end := start + pourTime
		fill := smoothstep(start+0.35, end, t)
		p.level = low + (high-low)*fill
		// The stream's head falls from the sky when the pour starts and its
		// tail falls after it ends, both under gravity.
		const g = 14.0
		if t > start {
			p.streamBottom = math.Max(p.level, skyY-0.5*g*(t-start)*(t-start))
			p.streamTop = skyY
			if t > end {
				p.streamTop = skyY - 0.5*g*(t-end)*(t-end)
			}
			p.stream = p.streamTop > p.streamBottom+0.02
		}
		pouring := smoothstep(start, start+0.4, t) * (1 - smoothstep(end, end+1.2, t))
		p.impact = pouring
		p.slosh = 0.03*pouring + 0.02*math.Exp(-1.2*math.Max(t-end, 0))*smoothstep(start, start+1, t)
		p.phase = float64(i) * 1.7
	}
	return s
}

// surface is the height of the wine at (x, z), with a slosh tilt and
// ripples spreading from where the stream lands.
func (p *pour) surface(x, z, t float64) float64 {
	dx, dz := x-p.x, z
	h := p.level + p.slosh*(math.Sin(2.6*t+p.phase)*dx+0.6*math.Cos(2.1*t+p.phase)*dz)
	if p.impact > 0 {
		r := math.Hypot(dx-0.05, dz)
		h += 0.012 * p.impact * math.Sin(26*r-15*t) * math.Exp(-3.5*r)
	}
	return h
}

func glassSDF(p vec, gx float64) float64 {
	q := v(p.x-gx, p.y, p.z)
	b := q.sub(v(0, bowlY, 0))
	bowl := math.Max(math.Abs(b.length()-bowlR)-shell, q.y-rimY)
	r := math.Hypot(q.x, q.z)
	stemTop := bowlY - bowlR + 0.03
	stem := math.Max(r-stemR, math.Abs(q.y-stemTop/2)-stemTop/2)
	foot := math.Max(r-footR+0.012, math.Abs(q.y-footH/2)-footH/2) - 0.012
	return smin(smin(bowl, stem, 0.06), foot, 0.05)
}

func (s *scene) glassDist(p vec) float64 {
	// Only the nearer glass matters for distance.
	if p.x < 0 {
		return glassSDF(p, glassX[0])
	}
	return glassSDF(p, glassX[1])
}

func (s *scene) wineDist(p vec) float64 {
	g := &s.glass[1]
	if p.x < 0 {
		g = &s.glass[0]
	}
	d := math.Inf(1)
	if g.level > bowlY-bowlR+0.01 {
		inner := v(p.x-g.x, p.y-bowlY, p.z).length() - (bowlR - shell - 0.004)
		// The surface is a height field, not an exact distance; the 0.8
		// keeps the march from stepping through the ripples.
		d = math.Max(inner, 0.8*(p.y-g.surface(p.x, p.z, s.t)))
	}
	if g.stream {
		sx := g.x + 0.05 + 0.008*math.Sin(p.y*7+s.t*11)
		y := clamp(p.y, g.streamBottom, g.streamTop)
		stream := v(p.x-sx, p.y-y, p.z).length() - streamR
		d = smin(d, stream, 0.05)
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

// trace marches one ray. Glass is see-through: each time the ray touches
// it, a Fresnel highlight is added and the ray carries on, dimmed. Glow is
// accumulated near glass surfaces, a cheap bloom around the rims.
func (s *scene) trace(ro, rd vec) vec {
	col, trans, glow := vec{}, 1.0, 0.0
	t, glassHits := 0.02, 0
	for i := 0; i < 140 && t < 18; i++ {
		p := ro.add(rd.mul(t))
		d, what := s.dist(p)
		if gd := s.glassDist(p); gd > 0 {
			glow += trans * math.Exp(-gd*70) * math.Min(math.Max(d, 0.004), 0.05)
		}
		if d < 0.0015 {
			switch what {
			case hitGlass:
				n := normal(s.glassDist, p)
				fres := math.Pow(1-math.Abs(n.dot(rd)), 4)
				spec := math.Pow(math.Max(reflect(rd, n).dot(light), 0), 80)
				col = col.add(v(0.95, 0.93, 0.97).mul(trans * (fres*environment(reflect(rd, n)) + 1.2*spec)))
				trans *= 0.96 - 0.5*fres
				glassHits++
				if glassHits > 6 {
					return col.add(v(1, 0.95, 0.95).mul(glow * 1.3))
				}
				t += 0.03 // through the shell
				continue
			case hitWine:
				col = col.add(s.shadeWine(p, rd).mul(trans))
			case hitFloor:
				col = col.add(s.shadeFloor(p, rd).mul(trans))
			}
			return col.add(v(1, 0.95, 0.95).mul(glow * 1.3))
		}
		t += math.Max(d*0.85, 0.002)
	}
	return col.add(background(rd).mul(trans)).add(v(1, 0.95, 0.95).mul(glow * 1.3))
}

func (s *scene) shadeWine(p, rd vec) vec {
	n := normal(s.wineDist, p)
	base := wine
	if n.y > 0.75 && p.y > bowlY-bowlR+0.1 {
		base = v(0.62, 0.035, 0.055) // the lit surface
	}
	diff := 0.3 + 0.7*math.Max(n.dot(light), 0)
	fres := math.Pow(1-math.Abs(n.dot(rd)), 3)
	spec := math.Pow(math.Max(reflect(rd, n).dot(light), 0), 40)
	return base.mul(diff).add(v(0.9, 0.25, 0.25).mul(0.35 * fres)).add(v(1, 0.8, 0.8).mul(0.7 * spec))
}

func (s *scene) shadeFloor(p, rd vec) vec {
	c := v(0.03, 0.022, 0.022).mul(math.Exp(-0.06 * (p.x*p.x + p.z*p.z)))
	for i := range s.glass {
		g := &s.glass[i]
		dx, dz := p.x-g.x, p.z
		// Contact shadow under the foot.
		c = c.mul(1 - 0.75*math.Exp(-(dx*dx+dz*dz)*9))
		// Light through the wine lands to the right of the glass.
		fill := smoothstep(bowlY-bowlR, bowlY+0.06, g.level)
		lx, lz := dx-0.8, dz-0.25
		c = c.add(v(0.42, 0.02, 0.03).mul(fill * math.Exp(-(lx*lx*2.2 + lz*lz*9))))
	}
	// A dim, blurry reflection of the rims.
	spec := math.Pow(math.Max(reflect(rd, v(0, 1, 0)).dot(light), 0), 8)
	c = c.add(v(0.03, 0.022, 0.022).mul(spec))
	// Fade the table into the background with distance, so there is no
	// hard horizon.
	fog := smoothstep(4, 12, math.Hypot(p.x, p.z+2))
	return c.mul(1 - fog).add(background(rd).mul(fog))
}

// environment is how bright the dark room looks in a reflected direction:
// two tall soft lights stand off to the sides, so glass edges that face
// sideways catch them and the rest stays dark.
func environment(r vec) float64 {
	return 0.06 + 7*smoothstep(0.3, 0.9, math.Abs(r.x))*smoothstep(-0.3, 0.2, r.y)
}

func background(rd vec) vec {
	h := 0.5 + 0.5*rd.y
	spot := math.Exp(-rd.x * rd.x * 2.5)
	return v(0.028, 0.011, 0.011).mul(0.25 + 1.1*h*spot)
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
