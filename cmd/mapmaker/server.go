package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/jsok/garbage-truck-simulator/internal/osm"
	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

//go:embed index.html
var indexHTML []byte

type server struct {
	out   string
	mu    sync.Mutex
	cache map[osm.BBox]*osm.Data // Overpass responses, so previews and saves don't refetch
}

func serve(addr, out string) {
	s := &server{out: out, cache: map[osm.BBox]*osm.Data{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})
	mux.HandleFunc("POST /api/preview", s.preview)
	mux.HandleFunc("POST /api/save", s.save)
	log.Printf("map maker: open http://%s/", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

// data returns the streets in b, downloading them the first time.
func (s *server) data(ctx context.Context, b osm.BBox) (*osm.Data, error) {
	s.mu.Lock()
	d := s.cache[b]
	s.mu.Unlock()
	if d != nil {
		return d, nil
	}
	d, err := osm.Fetch(ctx, b)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cache[b] = d
	s.mu.Unlock()
	return d, nil
}

type request struct {
	BBox      osm.BBox `json:"bbox"`
	MainRoads bool     `json:"mainRoads"`
	Name      string   `json:"name"`
}

func (r request) options() osm.Options { return osm.Options{MainRoads: r.MainRoads} }

func decode(w http.ResponseWriter, r *http.Request) (request, bool) {
	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return req, false
	}
	return req, true
}

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// LatLon is a point as [lat, lon], the way Leaflet takes it.
type LatLon [2]float64

type previewBin struct {
	At     LatLon `json:"at"`
	Colour string `json:"colour"`
}

// previewReply is what the game would build on the area, in degrees for
// drawing over the map.
type previewReply struct {
	Error  string       `json:"error,omitempty"`
	Hint   string       `json:"hint,omitempty"`
	Width  float64      `json:"width"`
	Height float64      `json:"height"`
	Roads  [][]LatLon   `json:"roads"`
	Bulbs  []LatLon     `json:"bulbs"`
	Houses [][]LatLon   `json:"houses"`
	Bins   []previewBin `json:"bins"`
	Start  *LatLon      `json:"start,omitempty"`
	Km     float64      `json:"km"`
}

func (s *server) preview(w http.ResponseWriter, r *http.Request) {
	req, ok := decode(w, r)
	if !ok {
		return
	}
	var out previewReply
	out.Width, out.Height = size(req.BBox)
	if err := checkSize(req.BBox); err != nil {
		out.Error = err.Error()
		reply(w, out)
		return
	}
	d, err := s.data(r.Context(), req.BBox)
	if err != nil {
		out.Error = err.Error()
		reply(w, out)
		return
	}
	l, st, t, err := build(d, req.BBox, req.options(), 1)
	if err != nil {
		out.Error = err.Error()
	}
	out.Hint = cutHint(st, req.MainRoads)
	proj := osm.Centre(req.BBox)
	ll := func(v sim.V2) LatLon {
		lat, lon := proj.ToLatLon(v)
		return LatLon{lat, lon}
	}
	if l != nil {
		for _, rd := range l.Roads {
			var line []LatLon
			for i, p := range rd.Pts {
				line = append(line, ll(p))
				if i > 0 {
					out.Km += p.Dist(rd.Pts[i-1]) / 1000
				}
			}
			out.Roads = append(out.Roads, line)
		}
	}
	if t != nil {
		for _, n := range t.Nodes {
			if n.Bulb {
				out.Bulbs = append(out.Bulbs, ll(n.P))
			}
		}
		for _, h := range t.Houses {
			f := sim.Dir(h.Heading)
			var poly []LatLon
			for _, c := range [][2]float64{{1, 1}, {1, -1}, {-1, -1}, {-1, 1}} {
				poly = append(poly, ll(h.P.Add(f.Scale(c[0]*h.D/2)).Add(f.Left().Scale(c[1]*h.W/2))))
			}
			out.Houses = append(out.Houses, poly)
		}
		for _, b := range t.Bins {
			out.Bins = append(out.Bins, previewBin{ll(b.Home), b.Colour.String()})
		}
		start := ll(t.StartPos)
		out.Start = &start
	}
	reply(w, out)
}

type saveReply struct {
	Error   string `json:"error,omitempty"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
}

func (s *server) save(w http.ResponseWriter, r *http.Request) {
	req, ok := decode(w, r)
	if !ok {
		return
	}
	fail := func(err error) { reply(w, saveReply{Error: err.Error()}) }
	d, err := s.data(r.Context(), req.BBox)
	if err != nil {
		fail(err)
		return
	}
	l, _, _, err := build(d, req.BBox, req.options(), 1)
	if err != nil {
		fail(err)
		return
	}
	path, err := save(s.out, req.Name, l)
	if err != nil {
		fail(err)
		return
	}
	log.Printf("saved %s", path)
	reply(w, saveReply{Path: path, Command: playCommand(path)})
}
