package main

import (
	"encoding/binary"
	"math"
	"math/rand/v2"

	"graphics.gd/variant/Object"

	"graphics.gd/classdb/AudioStream"
	"graphics.gd/classdb/AudioStreamPlayer"
	"graphics.gd/classdb/AudioStreamWAV"
	"graphics.gd/classdb/Node"
)

const mixRate = 22050

// Audio plays synthesised sound effects; there are no audio assets.
type Audio struct {
	engine, reverse, horn, hydraulic AudioStreamPlayer.Instance
	sfx                              map[string]AudioStream.Instance
	pool                             []AudioStreamPlayer.Instance
	next                             int
}

func wav(samples []float64, loop bool) AudioStream.Instance {
	buf := make([]byte, 2*len(samples))
	for i, s := range samples {
		v := int16(math.Max(-1, math.Min(1, s)) * 32000)
		binary.LittleEndian.PutUint16(buf[2*i:], uint16(v))
	}
	w := AudioStreamWAV.New()
	w.SetFormat(AudioStreamWAV.Format16Bits)
	w.SetMixRate(mixRate)
	w.SetStereo(false)
	w.SetData(buf)
	if loop {
		w.SetLoopMode(AudioStreamWAV.LoopForward)
		w.SetLoopBegin(0)
		w.SetLoopEnd(len(samples))
	}
	return Object.Leak(w.AsAudioStream())
}

// synth renders secs of audio from f(t).
func synth(secs float64, f func(t float64) float64) []float64 {
	out := make([]float64, int(secs*mixRate))
	for i := range out {
		out[i] = f(float64(i) / mixRate)
	}
	return out
}

func sine(f, t float64) float64 { return math.Sin(2 * math.Pi * f * t) }

// soft square: a few odd harmonics.
func buzz(f, t float64) float64 {
	return sine(f, t) + sine(3*f, t)/3 + sine(5*f, t)/5 + sine(7*f, t)/7
}

func env(t, attack, decay float64) float64 {
	if t < attack {
		return t / attack
	}
	return math.Exp(-(t - attack) / decay)
}

type noiseGen struct {
	rng  *rand.Rand
	low  float64
	band float64
}

// lowpassed noise; k controls the cutoff (0..1, smaller is darker).
func (n *noiseGen) next(k float64) float64 {
	n.low += k * (n.rng.Float64()*2 - 1 - n.low)
	return n.low
}

