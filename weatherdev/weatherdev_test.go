// Copyright 2025 Cosmos Nicolaou. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package weatherdev_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"cloudeng.io/datetime"
	"cloudeng.io/webapi/clients/nws"
	"cloudeng.io/webapi/clients/nws/nwstestutil"
	"github.com/cosnicolaou/automation/devices"
	"github.com/cosnicolaou/weather/weatherdev"
)

func invokeCondition(t *testing.T, dev *weatherdev.Forecast, cond string, when time.Time, arg string) bool {
	ctx := context.Background()
	_, cover, err := dev.Conditions()[cond](ctx, devices.OperationArgs{
		Due:  when,
		Args: []string{arg},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	return cover
}

func TestMaxCloudCoverage(t *testing.T) {
	ctx := context.Background()

	srv := nwstestutil.NewMockServer()
	srv.SetValidTimes(time.Now())
	defer srv.Close()
	url := srv.Run()
	api := nws.NewAPI()
	api.SetHost(url)

	sys := devices.System{
		Location: devices.Location{
			Place: datetime.Place{
				TimeLocation: time.UTC,
				Latitude:     37.7749,
				Longitude:    -122.4194,
			},
		},
	}

	logOut := &strings.Builder{}
	logger := slog.New(slog.NewJSONHandler(logOut, nil))
	ws := weatherdev.NewService(devices.Options{
		Logger: logger,
	})
	ws.SetSystem(sys)
	ws.SetNWSAPI(api)

	forecast, err := ws.Forecasts(ctx, devices.OperationArgs{})
	if err != nil {
		t.Fatalf("failed to get forecasts: %v", err)
	}

	dev := weatherdev.NewForecast(devices.Options{Logger: logger})
	dev.SetController(ws)

	allArgs := []string{"sunny", "clear", "mostly clear", "mostly sunny", "partly cloudy",
		"partly sunny", "mostly cloudy", "cloudy"}

	for _, fc := range forecast.Periods {
		when := fc.StartTime
		for i, arg := range allArgs {
			maxCover := invokeCondition(t, dev, "max-cloud-cover", when, arg)
			minCover := invokeCondition(t, dev, "min-cloud-cover", when, arg)
			exactCover := invokeCondition(t, dev, "cloud-cover", when, arg)

			var maxOpacity, minOpacity bool
			forecastOpacity := nws.CloudOpacityFromShortForecast(fc.ShortForecast)
			argOpacity := nws.CloudOpacityFromShortForecast(arg)
			if forecastOpacity <= argOpacity {
				maxOpacity = true
			}
			if forecastOpacity >= argOpacity {
				minOpacity = true
			}
			if got, want := maxCover, maxOpacity; got != want {
				t.Errorf("period: %v, forecast: %q: arg: %q got %v, want %v", i, fc.ShortForecast, arg, got, want)
			}
			if got, want := minCover, minOpacity; got != want {
				t.Errorf("period: %v, forecast: %q: arg: %q got %v, want %v", i, fc.ShortForecast, arg, got, want)
			}

			if got, want := exactCover, maxOpacity == minOpacity; got != want {
				t.Errorf("period: %v, forecast: %q: arg: %q got %v, want %v", i, fc.ShortForecast, arg, got, want)
			}

			mostlySunny := invokeCondition(t, dev, "mostly-sunny", when, arg)
			partlyCloudy := invokeCondition(t, dev, "partly-cloudy", when, arg)
			partlySunny := invokeCondition(t, dev, "partly-cloudy", when, arg)
			mostlyCloudy := invokeCondition(t, dev, "mostly-cloudy", when, arg)

			if got, want := partlyCloudy, partlySunny; got != want {
				t.Errorf("period: %v, forecast: %q: arg: %q got %v, want %v", i, fc.ShortForecast, arg, got, want)
			}

			opc := nws.CloudOpacityFromShortForecast(fc.ShortForecast)
			if opc == nws.UnknownOpaqueCloudCoverage {
				t.Fatalf("unknown cloud cover: %q", arg)
			}
			if got, want := mostlySunny, opc <= nws.MostlyClearSunny; got != want {
				t.Errorf("period: %v, forecast: %q: mostlySunny got %v, want %v", i, fc.ShortForecast, got, want)
			}
			if got, want := mostlyCloudy, opc >= nws.MostlyCloudy; got != want {
				t.Errorf("period: %v, forecast: %q: mostlyCloudy got %v, want %v", i, fc.ShortForecast, got, want)
			}
		}
	}

	if got, want := srv.ForecastCalls(), 1; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}
