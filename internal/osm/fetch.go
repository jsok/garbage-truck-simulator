// Package osm turns OpenStreetMap street data into a sim.Layout, so a suburb
// can be built on real streets.
//
// Street data comes from the Overpass API, which needs no account or key.
// OpenStreetMap data is © OpenStreetMap contributors, available under the
// Open Database Licence; anything built from it must credit them.
package osm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Attribution is the credit OpenStreetMap requires wherever its data is shown.
const Attribution = "Map data © OpenStreetMap contributors"

// Endpoints are public Overpass API interpreters, tried in turn: any one of
// them can be busy.
var Endpoints = []string{
	"https://overpass-api.de/api/interpreter",
	"https://overpass.kumi.systems/api/interpreter",
	"https://maps.mail.ru/osm/tools/overpass/api/interpreter",
}

// client gives up on a server a little after the query's own 60s timeout.
var client = &http.Client{Timeout: 90 * time.Second}

// userAgent identifies us to Overpass, as its usage policy asks.
const userAgent = "BinDay-garbage-truck-simulator/1.0 (+https://github.com/jsok/garbage-truck-simulator)"

// BBox is an area in degrees.
type BBox struct {
	South float64 `json:"south"`
	West  float64 `json:"west"`
	North float64 `json:"north"`
	East  float64 `json:"east"`
}

// Classes are the highway types that become streets: the ones a garbage
// truck does its rounds on. Lanes and car parks are always left out.
var Classes = []string{"residential", "living_street", "unclassified", "tertiary", "tertiary_link"}

// MainClasses are main roads, used only when asked for. They often join up
// neighbourhoods that would otherwise be separate.
var MainClasses = []string{"primary", "primary_link", "secondary", "secondary_link"}

// Element is a node or way in an Overpass JSON response.
type Element struct {
	Type  string            `json:"type"`
	ID    int64             `json:"id"`
	Lat   float64           `json:"lat,omitempty"`
	Lon   float64           `json:"lon,omitempty"`
	Nodes []int64           `json:"nodes,omitempty"`
	Tags  map[string]string `json:"tags,omitempty"`
}

// Data is an Overpass JSON response.
type Data struct {
	Elements []Element `json:"elements"`
}

// Query is the Overpass QL for the streets and main roads in b, with the
// nodes they use.
func Query(b BBox) string {
	return fmt.Sprintf(`[out:json][timeout:60];way["highway"~"^(%s|%s)$"]["area"!="yes"]["access"!~"^(private|no)$"](%f,%f,%f,%f);(._;>;);out body;`,
		strings.Join(Classes, "|"), strings.Join(MainClasses, "|"), b.South, b.West, b.North, b.East)
}

// Fetch downloads the streets in b from Overpass.
func Fetch(ctx context.Context, b BBox) (*Data, error) {
	var errs []error
	for _, endpoint := range Endpoints {
		d, err := fetch(ctx, endpoint, Query(b))
		if err == nil {
			return d, nil
		}
		errs = append(errs, err)
		if ctx.Err() != nil {
			break
		}
	}
	return nil, fmt.Errorf("osm: no Overpass server could answer: %w", errors.Join(errs...))
}

func fetch(ctx context.Context, endpoint, query string) (*Data, error) {
	host := endpoint
	if u, err := url.Parse(endpoint); err == nil {
		host = u.Host
	}
	form := url.Values{"data": {query}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", host, resp.Status)
	}
	var d Data
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, fmt.Errorf("%s: %w", host, err)
	}
	return &d, nil
}