func newAudio(parent Node.Instance) *Audio {
	a := &Audio{sfx: map[string]AudioStream.Instance{}}
	ng := &noiseGen{rng: rand.New(rand.NewPCG(1, 2))}

	// Diesel idle: integer harmonics loop cleanly over one second.
	engine := synth(1, func(t float64) float64 {
		f := 34.0
		v := 0.0
		for k, amp := range []float64{1, 0.8, 0.55, 0.5, 0.3, 0.25, 0.15} {
			v += amp * math.Sin(2*math.Pi*f*float64(k+1)*t+float64(k)*1.3)
		}
		return 0.16 * v * (1 + 0.35*sine(f*2, t))
	})
	hyd := synth(1, func(t float64) float64 {
		return 0.18*(sine(190, t)+0.5*sine(380, t)+0.3*sine(570, t)) + 0.08*ng.next(0.3)
	})
	rev := synth(1, func(t float64) float64 {
		if t > 0.45 {
			return 0
		}
		return 0.35 * buzz(1030, t) * math.Min(1, t*200) * math.Min(1, (0.45-t)*200)
	})
	horn := synth(1, func(t float64) float64 { return 0.22 * (buzz(311, t) + buzz(370, t)) })

	a.engine = a.loop(parent, engine, -9)
	a.hydraulic = a.loop(parent, hyd, -12)
	a.reverse = a.loop(parent, rev, -12)
	a.horn = a.loop(parent, horn, -6)

	one := func(name string, secs float64, f func(t float64) float64) {
		a.sfx[name] = wav(synth(secs, f), false)
	}
	one("beep", 0.1, func(t float64) float64 { return 0.4 * sine(1320, t) * env(t, 0.005, 0.04) })
	one("chime", 0.7, func(t float64) float64 {
		v := 0.0
		for i, f := range []float64{784, 988, 1175, 1568} {
			st := float64(i) * 0.075
			if t > st {
				v += sine(f, t) * env(t-st, 0.004, 0.16)
			}
		}
		return 0.3 * v
	})
	one("thud", 0.5, func(t float64) float64 {
		return 0.9*sine(60*(1+0.6*math.Exp(-t*20)), t)*env(t, 0.003, 0.09) + 0.5*ng.next(0.2)*env(t, 0.002, 0.05)
	})
	one("red", 0.7, func(t float64) float64 {
		return 0.6*sine(75, t)*env(t, 0.005, 0.12) + 0.45*ng.next(0.15)*env(t, 0.01, 0.2)
	})
	pings := make([][2]float64, 14)
	for i := range pings {
		pings[i] = [2]float64{ng.rng.Float64() * 0.55, 1800 + ng.rng.Float64()*2600}
	}
	one("yellow", 0.8, func(t float64) float64 {
		v := 0.2 * ng.next(0.4) * env(t, 0.01, 0.15)
		for _, p := range pings {
			if t > p[0] {
				v += 0.25 * sine(p[1], t) * env(t-p[0], 0.001, 0.035)
			}
		}
		return v
	})
	one("green", 0.8, func(t float64) float64 {
		n := ng.next(0.6) - ng.next(0.05)
		return 0.7 * n * env(t, 0.12, 0.25)
	})
	one("blue", 0.6, func(t float64) float64 {
		// A soft whump of paper and a cardboard slap.
		v := 0.5*sine(110*(1+0.3*math.Exp(-t*15)), t)*env(t, 0.004, 0.1) + 0.35*ng.next(0.3)*env(t, 0.005, 0.12)
		if t > 0.12 {
			v += 0.3 * ng.next(0.5) * env(t-0.12, 0.002, 0.05)
		}
		return v
	})
	one("miss", 0.45, func(t float64) float64 {
		f := 380 - 500*t
		return 0.35 * math.Sin(2*math.Pi*f*t) * env(t, 0.01, 0.2)
	})
	one("bump", 0.4, func(t float64) float64 {
		return 0.9*sine(48, t)*env(t, 0.002, 0.1) + 0.6*ng.next(0.5)*env(t, 0.001, 0.03)
	})
	one("crash", 0.8, func(t float64) float64 {
		// A hollow plastic thump, a clatter, and a smaller bounce.
		v := 0.8*sine(95*(1+0.7*math.Exp(-t*25)), t)*env(t, 0.002, 0.08) + 0.5*ng.next(0.45)*env(t, 0.001, 0.12)
		if t > 0.2 {
			v += 0.4*sine(80, t)*env(t-0.2, 0.002, 0.06) + 0.3*ng.next(0.5)*env(t-0.2, 0.001, 0.06)
		}
		return v
	})
	one("shove", 0.15, func(t float64) float64 { return 0.5 * sine(170, t) * env(t, 0.002, 0.04) })
	one("tick", 0.05, func(t float64) float64 { return 0.35 * sine(2100, t) * env(t, 0.001, 0.01) })
	one("count", 0.25, func(t float64) float64 { return 0.35 * buzz(523, t) * env(t, 0.005, 0.1) })
	one("go", 0.6, func(t float64) float64 { return 0.35 * buzz(1046, t) * env(t, 0.005, 0.25) })
	one("buzzer", 1.1, func(t float64) float64 { return 0.3 * (buzz(174, t) + buzz(233, t)) * math.Min(1, (1.1-t)*8) })
	one("select", 0.08, func(t float64) float64 { return 0.3 * sine(880, t) * env(t, 0.002, 0.03) })
	one("combo", 0.5, func(t float64) float64 {
		f := 660 + 900*t
		return 0.25 * math.Sin(2*math.Pi*f*t) * env(t, 0.01, 0.2)
	})

	for range 8 {
		p := AudioStreamPlayer.New()
		addChild(parent, p.AsNode())
		a.pool = append(a.pool, p)
	}
	return a
}

func (a *Audio) loop(parent Node.Instance, samples []float64, db float64) AudioStreamPlayer.Instance {
	p := AudioStreamPlayer.New()
	addChild(parent, p.AsNode())
	p.SetStream(wav(samples, true))
	p.SetVolumeDb(f32(db))
	return p
}

// Play fires a one-shot effect.
func (a *Audio) Play(name string, pitch, db float64) {
	s, ok := a.sfx[name]
	if !ok {
		return
	}
	p := a.pool[a.next]
	a.next = (a.next + 1) % len(a.pool)
	p.SetStream(s)
	p.SetPitchScale(f32(pitch))
	p.SetVolumeDb(f32(db))
	p.Play()
}

func setLoop(p AudioStreamPlayer.Instance, on bool) {
	if on && !p.Playing() {
		p.Play()
	} else if !on && p.Playing() {
		p.Stop()
	}
}

// Update drives the continuous sounds.
func (a *Audio) Update(running bool, speed, throttle float64, horn, arm bool) {
	for _, p := range a.pool {
		Object.Use(p)
	}
	setLoop(a.engine, running)
	if running {
		a.engine.SetPitchScale(f32(0.75 + 1.3*math.Min(1, math.Abs(speed)/15) + 0.12*math.Max(0, throttle)))
	}
	setLoop(a.reverse, running && speed < -0.2)
	setLoop(a.horn, running && horn)
	setLoop(a.hydraulic, running && arm)
}
