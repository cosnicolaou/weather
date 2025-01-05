// Copyright 2024 Cosmos Nicolaou. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package weathergov_test

import (
	"context"
	"testing"
	"time"

	"github.com/cosnicolaou/weather/weathergov"
)

func TestLookup(t *testing.T) {
	ctx := context.Background()
	api := weathergov.NewAPI()
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
