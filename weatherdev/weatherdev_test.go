// Copyright 2025 Cosmos Nicolaou. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package weatherdev_test

import (
	"context"
	"fmt"
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
	writeOut := &strings.Builder{}
	ws := weatherdev.NewService(devices.Options{
		Logger: logger,
	})
	ws.SetSystem(sys)
	ws.SetNWSAPI(api)

	forecast, err := ws.Forecasts(ctx, devices.OperationArgs{})
	if err != nil {
		t.Fatalf("failed to get forecasts: %v", err)
	}

	dev := weatherdev.NewForecast(devices.Options{
		Logger: logger,
	})
	dev.SetController(ws)

	isItCloudy := dev.Conditions()["maxCloudCover"]

	allArgs := []string{"sunny", "clear", "mostly clear", "mostly sunny", "partly cloudy", "mostly sunny", "mostly cloudy", "cloudy"}

	for i, fc := range forecast.Periods {
		when := fc.StartTime
		for _, arg := range allArgs {
			ok, err := isItCloudy(ctx, devices.OperationArgs{
				Due:    when,
				Writer: writeOut,
				Args:   []string{arg}})
			if err != nil {
				t.Fatalf("%v: err: %v", when, err)
			}
			var expected bool
			forecastOpacity := nws.CloudOpacityFromShortForecast(fc.ShortForecast)
			argOpacity := nws.CloudOpacityFromShortForecast(arg)
			if forecastOpacity <= argOpacity {
				expected = true
			}
			if got, want := ok, expected; got != want {
				fmt.Printf("period: %v, forecast: %q: arg: %q got %v, want %v\n", i, fc.ShortForecast, arg, got, want)
				t.Errorf("period: %v, forecast: %q: arg: %q got %v, want %v", i, fc.ShortForecast, arg, got, want)
			}
		}
	}

	if got, want := srv.ForecastCalls(), 1; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

}
