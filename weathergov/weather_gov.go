// Copyright 2024 Cosmos Nicolaou. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package weathergov

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"cloudeng.io/webapi/operations"
)

// OpaqueCloudCoverage represents the cloud coverage as a fraction of the sky
// as defined at https://www.weather.gov/bgm/forecast_terms.
type OpaqueCloudCoverage int

const (
	UnknownOpaqueCloudCoverage OpaqueCloudCoverage = iota
	ClearSunny                                     // 0 to 1/8 Opaque Cloud Coverage
	MostlyClearSunny                               // 1/8 to 3/8
	PartlyCloudySunny                              // 3/8 to 5/8
	MostlyCloudy                                   // 5/8 to 7/8
	Cloudy
	Rain
	Snow
)

const (
	APIHost = "https://api.weather.gov"
)

type gridPointForecasts struct {
	X        int    `json:"gridX"`
	Y        int    `json:"gridY"`
	Hourly   string `json:"forecastHourly"`
	Forecast string `json:"forecast"`
}

type gridPointResponse struct {
	Properties gridPointForecasts `json:"properties"`
}

type Forecast struct {
	StartTime           time.Time `json:"startTime"`
	EndTime             time.Time `json:"endTime"`
	Name                string    `json:"name"`
	ShortForecast       string    `json:"shortForecast"`
	OpaqueCloudCoverage OpaqueCloudCoverage
}

type forecastResponse struct {
	Properties struct {
		Periods []Forecast `json:"periods"`
	}
}

type Forecasts struct {
	Lat     float64
	Long    float64
	GridX   int
	GridY   int
	Periods []Forecast
}

type API struct {
	gridEP *operations.Endpoint[gridPointResponse]
	opts   []operations.Option
	host   string
}

func NewAPI(opts ...operations.Option) *API {
	return &API{
		opts:   opts,
		host:   APIHost,
		gridEP: operations.NewEndpoint[gridPointResponse](opts...),
	}
}
func (a *API) SetHost(host string) {
	a.host = host
}

func (a *API) GetForecast(ctx context.Context, lat, long float64) (Forecasts, error) {
	//var u url.URL
	//	u.Scheme = "https"
	//	u.Host = a.host
	//	u.Path = fmt.Sprintf("%s/points/%f,%f", a.host, lat, long)
	u, err := url.Parse(fmt.Sprintf("%s/points/%f,%f", a.host, lat, long))
	if err != nil {
		return Forecasts{}, fmt.Errorf("failed to parse URL: %w", err)
	}
	gpr, buf, _, err := a.gridEP.Get(ctx, u.String())
	if err != nil {
		return Forecasts{}, fmt.Errorf("%v: grid point lookup failed: %w", u.String(), err)
	}
	os.WriteFile("gridpoint.json", buf, 0644)
	fcep := operations.NewEndpoint[forecastResponse](a.opts...)
	up, err := url.Parse(gpr.Properties.Forecast)
	if err != nil {
		return Forecasts{}, fmt.Errorf("%v: failed to parse forecast URL: %w", gpr.Properties.Forecast, err)
	}
	frc, buf, _, err := fcep.Get(ctx, up.String())
	if err != nil {
		return Forecasts{}, fmt.Errorf("%v: forecast download failed: %w", u.String(), err)
	}
	os.WriteFile("forecast.json", buf, 0644)
	fc := Forecasts{
		Lat:   lat,
		Long:  long,
		GridX: gpr.Properties.X,
		GridY: gpr.Properties.Y,
	}
	fc.Periods = make([]Forecast, len(frc.Properties.Periods))
	copy(fc.Periods, frc.Properties.Periods)
	for i, p := range fc.Periods {
		switch p.ShortForecast {
		case "Clear", "Sunny":
			fc.Periods[i].OpaqueCloudCoverage = ClearSunny
		case "Mostly Clear", "Mostly Sunny":
			fc.Periods[i].OpaqueCloudCoverage = MostlyClearSunny
		case "Partly Cloudy", "Partly Sunny":
			fc.Periods[i].OpaqueCloudCoverage = PartlyCloudySunny
		case "Mostly Cloudy":
			fc.Periods[i].OpaqueCloudCoverage = MostlyCloudy
		case "Cloudy":
			fc.Periods[i].OpaqueCloudCoverage = Cloudy
		default:
			fc.Periods[i].OpaqueCloudCoverage = estimateOpaqueCloudCoverage(p.ShortForecast)
		}
	}
	return fc, nil
}

func estimateOpaqueCloudCoverage(shortForecast string) OpaqueCloudCoverage {
	tl := strings.ToLower(shortForecast)
	if strings.Contains(tl, "rain") {
		return Rain
	}
	if strings.Contains(tl, "snow") {
		return Snow
	}
	return UnknownOpaqueCloudCoverage
}

func (fc Forecasts) ForTime(when time.Time) (Forecast, bool) {
	for _, f := range fc.Periods {
		if f.StartTime.Before(when) && f.EndTime.After(when) {
			return f, true
		}
	}
	return Forecast{}, false
}
