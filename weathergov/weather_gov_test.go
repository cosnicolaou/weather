// Copyright 2024 Cosmos Nicolaou. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package weathergov_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cloudeng.io/webapi/webapitestutil"
	"github.com/cosnicolaou/weather/weathergov"
)

func writeFile(name string, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	io.Copy(w, f)
}

func runMock() *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains("points", r.URL.Path) {
			writeFile("gridpoint.json", w)
		}
		if strings.Contains("forecast", r.URL.Path) {
			writeFile("forecast.json", w)
		}
	})
	return webapitestutil.NewServer(handler)
}

func TestLookup(t *testing.T) {
	ctx := context.Background()
	srv := runMock()
	defer srv.Close()

	fmt.Printf("server url: %s\n", srv.URL)
	api := weathergov.NewAPI()
	api.SetHost(srv.URL)
	gp, err := api.GetForecast(ctx, 39.7456, -97.0892)
	if err != nil {
		t.Fatalf("failed to get forecast: %v", err)
	}
	if got, want := gp.Lat, 39.7456; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := gp.Long, -97.0892; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := gp.GridX, 32; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := gp.GridY, 81; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := len(gp.Periods), 14; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	start := gp.Periods[0].StartTime
	end := gp.Periods[0].EndTime
	if got, want := end.Sub(start), time.Hour*6; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	for _, p := range gp.Periods[1:] {
		start := p.StartTime
		end := p.EndTime
		if got, want := end.Sub(start), time.Hour*12; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	}
	for _, p := range gp.Periods {
		if p.OpaqueCloudCoverage == weathergov.UnknownOpaqueCloudCoverage {
			t.Errorf("unexpected unknown cloud coverage: %q", p.ShortForecast)
		}
	}
}
